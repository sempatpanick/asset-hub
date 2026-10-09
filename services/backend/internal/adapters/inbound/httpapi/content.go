package httpapi

import (
	"assethub/internal/domain"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Refresh the write deadline as bytes flow, so bounded streams can outlive the
// server's ordinary response timeout while stalled clients still time out.
type idleContentWriter struct {
	writer     io.Writer
	controller *http.ResponseController
	timeout    time.Duration
}

func (w idleContentWriter) Write(data []byte) (int, error) {
	if err := w.controller.SetWriteDeadline(time.Now().Add(w.timeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return 0, err
	}
	return w.writer.Write(data)
}

func (s *Server) streamLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case s.streams <- struct{}{}:
			defer func() { <-s.streams }()
			next.ServeHTTP(w, r)
		default:
			w.Header().Set("Retry-After", "2")
			reply(w, 429, map[string]any{"error": map[string]string{"code": "busy", "message": "Download capacity is busy; retry shortly"}})
		}
	})
}
func (s *Server) contentIdentity(r *http.Request) (domain.Principal, error) {
	if authorization := r.Header.Get("Authorization"); authorization != "" {
		if !strings.HasPrefix(authorization, "Bearer ") {
			return domain.Principal{}, domain.ErrUnauthorized
		}
		return s.service.Key(r.Context(), strings.TrimPrefix(authorization, "Bearer "))
	}
	if cookie, err := r.Cookie(s.cookie); err == nil {
		p, err := s.service.Session(r.Context(), cookie.Value)
		if err == nil {
			return p, nil
		}
	}
	return domain.Principal{}, nil
}
func parseRange(value string, size int64) (int64, int64, bool, error) {
	if value == "" || strings.Contains(value, ",") {
		return 0, size - 1, false, nil
	}
	if !strings.HasPrefix(value, "bytes=") {
		return 0, size - 1, false, nil
	}
	bounds := strings.Split(strings.TrimPrefix(value, "bytes="), "-")
	if len(bounds) != 2 {
		return 0, 0, false, domain.ErrInvalid
	}
	if bounds[0] == "" {
		n, err := strconv.ParseInt(bounds[1], 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, false, domain.ErrInvalid
		}
		n = min(n, size)
		return size - n, size - 1, true, nil
	}
	start, err := strconv.ParseInt(bounds[0], 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, false, domain.ErrInvalid
	}
	end := size - 1
	if bounds[1] != "" {
		end, err = strconv.ParseInt(bounds[1], 10, 64)
		if err != nil || end < start {
			return 0, 0, false, domain.ErrInvalid
		}
		end = min(end, size-1)
	}
	return start, end, true, nil
}
func (s *Server) content(w http.ResponseWriter, r *http.Request) {
	i := s.ingest(w, r)
	if i == nil {
		return
	}
	p, err := s.contentIdentity(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	a, err := i.AuthorizeContent(r.Context(), p, chi.URLParam(r, "id"), r.URL.Query().Get("token"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	etag := `"` + a.SHA256 + `"`
	rangeValue := r.Header.Get("Range")
	if condition := r.Header.Get("If-Range"); condition != "" && condition != etag {
		rangeValue = ""
	}
	start, end, partial, err := parseRange(rangeValue, a.Size)
	if err != nil {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", a.Size))
		reply(w, 416, map[string]string{"error": "range_not_satisfiable"})
		return
	}
	var reader io.ReadCloser
	if r.Method != "HEAD" {
		reader, err = i.OpenContent(r.Context(), a, start, end)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		defer reader.Close()
	}
	disposition := "attachment"
	switch a.MIME {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/avif", "video/mp4", "video/webm", "video/ogg":
		disposition = "inline"
	}
	if r.URL.Query().Get("download") == "1" {
		disposition = "attachment"
	}
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Content-Type", a.MIME)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": a.Name}))
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	status := 200
	if partial {
		status = 206
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, a.Size))
	}
	controller := http.NewResponseController(w)
	if err = controller.SetWriteDeadline(time.Now().Add(150 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		panic(http.ErrAbortHandler)
	}
	w.WriteHeader(status)
	if r.Method == "HEAD" {
		return
	}
	if _, err = io.Copy(idleContentWriter{writer: w, controller: controller, timeout: 150 * time.Second}, reader); err != nil {
		panic(http.ErrAbortHandler)
	} // Never append JSON errors to partially streamed content.
}
func (s *Server) signedURL(w http.ResponseWriter, r *http.Request) {
	i := s.ingest(w, r)
	if i == nil {
		return
	}
	var body struct {
		Seconds int `json:"expires_in"`
	}
	if err := decode(w, r, &body); err != nil {
		s.fail(w, r, err)
		return
	}
	id := chi.URLParam(r, "id")
	token, err := i.SignedLink(r.Context(), current(r), id, body.Seconds)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	seconds := body.Seconds
	if seconds == 0 {
		seconds = 900
	}
	reply(w, 201, map[string]any{"url": s.origin + "/v1/assets/" + url.PathEscape(id) + "/content?token=" + url.QueryEscape(token), "expires_in": seconds})
}
func (s *Server) updateAsset(w http.ResponseWriter, r *http.Request) {
	i := s.ingest(w, r)
	if i == nil {
		return
	}
	var body struct {
		Name       *string `json:"name"`
		Visibility *string `json:"visibility"`
	}
	if err := decode(w, r, &body); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := i.Update(r.Context(), current(r), chi.URLParam(r, "id"), body.Name, body.Visibility); err != nil {
		s.fail(w, r, err)
		return
	}
	reply(w, 200, map[string]bool{"updated": true})
}
func (s *Server) deleteAsset(w http.ResponseWriter, r *http.Request) {
	i := s.ingest(w, r)
	if i == nil {
		return
	}
	if err := i.Delete(r.Context(), current(r), chi.URLParam(r, "id")); err != nil {
		s.fail(w, r, err)
		return
	}
	reply(w, 202, map[string]string{"status": "deleted"})
}
func (s *Server) restoreAsset(w http.ResponseWriter, r *http.Request) {
	i := s.ingest(w, r)
	if i == nil {
		return
	}
	if err := i.Restore(r.Context(), current(r), chi.URLParam(r, "id")); err != nil {
		s.fail(w, r, err)
		return
	}
	reply(w, 200, map[string]string{"status": "ready"})
}
