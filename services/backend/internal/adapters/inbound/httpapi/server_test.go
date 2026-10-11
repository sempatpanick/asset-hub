package httpapi

import (
	"assethub/internal/domain"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPasswordConfirmationRateLimitPerAccount(t *testing.T) {
	s := &Server{limiter: &limiter{items: map[string]bucket{}, active: make(chan struct{}, 2)}}
	handler := s.authLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	for i := range 11 {
		r := httptest.NewRequest("POST", "/v1/api-keys/key/reveal", nil)
		r.RemoteAddr = "127.0.0.1:1000"
		if i%2 == 0 {
			r.RemoteAddr = "127.0.0.2:1000"
		}
		r = r.WithContext(context.WithValue(r.Context(), principalKey{}, domain.Principal{User: domain.User{ID: "account"}}))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if i < 10 && w.Code != 200 {
			t.Fatal("confirmation prematurely limited", i, w.Code)
		}
		if i == 10 && (w.Code != 429 || w.Header().Get("Retry-After") != "60") {
			t.Fatal("password guesses not bounded")
		}
	}
	r := httptest.NewRequest("POST", "/v1/api-keys/key/reveal", nil)
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, domain.Principal{User: domain.User{ID: "other-account"}}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal("another account was limited")
	}
}

func TestOriginRejectedBeforeLogin(t *testing.T) {
	h := New(nil, "https://assets.example.test", true, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest("POST", "/v1/auth/login", strings.NewReader(`{"email":"x@example.test","password":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://untrusted.example.test")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatal(w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing cache protection")
	}
}
func TestMalformedAuthorization(t *testing.T) {
	h := New(nil, "https://assets.example.test", true, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest("GET", "/v1/projects", nil)
	req.Header.Set("Authorization", "Basic token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}
func TestUnauthenticatedMetadataDenied(t *testing.T) {
	h := New(nil, "https://assets.example.test", true, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, path := range []string{"/v1/projects", "/v1/assets", "/v1/activity", "/v1/storage/health", "/v1/api-keys"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 401 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
}
