package integration

import (
	"assethub/internal/adapters/inbound/httpapi"
	"assethub/internal/adapters/outbound/cryptography"
	"assethub/internal/adapters/outbound/postgres"
	"assethub/internal/adapters/outbound/security"
	"assethub/internal/application/usecases"
	"assethub/internal/domain"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPasswordConfirmedKeySecrets(t *testing.T) {
	repo := isolatedRepository(t)
	ctx := context.Background()
	sec := security.Security{}
	crypto, _ := cryptography.New(bytes.Repeat([]byte{23}, 32))
	app := usecases.New(repo, sec, security.Clock{}, healthy{})
	app.SetKeySecrets(crypto)
	password := "integration-password-123"
	if err := app.Bootstrap(ctx, "keys@example.test", "Key manager", password); err != nil {
		t.Fatal(err)
	}
	user, token, csrf, err := app.Login(ctx, "keys@example.test", password)
	if err != nil {
		t.Fatal(err)
	}
	p := domain.Principal{User: user}
	project, err := app.CreateProject(ctx, p, "Key secrets", "", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	key, secret, err := app.CreateKey(ctx, p, project.ID, "Recoverable key", []string{"assets:read"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var cipher []byte
	if err = repo.Pool.QueryRow(ctx, "SELECT encrypted_secret FROM api_keys WHERE id=$1", key.ID).Scan(&cipher); err != nil || len(cipher) == 0 || bytes.Contains(cipher, []byte(secret)) {
		t.Fatal("secret not encrypted at rest", err)
	}
	keys, err := app.Keys(ctx, p, project.ID)
	if err != nil || len(keys) != 1 || !keys[0].SecretAvailable || len(keys[0].EncryptedSecret) != 0 || keys[0].Hash != "" {
		t.Fatal("unsafe metadata projection", err)
	}
	handler := httpapi.New(app, "http://localhost:8080", false, slog.New(slog.NewTextHandler(io.Discard, nil)))
	call := func(path, value, session, csrfToken, bearer, origin string) *httptest.ResponseRecorder {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"password": value})
		r := httptest.NewRequest("POST", path, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		if session != "" {
			r.Header.Set("Cookie", "ah_session="+session)
		}
		if csrfToken != "" {
			r.Header.Set("X-CSRF-Token", csrfToken)
		}
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	path := "/v1/api-keys/" + key.ID + "/reveal?project_id=" + project.ID
	check := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("expected %d, got %d: %s", status, w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("secret response may be cached")
		}
		if status != 200 && strings.Contains(w.Body.String(), secret) {
			t.Fatal("secret in rejected response")
		}
	}
	check(call(path, password, "", "", "", "http://localhost:8080"), 401)
	check(call(path, password, token, "", "", "http://localhost:8080"), 403)
	check(call(path, password, token, csrf, "", "http://untrusted.test"), 403)
	wrong := call(path, "incorrect", token, csrf, "", "http://localhost:8080")
	check(wrong, 403)
	if !strings.Contains(wrong.Body.String(), "incorrect_password") {
		t.Fatal("password rejection unclear")
	}
	w := call(path, password, token, csrf, "", "http://localhost:8080")
	check(w, 200)
	var recovered map[string]string
	if json.Unmarshal(w.Body.Bytes(), &recovered) != nil || recovered["secret"] != secret {
		t.Fatal("retrieved a different secret")
	}
	check(call(path, password, "", "", secret, "http://localhost:8080"), 403)
	otherProject, err := app.CreateProject(ctx, p, "Other project", "", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	check(call("/v1/api-keys/"+key.ID+"/reveal?project_id="+otherProject.ID, password, token, csrf, "", "http://localhost:8080"), 404)
	// A new pool and service must retrieve the same persistent secret using the installation key.
	other, err := postgres.Open(ctx, repo.Pool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Pool.Close()
	restarted := usecases.New(other, sec, security.Clock{}, healthy{})
	restarted.SetKeySecrets(crypto)
	if value, err := restarted.RevealKey(ctx, p, project.ID, key.ID, password); err != nil || value != secret {
		t.Fatal("secret did not survive service restart", err)
	}
	invitation, err := app.Invite(ctx, p, "key-viewer@example.test", "member")
	if err != nil {
		t.Fatal(err)
	}
	if err = app.Accept(ctx, invitation, "Key viewer", "viewer-password-123"); err != nil {
		t.Fatal(err)
	}
	_, viewerToken, viewerCSRF, err := app.Login(ctx, "key-viewer@example.test", "viewer-password-123")
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"viewer", "editor"} {
		if err = app.SetMember(ctx, p, project.ID, "key-viewer@example.test", role); err != nil {
			t.Fatal(err)
		}
		check(call(path, "viewer-password-123", viewerToken, viewerCSRF, "", "http://localhost:8080"), 403)
	}
	check(call(path, "viewer-password-123", token, csrf, "", "http://localhost:8080"), 403)
	if _, err = repo.Pool.Exec(ctx, "UPDATE api_keys SET encrypted_secret=NULL WHERE id=$1", key.ID); err != nil {
		t.Fatal(err)
	}
	check(call(path, password, token, csrf, "", "http://localhost:8080"), 409)
	keys, err = app.Keys(ctx, p, project.ID)
	if err != nil || keys[0].SecretAvailable {
		t.Fatal("legacy key shown as recoverable")
	}
	if _, err = repo.Pool.Exec(ctx, "UPDATE api_keys SET encrypted_secret=$2,expires_at=$3 WHERE id=$1", key.ID, cipher, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	check(call(path, password, token, csrf, "", "http://localhost:8080"), 404)
	if _, err = repo.Pool.Exec(ctx, "UPDATE api_keys SET expires_at=NULL WHERE id=$1", key.ID); err != nil {
		t.Fatal(err)
	}
	if err = app.RevokeKey(ctx, p, project.ID, key.ID); err != nil {
		t.Fatal(err)
	}
	check(call(path, password, token, csrf, "", "http://localhost:8080"), 404)
	var count int
	if err = repo.Pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE action='key.secret_accessed' AND object_id=$1", key.ID).Scan(&count); err != nil || count != 2 {
		t.Fatal("secret accesses not audited", count, err)
	}
	var leaked bool
	if err = repo.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM audit_events WHERE position($1 in detail)>0 OR position($2 in detail)>0)", secret, password).Scan(&leaked); err != nil || leaked {
		t.Fatal("audit contains credentials", err)
	}
}
