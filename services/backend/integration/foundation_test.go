package integration

import (
	"assethub/internal/adapters/inbound/httpapi"
	"assethub/internal/adapters/outbound/cryptography"
	"assethub/internal/adapters/outbound/postgres"
	"assethub/internal/adapters/outbound/security"
	"assethub/internal/application/usecases"
	"assethub/internal/domain"
	"assethub/migrations"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

type healthy struct{}

func (healthy) Check(context.Context) []domain.Health { return []domain.Health{} }
func (healthy) Ready(context.Context) error           { return nil }
func TestPersistentFoundation(t *testing.T) {
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL required; integration test uses a fresh dedicated database")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, base)
	if err != nil {
		t.Fatal("invalid integration database configuration")
	}
	defer admin.Close()
	sec := security.Security{}
	name := "assethub_it_" + strings.ReplaceAll(sec.ID(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal("cannot create isolated integration database")
	}
	defer func() {
		if _, err := admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Error("isolated database cleanup failed")
		}
	}()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal("invalid integration configuration")
	}
	u.Path = "/" + name
	databaseURL := u.String()
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrations.Files)
	if err = goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpContext(ctx, db, "."); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpContext(ctx, db, "."); err != nil {
		t.Fatal("migration was not idempotent", err)
	}
	repo, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Pool.Close()
	app := usecases.New(repo, sec, security.Clock{}, healthy{})
	crypto, _ := cryptography.New(bytes.Repeat([]byte{5}, 32))
	app.SetKeySecrets(crypto)
	if err = app.Bootstrap(ctx, "admin@example.test", "Test Administrator", "integration-password-123"); err != nil {
		t.Fatal(err)
	}
	if err = app.Bootstrap(ctx, "second@example.test", "Second Admin", "integration-password-123"); err == nil {
		t.Fatal("second bootstrap permitted")
	}
	handler := httpapi.New(app, "http://localhost:8080", false, slog.New(slog.NewTextHandler(io.Discard, nil)))
	call := func(method, path string, body any, cookie, csrf, key string) (int, map[string]any, *httptest.ResponseRecorder) {
		t.Helper()
		var reader io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			reader = strings.NewReader(string(b))
		}
		req := httptest.NewRequest(method, path, reader)
		req.RemoteAddr = "127.0.0.1:12000"
		req.Header.Set("Origin", "http://localhost:8080")
		req.Header.Set("Content-Type", "application/json")
		if cookie != "" {
			req.Header.Set("Cookie", cookie)
		}
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		var result map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &result)
		return w.Code, result, w
	}
	status, auth, w := call("POST", "/v1/auth/login", map[string]string{"email": "admin@example.test", "password": "integration-password-123"}, "", "", "")
	if status != 200 {
		t.Fatal("login", status, w.Body.String())
	}
	cookie := strings.Split(w.Header().Get("Set-Cookie"), ";")[0]
	csrf := auth["csrf_token"].(string)
	if !strings.Contains(w.Header().Get("Set-Cookie"), "HttpOnly") || !strings.Contains(w.Header().Get("Set-Cookie"), "SameSite=Strict") {
		t.Fatal("unsafe session cookie")
	}
	status, _, _ = call("POST", "/v1/projects", map[string]any{"name": "Forbidden", "description": "", "quota_bytes": 1 << 30}, cookie, "", "")
	if status != 403 {
		t.Fatal("missing CSRF accepted", status)
	}
	status, project, w := call("POST", "/v1/projects", map[string]any{"name": "Website", "description": "Real persistent project", "quota_bytes": int64(10) << 30}, cookie, csrf, "")
	if status != 201 {
		t.Fatal("project", status, w.Body.String())
	}
	id := project["id"].(string)
	status, summary, w := call("GET", "/v1/overview", nil, cookie, "", "")
	if status != 200 || summary["projects"] != float64(1) || summary["files"] != float64(0) {
		t.Fatal("overview", status, w.Body.String())
	}
	status, key, w := call("POST", "/v1/api-keys", map[string]any{"project_id": id, "name": "Application", "scopes": []string{"assets:read"}}, cookie, csrf, "")
	if status != 201 {
		t.Fatal("key", status, w.Body.String())
	}
	secret := key["secret"].(string)
	kid := key["key"].(map[string]any)["id"].(string)
	status, _, _ = call("GET", "/v1/assets?project_id="+id, nil, "", "", secret)
	if status != 200 {
		t.Fatal("key read denied", status)
	}
	status, _, _ = call("POST", "/v1/projects", map[string]any{"name": "Key project", "description": "", "quota_bytes": 1 << 30}, "", "", secret)
	if status != 403 {
		t.Fatal("key created project", status)
	}
	status, _, _ = call("GET", "/v1/assets?project_id="+sec.ID(), nil, "", "", secret)
	if status != 404 {
		t.Fatal("key crossed project boundary", status)
	}
	status, _, _ = call("GET", "/v1/api-keys?project_id="+id, nil, "", "", secret)
	if status != 403 {
		t.Fatal("key administered credentials", status)
	}
	status, invitation, w := call("POST", "/v1/invitations", map[string]string{"email": "viewer@example.test", "role": "member"}, cookie, csrf, "")
	if status != 201 {
		t.Fatal("invite", status, w.Body.String())
	}
	token := strings.Split(invitation["url"].(string), "token=")[1]
	accept := map[string]string{"token": token, "name": "Viewer", "password": "viewer-password-123"}
	status, _, w = call("POST", "/v1/auth/accept-invitation", accept, "", "", "")
	if status != 201 {
		t.Fatal("accept", status, w.Body.String())
	}
	status, _, _ = call("POST", "/v1/auth/accept-invitation", accept, "", "", "")
	if status != 409 {
		t.Fatal("reused invitation", status)
	}
	status, viewer, vw := call("POST", "/v1/auth/login", map[string]string{"email": "viewer@example.test", "password": "viewer-password-123"}, "", "", "")
	if status != 200 {
		t.Fatal("viewer login", status)
	}
	vc := strings.Split(vw.Header().Get("Set-Cookie"), ";")[0]
	vcsrf := viewer["csrf_token"].(string)
	status, list, _ := call("GET", "/v1/projects", nil, vc, "", "")
	if status != 200 || len(list["items"].([]any)) != 0 {
		t.Fatal("unassigned viewer saw project", status)
	}
	status, filteredBeforeMembership, _ := call("GET", "/v1/activity?q=admin&project_id="+id, nil, vc, "", "")
	if status != 200 || len(filteredBeforeMembership["items"].([]any)) != 0 {
		t.Fatal("audit filtering exposed an unassigned project", status, filteredBeforeMembership)
	}
	status, _, _ = call("POST", "/v1/projects/"+id+"/members", map[string]string{"email": "viewer@example.test", "role": "viewer"}, cookie, csrf, "")
	if status != 200 {
		t.Fatal("assign viewer", status)
	}
	status, list, _ = call("GET", "/v1/projects", nil, vc, "", "")
	if status != 200 || len(list["items"].([]any)) != 1 {
		t.Fatal("membership missing", status)
	}
	status, _, _ = call("PATCH", "/v1/projects/"+id, map[string]any{"name": "Stolen", "description": "", "quota_bytes": 1 << 30}, vc, vcsrf, "")
	if status != 403 {
		t.Fatal("viewer edited project", status)
	}
	status, _, _ = call("GET", "/v1/storage/health", nil, vc, "", "")
	if status != 403 {
		t.Fatal("viewer accessed installation health", status)
	}
	status, _, _ = call("POST", "/v1/projects/"+id+"/members", map[string]string{"email": "admin@example.test", "role": "viewer"}, cookie, csrf, "")
	if status != 409 {
		t.Fatal("last manager removed", status)
	}
	status, _, _ = call("DELETE", "/v1/api-keys/"+kid+"?project_id="+id, nil, cookie, csrf, "")
	if status != 200 {
		t.Fatal("revoke", status)
	}
	status, _, _ = call("GET", "/v1/assets", nil, "", "", secret)
	if status != 401 {
		t.Fatal("revoked key still worked", status)
	}
	status, events, _ := call("GET", "/v1/activity", nil, cookie, "", "")
	if status != 200 || len(events["items"].([]any)) < 8 {
		t.Fatal("missing audit events", status)
	}
	status, filtered, _ := call("GET", "/v1/activity?q=APPLICATION&action=key.&project_id="+id+"&limit=1", nil, cookie, "", "")
	if status != 200 || len(filtered["items"].([]any)) != 1 {
		t.Fatal("audit filters before pagination", status, filtered)
	}
	status, filtered, _ = call("GET", "/v1/activity?action=key.revoked&project_id="+id+"&limit=1&offset=1", nil, cookie, "", "")
	if status != 200 || len(filtered["items"].([]any)) != 0 {
		t.Fatal("audit offset applies to matching records", status, filtered)
	}
	status, filtered, _ = call("GET", "/v1/activity?action=key.&project_id="+id, nil, vc, "", "")
	if status != 200 || len(filtered["items"].([]any)) != 2 {
		t.Fatal("authorized member audit filters", status, filtered)
	}
	status, filtered, _ = call("GET", "/v1/activity?q=admin&action=auth.&project_id="+sec.ID(), nil, vc, "", "")
	if status != 200 || len(filtered["items"].([]any)) != 0 {
		t.Fatal("audit filters bypassed authorization", status, filtered)
	}
	status, _, _ = call("POST", "/v1/assets", map[string]any{}, cookie, csrf, "")
	if status != 503 {
		t.Fatal("unimplemented upload not explicit", status)
	}
	status, _, _ = call("POST", "/v1/auth/logout", nil, cookie, csrf, "")
	if status != 200 {
		t.Fatal("logout", status)
	}
	status, _, _ = call("GET", "/v1/auth/me", nil, cookie, "", "")
	if status != 401 {
		t.Fatal("session survived logout", status)
	}
	// A fresh pool verifies state persisted beyond the repository instance.
	other, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Pool.Close()
	user, err := other.UserByEmail(ctx, "admin@example.test")
	if err != nil {
		t.Fatal(err)
	}
	ps, err := other.Projects(ctx, domain.Principal{User: user})
	if err != nil || len(ps) != 1 {
		t.Fatal("project persistence", err)
	}
}
