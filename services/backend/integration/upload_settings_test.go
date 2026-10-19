package integration

import (
	"assethub/internal/adapters/inbound/httpapi"
	"assethub/internal/adapters/outbound/cryptography"
	"assethub/internal/adapters/outbound/filesystem"
	"assethub/internal/adapters/outbound/postgres"
	"assethub/internal/adapters/outbound/security"
	"assethub/internal/adapters/outbound/validation"
	"assethub/internal/application/usecases"
	"assethub/internal/domain"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type heldUploadBody struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	reader  io.Reader
}

func (b *heldUploadBody) Read(p []byte) (int, error) {
	b.once.Do(func() { close(b.entered); <-b.release })
	return b.reader.Read(p)
}

func TestUploadSettingsAndConcurrentRequests(t *testing.T) {
	repo := isolatedRepository(t)
	ctx := context.Background()
	app := usecases.New(repo, security.Security{}, security.Clock{}, healthy{})
	crypto, _ := cryptography.New(bytes.Repeat([]byte{18}, 32))
	app.SetKeySecrets(crypto)
	if err := app.Bootstrap(ctx, "uploads-admin@example.test", "Upload admin", "integration-password-123"); err != nil {
		t.Fatal(err)
	}
	user, token, csrf, err := app.Login(ctx, "uploads-admin@example.test", "integration-password-123")
	if err != nil {
		t.Fatal(err)
	}
	p := domain.Principal{User: user}
	project, err := app.CreateProject(ctx, p, "Concurrent uploads", "", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	_, key, err := app.CreateKey(ctx, p, project.ID, "Uploader", []string{"assets:read", "assets:write"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	reserve := uint64(0)
	root := t.TempDir()
	stage := &filesystem.Staging{Root: filepath.Join(root, "staging"), ReservePercent: &reserve}
	primary := &filesystem.Local{Root: filepath.Join(root, "primary"), ReservePercent: &reserve}
	ingest := usecases.NewIngestion(app, repo, stage, crypto, primary, nil, nil, 1<<30, validation.FileInspector{})
	app.SetIngestion(ingest)
	handler := httpapi.New(app, "http://localhost:8080", false, slog.New(slog.NewTextHandler(io.Discard, nil)))
	call := func(method, body, session, csrfToken, origin, bearer string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/v1/settings", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		r.Header.Set("X-CSRF-Token", csrfToken)
		if session != "" {
			r.Header.Set("Cookie", "ah_session="+session)
		}
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	check := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("expected %d, got %d: %s", status, w.Code, w.Body.String())
		}
	}
	settings := call("GET", "", token, "", "", "")
	check(settings, 200)
	var view struct {
		Limit    int  `json:"upload_concurrency"`
		Editable bool `json:"editable"`
		Request  int  `json:"upload_request_timeout_seconds"`
		Response int  `json:"upload_response_timeout_seconds"`
		Lifetime int  `json:"upload_session_lifetime_hours"`
	}
	if err = json.Unmarshal(settings.Body.Bytes(), &view); err != nil || view.Limit != 20 || !view.Editable || view.Request != 120 || view.Response != 150 || view.Lifetime != 24 {
		t.Fatal("incorrect defaults", view, err)
	}
	for _, body := range []string{`{}`, `{"upload_concurrency":0}`, `{"upload_concurrency":21}`, `{"upload_concurrency":-1}`, `{"upload_concurrency":1.5}`, `{"upload_concurrency":null}`, `{"upload_concurrency":"20"}`, `{"upload_concurrency":20,"unknown":true}`} {
		check(call("PATCH", body, token, csrf, "http://localhost:8080", ""), 400)
	}
	for _, body := range []string{
		`{"upload_request_timeout_seconds":29}`, `{"upload_request_timeout_seconds":1801}`, `{"upload_request_timeout_seconds":null}`,
		`{"upload_response_timeout_seconds":59}`, `{"upload_response_timeout_seconds":1831}`, `{"upload_response_timeout_seconds":149}`,
		`{"upload_session_lifetime_hours":0}`, `{"upload_session_lifetime_hours":169}`, `{"upload_session_lifetime_hours":1.5}`, `{"upload_session_lifetime_hours":null}`,
		`{"upload_request_timeout_seconds":300,"upload_response_timeout_seconds":320}`, `{"upload_response_timeout_seconds":true}`,
	} {
		check(call("PATCH", body, token, csrf, "http://localhost:8080", ""), 400)
	}
	check(call("PATCH", `{"upload_concurrency":1}`, "", "", "", ""), 401)
	check(call("PATCH", `{"upload_concurrency":1}`, token, "wrong", "http://localhost:8080", ""), 403)
	check(call("PATCH", `{"upload_concurrency":1}`, token, csrf, "http://other.example.test", ""), 403)
	check(call("PATCH", `{"upload_concurrency":1}`, "", "", "", key), 403)
	invite, err := app.Invite(ctx, p, "uploads-member@example.test", "member")
	if err != nil {
		t.Fatal(err)
	}
	if err = app.Accept(ctx, invite, "Member", "integration-password-456"); err != nil {
		t.Fatal(err)
	}
	_, memberToken, memberCSRF, err := app.Login(ctx, "uploads-member@example.test", "integration-password-456")
	if err != nil {
		t.Fatal(err)
	}
	check(call("PATCH", `{"upload_concurrency":1}`, memberToken, memberCSRF, "http://localhost:8080", ""), 403)
	memberView := call("GET", "", memberToken, "", "", "")
	check(memberView, 200)
	if err = json.Unmarshal(memberView.Body.Bytes(), &view); err != nil || view.Editable || view.Limit != 20 {
		t.Fatal("member permissions", view, err)
	}
	setLimit := func(limit int) {
		t.Helper()
		check(call("PATCH", fmt.Sprintf(`{"upload_concurrency":%d}`, limit), token, csrf, "http://localhost:8080", ""), 200)
	}
	setLimit(2)
	// A new connection and application instance must read the saved setting.
	reopened, err := postgres.Open(ctx, repo.Pool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Pool.Close()
	restarted := usecases.New(reopened, security.Security{}, security.Clock{}, healthy{})
	if saved, e := restarted.UploadSettings(ctx); e != nil || saved.Concurrency != 2 {
		t.Fatal("setting lost after restart", saved, e)
	}
	var auditCount int
	if err = reopened.Pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE action='settings.updated'").Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatal("missing or unauthorized setting audit", auditCount, err)
	}
	patch := func(body string) {
		t.Helper()
		check(call("PATCH", body, token, csrf, "http://localhost:8080", ""), 200)
	}
	oldSession, err := ingest.Create(ctx, p, project.ID, "original-lifetime.txt", 1, "")
	if err != nil {
		t.Fatal(err)
	}
	// Compare persisted timestamps to persisted timestamps: PostgreSQL stores
	// microseconds and supplies created_at when it inserts the reservation.
	oldSession, err = ingest.Get(ctx, p, oldSession.ID, "read")
	if err != nil {
		t.Fatal(err)
	}
	patch(`{"upload_request_timeout_seconds":300,"upload_response_timeout_seconds":360,"upload_session_lifetime_hours":48}`)
	if saved, e := restarted.UploadSettings(ctx); e != nil || saved.Concurrency != 2 || saved.RequestTimeoutSeconds != 300 || saved.ResponseTimeoutSeconds != 360 || saved.SessionLifetimeHours != 48 {
		t.Fatal("timeout persistence or partial patch", saved, e)
	}
	newSession, err := ingest.Create(ctx, p, project.ID, "extended-lifetime.txt", 1, "")
	if err != nil || newSession.ExpiresAt.Sub(newSession.CreatedAt).Round(time.Second) != 48*time.Hour {
		t.Fatal("session lifetime not applied", newSession, err)
	}
	unchanged, err := ingest.Get(ctx, p, oldSession.ID, "read")
	if err != nil || !unchanged.ExpiresAt.Equal(oldSession.ExpiresAt) {
		t.Fatal("existing expiry changed", err)
	}
	patch(`{"upload_request_timeout_seconds":30,"upload_response_timeout_seconds":60,"upload_session_lifetime_hours":1}`)
	patch(`{"upload_request_timeout_seconds":1800,"upload_response_timeout_seconds":1830,"upload_session_lifetime_hours":168}`)
	// Concurrent partial patches serialize against the current row instead of
	// overwriting another administrator's unrelated fields.
	var updates sync.WaitGroup
	for _, values := range []domain.UploadSettingsPatch{{Concurrency: new(3)}, {SessionLifetimeHours: new(24)}} {
		updates.Go(func() {
			if err := app.UpdateUploadSettings(ctx, p, values); err != nil {
				t.Error(err)
			}
		})
	}
	updates.Wait()
	if saved, e := restarted.UploadSettings(ctx); e != nil || saved.Concurrency != 3 || saved.SessionLifetimeHours != 24 {
		t.Fatal("concurrent partial settings lost", saved, e)
	}
	setLimit(2)
	patch(`{"upload_request_timeout_seconds":300,"upload_response_timeout_seconds":360}`)
	uploads := make([]domain.Upload, 21)
	for index := range uploads {
		uploads[index], err = ingest.Create(ctx, p, project.ID, fmt.Sprintf("image-%d.txt", index), 1, "")
		if err != nil {
			t.Fatal(err)
		}
	}
	put := func(index int, body io.Reader) *httptest.ResponseRecorder {
		r := httptest.NewRequest("PUT", "/v1/uploads/"+uploads[index].ID+"/parts/0", body)
		r.Header.Set("Authorization", "Bearer "+key)
		r.Header.Set("X-Content-SHA256", checksum([]byte("x")))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	var releases []func()
	var completions []chan *httptest.ResponseRecorder
	begin := func(index int) {
		t.Helper()
		body := &heldUploadBody{entered: make(chan struct{}), release: make(chan struct{}), reader: strings.NewReader("x")}
		var once sync.Once
		release := func() { once.Do(func() { close(body.release) }) }
		t.Cleanup(release)
		releases = append(releases, release)
		done := make(chan *httptest.ResponseRecorder, 1)
		completions = append(completions, done)
		go func() { done <- put(index, body) }()
		select {
		case <-body.entered:
		case response := <-done:
			t.Fatalf("upload %d rejected: %d %s", index, response.Code, response.Body.String())
		case <-time.After(10 * time.Second):
			t.Fatal("upload did not start")
		}
	}
	busy := func() {
		t.Helper()
		w := put(20, strings.NewReader("x"))
		check(w, 429)
		if w.Header().Get("Retry-After") != "2" {
			t.Fatal("missing retry delay")
		}
	}
	begin(0)
	begin(1)
	var originalDeadline time.Time
	if err = repo.Pool.QueryRow(ctx, "SELECT transfer_until FROM uploads WHERE id=$1", uploads[0].ID).Scan(&originalDeadline); err != nil || time.Until(originalDeadline) < 375*time.Second {
		t.Fatal("configured deadline not recorded", originalDeadline, err)
	}
	patch(`{"upload_request_timeout_seconds":30,"upload_response_timeout_seconds":60}`)
	busy()
	// Increase while transfers are held open: all twenty must overlap.
	setLimit(20)
	for index := 2; index < 20; index++ {
		begin(index)
	}
	busy()
	// Lowering never interrupts active work and admits no replacement early.
	setLimit(1)
	busy()
	for index := 0; index < 19; index++ {
		releases[index]()
		select {
		case w := <-completions[index]:
			check(w, 200)
		case <-time.After(10 * time.Second):
			t.Fatal("upload did not finish")
		}
	}
	busy()
	releases[19]()
	select {
	case w := <-completions[19]:
		check(w, 200)
	case <-time.After(10 * time.Second):
		t.Fatal("last upload did not finish")
	}
	check(put(20, strings.NewReader("x")), 200)
	if err = ingest.Cancel(ctx, p, uploads[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(stage.Root, uploads[0].ID)); err != nil {
		t.Fatal("cancellation removed staging before its guarded deadline", err)
	}
	if _, err = repo.Pool.Exec(ctx, "UPDATE uploads SET ended_at=now()-interval '4 minutes' WHERE id=$1", uploads[0].ID); err != nil {
		t.Fatal(err)
	}
	cleanable, err := repo.CleanupUploads(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range cleanable {
		if id == uploads[0].ID {
			t.Fatal("cleanup ignored the older transfer deadline after lowering timeouts")
		}
	}
	if _, err = repo.Pool.Exec(ctx, "UPDATE uploads SET transfer_until=now()-interval '1 second' WHERE id=$1", uploads[0].ID); err != nil {
		t.Fatal(err)
	}
	cleanable, err = repo.CleanupUploads(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, id := range cleanable {
		if id == uploads[0].ID {
			found = true
		}
	}
	if !found {
		t.Fatal("staging never becomes eligible after transfer deadline")
	}
}
