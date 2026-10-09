package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
