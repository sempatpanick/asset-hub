package telegram

import (
	"assethub/internal/domain"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

type Reference struct {
	ChannelID string `json:"channel_id"`
	MessageID int64  `json:"message_id"`
	FileID    string `json:"file_id"`
	UniqueID  string `json:"file_unique_id"`
}

// Replacing this boundary supports a local Bot API/MTProto transport without changing use cases.
type Transport interface {
	Send(context.Context, string, string, []byte) (Reference, error)
	Fetch(context.Context, string) ([]byte, error)
	Remove(context.Context, Reference) error
	Check(context.Context, string) error
}
type Gate interface {
	Wait(context.Context) error
	Delay(context.Context, time.Duration) error
}
type Provider struct {
	Transport Transport
	ChannelID string
	Gate      Gate
}

func (p *Provider) Name() string { return "telegram" }
func (p *Provider) Put(ctx context.Context, name string, data []byte) ([]byte, error) {
	if len(data) > int(domain.StorageChunkSize)+28 {
		return nil, domain.ErrInvalid
	}
	if err := p.Gate.Wait(ctx); err != nil {
		return nil, &domain.StorageError{Code: "queue_unavailable"}
	}
	ref, err := p.Transport.Send(ctx, p.ChannelID, name, data)
	if e, ok := err.(*domain.StorageError); ok && e.RetryAfter > 0 {
		_ = p.Gate.Delay(ctx, e.RetryAfter)
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(ref)
}
func (p *Provider) Open(ctx context.Context, raw []byte) ([]byte, error) {
	var ref Reference
	if json.Unmarshal(raw, &ref) != nil || ref.FileID == "" {
		return nil, domain.ErrIntegrity
	}
	return p.Transport.Fetch(ctx, ref.FileID)
}
func (p *Provider) Delete(ctx context.Context, raw []byte) error {
	var ref Reference
	if json.Unmarshal(raw, &ref) != nil || ref.MessageID < 1 || ref.ChannelID == "" {
		return domain.ErrIntegrity
	}
	if err := p.Gate.Wait(ctx); err != nil {
		return &domain.StorageError{Code: "queue_unavailable"}
	}
	err := p.Transport.Remove(ctx, ref)
	if e, ok := err.(*domain.StorageError); ok && e.RetryAfter > 0 {
		_ = p.Gate.Delay(ctx, e.RetryAfter)
	}
	return err
}
func (p *Provider) Health(ctx context.Context) domain.Health {
	h := domain.Health{Component: "telegram", Status: "healthy", Detail: "Bot access and channel administration verified"}
	if p.Transport.Check(ctx, p.ChannelID) != nil {
		h.Status = "unavailable"
		h.Detail = "Bot or storage channel unavailable"
	}
	return h
}

type BotAPI struct {
	base, token string
	client      *http.Client
}

func NewBotAPI(base, token string) (*BotAPI, error) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, domain.ErrInvalid
	}
	if token == "" || strings.ContainsAny(token, "/?#\r\n") {
		return nil, domain.ErrInvalid
	}
	return &BotAPI{strings.TrimRight(base, "/"), token, &http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

type envelope struct {
	OK         bool            `json:"ok"`
	Result     json.RawMessage `json:"result"`
	Code       int             `json:"error_code"`
	Parameters struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

func (b *BotAPI) call(ctx context.Context, method, contentType string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, "POST", b.base+"/bot"+b.token+"/"+method, body)
	if err != nil {
		return &domain.StorageError{Code: "telegram_configuration", Permanent: true}
	}
	req.Header.Set("Content-Type", contentType)
	res, err := b.client.Do(req)
	if err != nil {
		code := "telegram_unavailable"
		ambiguous := method == "sendDocument"
		if ambiguous {
			code = "telegram_ambiguous_send"
		}
		return &domain.StorageError{Code: code, Ambiguous: ambiguous}
	}
	defer res.Body.Close()
	var e envelope
	if json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&e) != nil {
		return &domain.StorageError{Code: "telegram_invalid_response", Ambiguous: method == "sendDocument"}
	}
	if !e.OK || res.StatusCode != 200 {
		if e.Code == 429 || res.StatusCode == 429 {
			delay := time.Duration(e.Parameters.RetryAfter) * time.Second
			if delay < time.Second {
				delay = time.Second
			}
			return &domain.StorageError{Code: "telegram_rate_limited", RetryAfter: delay}
		}
		code := e.Code
		if code == 0 {
			code = res.StatusCode
		}
		permanent := code >= 400 && code < 500 && code != 408
		return &domain.StorageError{Code: "telegram_request_failed", Permanent: permanent, Ambiguous: method == "sendDocument" && !permanent}
	}
	if json.Unmarshal(e.Result, out) != nil {
		return &domain.StorageError{Code: "telegram_invalid_response", Ambiguous: method == "sendDocument"}
	}
	return nil
}
func (b *BotAPI) jsonCall(ctx context.Context, method string, body, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return b.call(ctx, method, "application/json", bytes.NewReader(data), out)
}
func (b *BotAPI) Send(ctx context.Context, channel, name string, data []byte) (Reference, error) {
	reader, writer := io.Pipe()
	mw := multipart.NewWriter(writer)
	go func() {
		err := mw.WriteField("chat_id", channel)
		if err == nil {
			err = mw.WriteField("disable_notification", "true")
		}
		if err == nil {
			err = mw.WriteField("disable_content_type_detection", "true")
		}
		if err == nil {
			var file io.Writer
			file, err = mw.CreateFormFile("document", strings.ReplaceAll(name, "/", "-")+".enc")
			if err == nil {
				_, err = file.Write(data)
			}
		}
		if err == nil {
			err = mw.Close()
		}
		_ = writer.CloseWithError(err)
	}()
	defer reader.Close()
	var message struct {
		ID       int64 `json:"message_id"`
		Document struct {
			FileID   string `json:"file_id"`
			UniqueID string `json:"file_unique_id"`
		} `json:"document"`
	}
	err := b.call(ctx, "sendDocument", mw.FormDataContentType(), reader, &message)
	if err != nil {
		return Reference{}, err
	}
	if message.ID < 1 || message.Document.FileID == "" {
		return Reference{}, &domain.StorageError{Code: "telegram_invalid_response", Ambiguous: true}
	}
	return Reference{channel, message.ID, message.Document.FileID, message.Document.UniqueID}, nil
}
func (b *BotAPI) Fetch(ctx context.Context, fileID string) ([]byte, error) {
	var file struct {
		Path string `json:"file_path"`
		Size int64  `json:"file_size"`
	}
	if err := b.jsonCall(ctx, "getFile", map[string]string{"file_id": fileID}, &file); err != nil {
		return nil, err
	}
	if file.Path == "" || strings.HasPrefix(file.Path, "/") || path.Clean(file.Path) != file.Path || strings.ContainsAny(file.Path, "?#\\") || strings.HasPrefix(file.Path, "../") || file.Size > domain.StorageChunkSize+28 {
		return nil, domain.ErrIntegrity
	}
	escaped := []string{}
	for _, part := range strings.Split(file.Path, "/") {
		escaped = append(escaped, url.PathEscape(part))
	}
	req, err := http.NewRequestWithContext(ctx, "GET", b.base+"/file/bot"+b.token+"/"+strings.Join(escaped, "/"), nil)
	if err != nil {
		return nil, domain.ErrIntegrity
	}
	res, err := b.client.Do(req)
	if err != nil {
		return nil, &domain.StorageError{Code: "telegram_retrieval_failed"}
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, &domain.StorageError{Code: "telegram_retrieval_failed"}
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, domain.StorageChunkSize+29))
	if err != nil || int64(len(data)) > domain.StorageChunkSize+28 {
		return nil, domain.ErrIntegrity
	}
	return data, nil
}
func (b *BotAPI) Remove(ctx context.Context, ref Reference) error {
	var out bool
	return b.jsonCall(ctx, "deleteMessage", map[string]string{"chat_id": ref.ChannelID, "message_id": strconv.FormatInt(ref.MessageID, 10)}, &out)
}
func (b *BotAPI) Check(ctx context.Context, channel string) error {
	var bot struct {
		ID int64 `json:"id"`
	}
	if err := b.jsonCall(ctx, "getMe", map[string]string{}, &bot); err != nil {
		return err
	}
	var member struct {
		Status  string `json:"status"`
		CanPost bool   `json:"can_post_messages"`
	}
	if err := b.jsonCall(ctx, "getChatMember", map[string]string{"chat_id": channel, "user_id": fmt.Sprint(bot.ID)}, &member); err != nil {
		return err
	}
	if member.Status != "creator" && (member.Status != "administrator" || !member.CanPost) {
		return domain.ErrForbidden
	}
	return nil
}

// Missing configuration is visible as unavailable, and is never a fallback to Local.
type Unconfigured struct{}

func (Unconfigured) Name() string { return "telegram" }
func (Unconfigured) Put(context.Context, string, []byte) ([]byte, error) {
	return nil, &domain.StorageError{Code: "telegram_not_configured", Permanent: true}
}
func (Unconfigured) Open(context.Context, []byte) ([]byte, error) { return nil, domain.ErrUnavailable }
func (Unconfigured) Delete(context.Context, []byte) error         { return domain.ErrUnavailable }
func (Unconfigured) Health(context.Context) domain.Health {
	return domain.Health{Component: "telegram", Status: "unavailable", Detail: "Configure bot credentials and a private storage channel"}
}
