package telegram

import (
	"assethub/internal/domain"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBotTransportUploadDownloadAndSafeFailures(t *testing.T) {
	data := []byte("encrypted bytes")
	token := "123:test-secret"
	mode := "success"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "sendDocument") {
			if mode == "rate" {
				w.WriteHeader(429)
				io.WriteString(w, `{"ok":false,"error_code":429,"description":"sensitive details","parameters":{"retry_after":19}}`)
				return
			}
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
				return
			}
			if r.FormValue("chat_id") != "private-channel" {
				t.Error("channel missing")
			}
			file, _, err := r.FormFile("document")
			if err != nil {
				t.Error(err)
				return
			}
			defer file.Close()
			actual, _ := io.ReadAll(file)
			if string(actual) != string(data) {
				t.Error("changed ciphertext")
			}
			io.WriteString(w, `{"ok":true,"result":{"message_id":42,"document":{"file_id":"file-ref","file_unique_id":"unique"}}}`)
		} else if strings.HasSuffix(r.URL.Path, "getFile") {
			path := "documents/encrypted.bin"
			if mode == "traversal" {
				path = "../secret"
			}
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"file_path": path, "file_size": len(data)}})
		} else if strings.Contains(r.URL.Path, "/file/bot") {
			w.Write(data)
		} else {
			w.WriteHeader(403)
			io.WriteString(w, `{"ok":false,"error_code":403,"description":"private-channel secret"}`)
		}
	}))
	defer server.Close()
	api, _ := NewBotAPI(server.URL, token)
	ref, err := api.Send(context.Background(), "private-channel", "asset/0", data)
	if err != nil || ref.MessageID != 42 {
		t.Fatal(err)
	}
	actual, err := api.Fetch(context.Background(), ref.FileID)
	if err != nil || string(actual) != string(data) {
		t.Fatal("fetch failed", err)
	}
	mode = "rate"
	_, err = api.Send(context.Background(), "private-channel", "asset/0", data)
	var storage *domain.StorageError
	if !errors.As(err, &storage) || storage.RetryAfter.Seconds() != 19 || strings.Contains(err.Error(), token) {
		t.Fatal("rate limit classification", err)
	}
	mode = "traversal"
	if _, err = api.Fetch(context.Background(), ref.FileID); !errors.Is(err, domain.ErrIntegrity) {
		t.Fatal("unsafe path accepted")
	}
	mode = "success"
	err = api.Remove(context.Background(), ref)
	if !errors.As(err, &storage) || !storage.Permanent || strings.Contains(err.Error(), "private-channel") {
		t.Fatal("private error escaped")
	}
	server.Close()
	_, err = api.Send(context.Background(), "private-channel", "asset/0", data)
	if !errors.As(err, &storage) || !storage.Ambiguous || strings.Contains(err.Error(), token) {
		t.Fatal("ambiguous network failure not recorded")
	}
}
