package httpapi

import (
	"assethub/internal/application/usecases"
	"assethub/internal/domain"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type principalKey struct{}
type Server struct {
	service        *usecases.Service
	origin, cookie string
	secure         bool
	log            *slog.Logger
	limiter        *limiter
	transfers      transferGate
	streams        chan struct{}
}
type bucket struct {
	count int
	until time.Time
}
type limiter struct {
	mu     sync.Mutex
	items  map[string]bucket
	active chan struct{}
}

func New(service *usecases.Service, origin string, secure bool, log *slog.Logger) http.Handler {
	cookie := "ah_session"
	if secure {
		cookie = "__Host-ah_session"
	}
	s := &Server{service: service, origin: origin, cookie: cookie, secure: secure, log: log, limiter: &limiter{items: map[string]bucket{}, active: make(chan struct{}, 2)}, streams: make(chan struct{}, 4)}
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.Recoverer, s.headers)
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, map[string]any{"status": "alive"}) })
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if service.Ready(ctx) != nil {
			reply(w, 503, map[string]string{"status": "unavailable"})
			return
		}
		reply(w, 200, map[string]string{"status": "ready"})
	})
	r.Route("/v1", func(r chi.Router) {
		r.With(s.streamLimit).Get("/assets/{id}/content", s.content)
		r.With(s.streamLimit).Head("/assets/{id}/content", s.content)
		r.With(s.authLimit, s.originCheck).Post("/auth/login", s.login)
		r.With(s.authLimit, s.originCheck).Post("/auth/accept-invitation", s.accept)
		r.Group(func(r chi.Router) {
			r.Use(s.authenticate)
			r.Get("/auth/me", s.me)
			r.With(s.mutation).Post("/auth/logout", s.logout)
			r.With(s.mutation).Post("/invitations", s.invite)
			r.Get("/projects", s.projects)
			r.With(s.mutation).Post("/projects", s.createProject)
			r.With(s.mutation).Patch("/projects/{id}", s.updateProject)
			r.Get("/projects/{id}/members", s.members)
			r.With(s.mutation).Post("/projects/{id}/members", s.setMember)
			r.Get("/api-keys", s.keys)
			r.With(s.mutation).Post("/api-keys", s.createKey)
			r.With(s.mutation).Delete("/api-keys/{id}", s.revokeKey)
			r.With(s.mutation, s.authLimit).Post("/api-keys/{id}/reveal", s.revealKey)
			r.Get("/overview", s.overview)
			r.Get("/assets", s.assets)
			r.Get("/activity", s.activity)
			r.Get("/storage/health", s.health)
			r.Get("/settings", s.settings)
			r.With(s.mutation).Patch("/settings", s.updateSettings)
			r.Get("/uploads", s.uploads)
			r.Get("/uploads/{id}", s.upload)
			r.Get("/assets/{id}", s.asset)
			r.With(s.mutation, s.uploadTimeout).Post("/uploads", s.createUpload)
			r.With(s.mutation, s.transferLimit).Put("/uploads/{id}/parts/{index}", s.putPart)
			r.With(s.mutation, s.transferLimit).Post("/uploads/{id}/complete", s.completeUpload)
			r.With(s.mutation).Delete("/uploads/{id}", s.cancelUpload)
			r.With(s.mutation).Post("/assets/{id}/retry", s.retryAsset)
			r.With(s.mutation, s.transferLimit).Post("/assets", s.multipartUpload)
			r.With(s.mutation).Post("/assets/{id}/signed-url", s.signedURL)
			r.With(s.mutation).Patch("/assets/{id}", s.updateAsset)
			r.With(s.mutation).Delete("/assets/{id}", s.deleteAsset)
			r.With(s.mutation).Post("/assets/{id}/restore", s.restoreAsset)
		})
	})
	return r
}
func (s *Server) headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Request-ID", middleware.GetReqID(r.Context()))
		next.ServeHTTP(w, r)
	})
}
func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message := 500, "internal_error", "Request failed"
	switch {
	case errors.Is(err, domain.ErrUnauthorized):
		status, code, message = 401, "unauthorized", "Invalid credentials or expired session"
	case errors.Is(err, domain.ErrForbidden):
		status, code, message = 403, "forbidden", "This action is not permitted"
	case errors.Is(err, domain.ErrPassword):
		status, code, message = 403, "incorrect_password", "Incorrect dashboard password. Try again."
	case errors.Is(err, domain.ErrKeySecret):
		status, code, message = 409, "key_secret_unavailable", "This older key cannot be copied. Create a replacement key."
	case errors.Is(err, domain.ErrNotFound):
		status, code, message = 404, "not_found", "Resource not found"
	case errors.Is(err, domain.ErrInvalid):
		status, code, message = 400, "invalid_input", "Check the supplied fields"
	case errors.Is(err, domain.ErrConflict):
		status, code, message = 409, "conflict", "Resource exists, has expired, or violates a constraint"
	case errors.Is(err, domain.ErrUnavailable):
		status, code, message = 503, "storage_unavailable", "Required storage, backup or signing configuration is unavailable"
	case errors.Is(err, domain.ErrCapacity):
		status, code, message = 413, "capacity_exceeded", "Project quota, staging capacity or disk reserve exceeded"
	case errors.Is(err, domain.ErrIntegrity):
		status, code, message = 422, "integrity_failed", "File length or checksum verification failed"
	}
	if status == 500 {
		s.log.Error("request failed", "request_id", middleware.GetReqID(r.Context()), "method", r.Method, "path", r.URL.Path)
	}
	reply(w, status, map[string]any{"error": map[string]string{"code": code, "message": message, "request_id": middleware.GetReqID(r.Context())}})
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return domain.ErrInvalid
	}
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return domain.ErrInvalid
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return domain.ErrInvalid
	}
	return nil
}
func (s *Server) originCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != s.origin {
			s.fail(w, r, domain.ErrForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p domain.Principal
		var err error
		if value := r.Header.Get("Authorization"); value != "" {
			if !strings.HasPrefix(value, "Bearer ") {
				s.fail(w, r, domain.ErrUnauthorized)
				return
			}
			p, err = s.service.Key(r.Context(), strings.TrimPrefix(value, "Bearer "))
		} else {
			c, e := r.Cookie(s.cookie)
			if e != nil {
				s.fail(w, r, domain.ErrUnauthorized)
				return
			}
			p, err = s.service.Session(r.Context(), c.Value)
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	})
}
func current(r *http.Request) domain.Principal {
	return r.Context().Value(principalKey{}).(domain.Principal)
}
func (s *Server) mutation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := current(r)
		if p.KeyID == "" && (r.Header.Get("Origin") != s.origin || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(p.CSRF)) != 1) {
			s.fail(w, r, domain.ErrForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) authLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		if p, ok := r.Context().Value(principalKey{}).(domain.Principal); ok && p.User.ID != "" {
			host = "password:" + p.User.ID
		}
		now := time.Now()
		l := s.limiter
		l.mu.Lock()
		for k, b := range l.items {
			if now.After(b.until) {
				delete(l.items, k)
			}
		}
		b := l.items[host]
		if b.until.IsZero() {
			b.until = now.Add(time.Minute)
		}
		_, known := l.items[host]
		denied := b.count >= 10 || (!known && len(l.items) >= 4096)
		if !denied {
			b.count++
			l.items[host] = b
		}
		l.mu.Unlock()
		if denied {
			w.Header().Set("Retry-After", "60")
			reply(w, 429, map[string]any{"error": map[string]string{"code": "rate_limited", "message": "Try again in one minute"}})
			return
		}
		select {
		case l.active <- struct{}{}:
			defer func() { <-l.active }()
			next.ServeHTTP(w, r)
		default:
			w.Header().Set("Retry-After", "2")
			reply(w, 429, map[string]any{"error": map[string]string{"code": "busy", "message": "Try again shortly"}})
		}
	})
}
func userView(u domain.User) any {
	return map[string]any{"id": u.ID, "email": u.Email, "name": u.Name, "role": u.Role}
}
func projectView(p domain.Project) any {
	return map[string]any{"id": p.ID, "name": p.Name, "description": p.Description, "role": p.Role, "quota_bytes": p.QuotaBytes, "used_bytes": p.UsedBytes, "files": p.Files, "created_at": p.CreatedAt}
}
func assetView(a domain.Asset) any {
	return map[string]any{"id": a.ID, "project_id": a.ProjectID, "name": a.Name, "mime": a.MIME, "size_bytes": a.Size, "sha256": a.SHA256, "status": a.Status, "visibility": a.Visibility, "created_at": a.CreatedAt}
}
func keyView(k domain.APIKey) any {
	return map[string]any{"id": k.ID, "project_id": k.ProjectID, "name": k.Name, "prefix": k.Prefix, "scopes": k.Scopes, "expires_at": k.ExpiresAt, "revoked_at": k.RevokedAt, "created_at": k.CreatedAt, "secret_available": k.SecretAvailable}
}
func (s *Server) revealKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := decode(w, r, &body); err != nil {
		s.fail(w, r, err)
		return
	}
	secret, err := s.service.RevealKey(r.Context(), current(r), r.URL.Query().Get("project_id"), chi.URLParam(r, "id"), body.Password)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	reply(w, 200, map[string]string{"secret": secret})
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(w, r, &body); err != nil {
		s.fail(w, r, err)
		return
	}
	u, t, csrf, err := s.service.Login(r.Context(), body.Email, body.Password)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: s.cookie, Value: t, Path: "/", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode, MaxAge: 12 * 3600})
	reply(w, 200, map[string]any{"user": userView(u), "csrf_token": csrf})
}
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	p := current(r)
	if p.KeyID != "" {
		s.fail(w, r, domain.ErrForbidden)
		return
	}
	reply(w, 200, map[string]any{"user": userView(p.User), "csrf_token": p.CSRF})
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	p := current(r)
	if p.KeyID != "" {
		s.fail(w, r, domain.ErrForbidden)
		return
	}
	c, _ := r.Cookie(s.cookie)
	if err := s.service.Logout(r.Context(), c.Value); err != nil {
		s.fail(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: s.cookie, Path: "/", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	reply(w, 200, map[string]bool{"ok": true})
}
func (s *Server) invite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := decode(w, r, &body); err != nil {
		s.fail(w, r, err)
		return
	}
	t, err := s.service.Invite(r.Context(), current(r), body.Email, body.Role)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	reply(w, 201, map[string]string{"url": s.origin + "/invite/#token=" + t, "expires_in": "48 hours"})
}
func (s *Server) accept(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token    string `json:"token"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := decode(w, r, &body); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.service.Accept(r.Context(), body.Token, body.Name, body.Password); err != nil {
		s.fail(w, r, err)
		return
	}
	reply(w, 201, map[string]bool{"ok": true})
}
func (s *Server) projects(w http.ResponseWriter, r *http.Request) {
	v, err := s.service.Projects(r.Context(), current(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items := []any{}
	for _, p := range v {
		items = append(items, projectView(p))
	}
	reply(w, 200, map[string]any{"items": items})
}
func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Quota       int64  `json:"quota_bytes"`
	}
	if err := decode(w, r, &b); err != nil {
		s.fail(w, r, err)
		return
	}
	v, err := s.service.CreateProject(r.Context(), current(r), b.Name, b.Description, b.Quota)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	reply(w, 201, projectView(v))
}
func (s *Server) updateProject(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Quota       int64  `json:"quota_bytes"`
	}
	if err := decode(w, r, &b); err != nil {
		s.fail(w, r, err)
		return
	}
	err := s.service.UpdateProject(r.Context(), current(r), domain.Project{ID: chi.URLParam(r, "id"), Name: b.Name, Description: b.Description, QuotaBytes: b.Quota})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	reply(w, 200, map[string]bool{"ok": true})
}
func (s *Server) members(w http.ResponseWriter, r *http.Request) {
	v, err := s.service.Members(r.Context(), current(r), chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items := []any{}
	for _, m := range v {
		items = append(items, map[string]string{"user_id": m.UserID, "email": m.Email, "name": m.Name, "role": m.Role})
	}
	reply(w, 200, map[string]any{"items": items})
}
func (s *Server) setMember(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := decode(w, r, &b); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.service.SetMember(r.Context(), current(r), chi.URLParam(r, "id"), b.Email, b.Role); err != nil {
		s.fail(w, r, err)
		return
	}
	reply(w, 200, map[string]bool{"ok": true})
}
func (s *Server) keys(w http.ResponseWriter, r *http.Request) {
	v, err := s.service.Keys(r.Context(), current(r), r.URL.Query().Get("project_id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items := []any{}
	for _, k := range v {
		items = append(items, keyView(k))
	}
	reply(w, 200, map[string]any{"items": items})
}
func (s *Server) createKey(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Project string     `json:"project_id"`
		Name    string     `json:"name"`
		Scopes  []string   `json:"scopes"`
		Expires *time.Time `json:"expires_at"`
	}
	if err := decode(w, r, &b); err != nil {
		s.fail(w, r, err)
		return
	}
	k, t, err := s.service.CreateKey(r.Context(), current(r), b.Project, b.Name, b.Scopes, b.Expires)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	reply(w, 201, map[string]any{"key": keyView(k), "secret": t})
}
func (s *Server) revokeKey(w http.ResponseWriter, r *http.Request) {
	if err := s.service.RevokeKey(r.Context(), current(r), r.URL.Query().Get("project_id"), chi.URLParam(r, "id")); err != nil {
		s.fail(w, r, err)
		return
	}
	reply(w, 200, map[string]bool{"ok": true})
}
func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	p := current(r)
	if p.KeyID != "" {
		s.fail(w, r, domain.ErrForbidden)
		return
	}
	v, err := s.service.Overview(r.Context(), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	recent := []any{}
	days := []any{}
	for _, a := range v.Recent {
		recent = append(recent, assetView(a))
	}
	for _, d := range v.Activity {
		days = append(days, map[string]any{"day": d.Day, "count": d.Count})
	}
	reply(w, 200, map[string]any{"files": v.Files, "bytes": v.Bytes, "projects": v.Projects, "queued": v.Queued, "failed": v.Failed, "recent": recent, "activity": days})
}
func page(r *http.Request) (int, int) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if offset < 0 || offset > 100000 {
		offset = 0
	}
	return limit, offset
}
func (s *Server) assets(w http.ResponseWriter, r *http.Request) {
	limit, offset := page(r)
	v, err := s.service.Assets(r.Context(), current(r), r.URL.Query().Get("project_id"), r.URL.Query().Get("q"), r.URL.Query().Get("status"), r.URL.Query().Get("type"), limit, offset)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items := []any{}
	for _, a := range v {
		items = append(items, assetView(a))
	}
	reply(w, 200, map[string]any{"items": items, "limit": limit, "offset": offset})
}
func (s *Server) activity(w http.ResponseWriter, r *http.Request) {
	limit, offset := page(r)
	v, err := s.service.Activity(r.Context(), current(r), r.URL.Query().Get("q"), r.URL.Query().Get("action"), r.URL.Query().Get("project_id"), limit, offset)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items := []any{}
	for _, a := range v {
		items = append(items, map[string]any{"id": a.ID, "actor": a.Actor, "action": a.Action, "project_id": a.ProjectID, "object_id": a.ObjectID, "detail": a.Detail, "created_at": a.CreatedAt})
	}
	reply(w, 200, map[string]any{"items": items, "limit": limit, "offset": offset})
}
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	values, err := s.service.Health(ctx, current(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items := []any{}
	for _, v := range values {
		items = append(items, map[string]string{"component": v.Component, "status": v.Status, "detail": v.Detail})
	}
	reply(w, 200, map[string]any{"items": items})
}
func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	if current(r).KeyID != "" {
		s.fail(w, r, domain.ErrForbidden)
		return
	}
	upload, err := s.service.UploadSettings(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	reply(w, 200, map[string]any{"name": "AssetHub", "version": "0.3.0", "phase": "content", "features": map[string]bool{"uploads": s.service.Ingestion().Enabled(), "content": s.service.Ingestion().ContentEnabled(), "projects": true, "api_keys": true, "invitations": current(r).IsAdmin(), "backups": s.service.Ingestion().BackupEnabled()}, "storage_provider": s.service.Ingestion().ProviderName(), "asset_limit_bytes": 512 << 20, "upload_part_bytes": 8 << 20, "storage_chunk_bytes": 16 << 20, "backup_retention_days": s.service.Ingestion().BackupRetentionDays(), "signed_url_seconds": 900, "encryption": "AES-256-GCM", "upload_concurrency": upload.Concurrency, "upload_request_timeout_seconds": upload.RequestTimeoutSeconds, "upload_response_timeout_seconds": upload.ResponseTimeoutSeconds, "upload_session_lifetime_hours": upload.SessionLifetimeHours, "editable": current(r).IsAdmin()})
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	if !current(r).IsAdmin() {
		s.fail(w, r, domain.ErrForbidden)
		return
	}
	var body struct {
		Concurrency     json.RawMessage `json:"upload_concurrency"`
		RequestTimeout  json.RawMessage `json:"upload_request_timeout_seconds"`
		ResponseTimeout json.RawMessage `json:"upload_response_timeout_seconds"`
		SessionLifetime json.RawMessage `json:"upload_session_lifetime_hours"`
	}
	if err := decode(w, r, &body); err != nil {
		s.fail(w, r, err)
		return
	}
	var patch domain.UploadSettingsPatch
	for _, field := range []struct {
		raw    json.RawMessage
		target **int
	}{
		{body.Concurrency, &patch.Concurrency}, {body.RequestTimeout, &patch.RequestTimeoutSeconds},
		{body.ResponseTimeout, &patch.ResponseTimeoutSeconds}, {body.SessionLifetime, &patch.SessionLifetimeHours},
	} {
		if len(field.raw) == 0 {
			continue
		}
		var value int
		if err := json.Unmarshal(field.raw, &value); err != nil {
			s.fail(w, r, domain.ErrInvalid)
			return
		}
		// JSON null decodes to zero, which the domain rejects for every setting.
		*field.target = &value
	}
	if err := s.service.UpdateUploadSettings(r.Context(), current(r), patch); err != nil {
		s.fail(w, r, err)
		return
	}
	s.settings(w, r)
}
func (s *Server) unavailable(w http.ResponseWriter, r *http.Request) {
	reply(w, 503, map[string]any{"error": map[string]string{"code": "feature_unavailable", "message": "This feature is unavailable in the current configuration or implementation phase"}})
}
