package httpapi

import (
	"assethub/internal/application/usecases"
	"assethub/internal/domain"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"github.com/go-chi/chi/v5"
	"io"
	"net/http"
	"strconv"
	"sync"
)

// Keep the active count when the limit changes so lowering it never interrupts
// existing requests or creates extra capacity while they are still running.
type transferGate struct {
	mu     sync.Mutex
	active int
}

func (g *transferGate) acquire(limit int) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active >= limit {
		return false
	}
	g.active++
	return true
}

func (g *transferGate) release() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.active--
}

func (s *Server) transferLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit, err := s.service.UploadConcurrency(r.Context())
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if !s.transfers.acquire(limit) {
			w.Header().Set("Retry-After", "2")
			reply(w, 429, map[string]any{"error": map[string]string{"code": "busy", "message": "Upload transfers are busy; retry shortly"}})
			return
		}
		defer s.transfers.release()
		next.ServeHTTP(w, r)
	})
}
func (s *Server) ingest(w http.ResponseWriter, r *http.Request) *usecases.Ingestion {
	v := s.service.Ingestion()
	if v == nil {
		s.unavailable(w, r)
	}
	return v
}
func uploadView(u domain.Upload) any {
	parts := []any{}
	for _, p := range u.Parts {
		parts = append(parts, map[string]any{"index": p.Index, "size_bytes": p.Size, "sha256": p.SHA256})
	}
	return map[string]any{"id": u.ID, "project_id": u.ProjectID, "name": u.Name, "size_bytes": u.Size, "status": u.Status, "asset_id": u.AssetID, "part_bytes": domain.UploadPartSize, "parts": parts, "expires_at": u.ExpiresAt, "created_at": u.CreatedAt, "asset_status": u.AssetStatus, "error_code": u.ErrorCode, "uploaded_chunks": u.UploadedChunks}
}
func (s *Server) uploads(w http.ResponseWriter, r *http.Request) {
	i := s.ingest(w, r)
	if i == nil {
		return
	}
	limit, offset := page(r)
	values, err := i.List(r.Context(), current(r), r.URL.Query().Get("q"), r.URL.Query().Get("status"), r.URL.Query().Get("project_id"), limit, offset)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items := []any{}
	for _, u := range values {
		items = append(items, uploadView(u))
	}
	reply(w, 200, map[string]any{"items": items, "limit": limit, "offset": offset})
}
func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	i := s.ingest(w, r)
	if i == nil {
		return
	}
	u, err := i.Get(r.Context(), current(r), chi.URLParam(r, "id"), "read")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	reply(w, 200, uploadView(u))
}
func (s *Server) createUpload(w http.ResponseWriter, r *http.Request) {
	i := s.ingest(w, r)
	if i == nil {
		return
	}
	var b struct {
		Project string `json:"project_id"`
		Name    string `json:"name"`
		Size    int64  `json:"size_bytes"`
		SHA256  string `json:"sha256"`
	}
	if err := decode(w, r, &b); err != nil {
		s.fail(w, r, err)
		return
	}
	u, err := i.Create(r.Context(), current(r), b.Project, b.Name, b.Size, b.SHA256)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	reply(w, 201, uploadView(u))
}
func (s *Server) putPart(w http.ResponseWriter, r *http.Request) {
	i := s.ingest(w, r)
	if i == nil {
		return
	}
	index, err := strconv.Atoi(chi.URLParam(r, "index"))
	if err != nil {
		s.fail(w, r, domain.ErrInvalid)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, domain.UploadPartSize+1)
	err = i.PutPart(r.Context(), current(r), chi.URLParam(r, "id"), index, r.Header.Get("X-Content-SHA256"), r.Body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	reply(w, 200, map[string]bool{"received": true})
}
func (s *Server) completeUpload(w http.ResponseWriter, r *http.Request) {
	i := s.ingest(w, r)
	if i == nil {
		return
	}
	a, err := i.Complete(r.Context(), current(r), chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	reply(w, 202, assetView(a))
}
func (s *Server) cancelUpload(w http.ResponseWriter, r *http.Request) {
	i := s.ingest(w, r)
	if i == nil {
		return
	}
	if err := i.Cancel(r.Context(), current(r), chi.URLParam(r, "id")); err != nil {
		s.fail(w, r, err)
		return
	}
	reply(w, 200, map[string]bool{"cancelled": true})
}
func (s *Server) asset(w http.ResponseWriter, r *http.Request) {
	i := s.ingest(w, r)
	if i == nil {
		return
	}
	a, err := i.Metadata(r.Context(), current(r), chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v := assetView(a.Asset).(map[string]any)
	v["error_code"] = a.ErrorCode
	v["attempts"] = a.Attempts
	v["uploaded_chunks"] = len(a.Chunks)
	reply(w, 200, v)
}
func (s *Server) retryAsset(w http.ResponseWriter, r *http.Request) {
	i := s.ingest(w, r)
	if i == nil {
		return
	}
	if err := i.Retry(r.Context(), current(r), chi.URLParam(r, "id")); err != nil {
		s.fail(w, r, err)
		return
	}
	reply(w, 202, map[string]string{"status": "queued"})
}
func (s *Server) multipartUpload(w http.ResponseWriter, r *http.Request) {
	i := s.ingest(w, r)
	if i == nil {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, domain.UploadPartSize+(64<<10))
	reader, err := r.MultipartReader()
	if err != nil {
		s.fail(w, r, domain.ErrInvalid)
		return
	}
	project, name := "", ""
	var data []byte
	seen := map[string]bool{}
	for {
		part, e := reader.NextPart()
		if e == io.EOF {
			break
		}
		if e != nil {
			s.fail(w, r, domain.ErrInvalid)
			return
		}
		field := part.FormName()
		if seen[field] {
			part.Close()
			s.fail(w, r, domain.ErrInvalid)
			return
		}
		seen[field] = true
		switch field {
		case "project_id":
			v, e := io.ReadAll(io.LimitReader(part, 181))
			if e != nil || len(v) > 180 {
				part.Close()
				s.fail(w, r, domain.ErrInvalid)
				return
			}
			project = string(v)
		case "file":
			name = part.FileName()
			data, e = io.ReadAll(io.LimitReader(part, domain.UploadPartSize+1))
			if e != nil || int64(len(data)) > domain.UploadPartSize {
				part.Close()
				s.fail(w, r, domain.ErrCapacity)
				return
			}
		default:
			part.Close()
			s.fail(w, r, domain.ErrInvalid)
			return
		}
		part.Close()
	}
	hash := sha256.Sum256(data)
	checksum := hex.EncodeToString(hash[:])
	u, err := i.Create(r.Context(), current(r), project, name, int64(len(data)), checksum)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err = i.PutPart(r.Context(), current(r), u.ID, 0, checksum, bytes.NewReader(data)); err != nil {
		_ = i.Cancel(r.Context(), current(r), u.ID)
		s.fail(w, r, err)
		return
	}
	a, err := i.Complete(r.Context(), current(r), u.ID)
	if err != nil {
		_ = i.Cancel(r.Context(), current(r), u.ID)
		s.fail(w, r, err)
		return
	}
	reply(w, 202, assetView(a))
}
