package integration

import (
	"assethub/internal/adapters/inbound/httpapi"
	"assethub/internal/adapters/outbound/cryptography"
	"assethub/internal/adapters/outbound/filesystem"
	"assethub/internal/adapters/outbound/redisqueue"
	"assethub/internal/adapters/outbound/security"
	"assethub/internal/adapters/outbound/validation"
	"assethub/internal/application/usecases"
	"assethub/internal/domain"
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestContentRangesCapabilitiesAndRecovery(t *testing.T) {
	raw := os.Getenv("TEST_REDIS_URL")
	if raw == "" {
		t.Skip("TEST_REDIS_URL required")
	}
	options, err := redis.ParseURL(raw)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(options)
	defer client.Close()
	ctx := context.Background()
	defer client.Del(ctx, "assethub:upload:pending")
	repo := isolatedRepository(t)
	sec := security.Security{}
	app := usecases.New(repo, sec, security.Clock{}, healthy{})
	if err = app.Bootstrap(ctx, "content@example.test", "Content Admin", "integration-password-123"); err != nil {
		t.Fatal(err)
	}
	user, _, _, err := app.Login(ctx, "content@example.test", "integration-password-123")
	if err != nil {
		t.Fatal(err)
	}
	p := domain.Principal{User: user}
	project, err := app.CreateProject(ctx, p, "Content project", "", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	zero := uint64(0)
	stage := &filesystem.Staging{Root: filepath.Join(root, "staging"), ReservePercent: &zero}
	backup := &filesystem.Backup{Root: filepath.Join(root, "backup"), StagingRoot: stage.Root, ReservePercent: &zero}
	os.MkdirAll(backup.Root, 0700)
	os.WriteFile(filepath.Join(backup.Root, ".assethub-backup"), []byte("fixture"), 0600)
	crypto, _ := cryptography.New(bytes.Repeat([]byte{8}, 32))
	app.SetKeySecrets(crypto)
	signer, _ := security.NewSigner(bytes.Repeat([]byte{4}, 32))
	primary := &filesystem.Local{Root: filepath.Join(root, "primary"), ReservePercent: &zero}
	cache := &filesystem.Cache{Root: filepath.Join(root, "cache"), Capacity: 64 << 20, ReservePercent: &zero}
	ingest := usecases.NewIngestion(app, repo, stage, crypto, primary, backup, &redisqueue.Queue{Client: client}, 20<<30, validation.FileInspector{})
	ingest.ConfigureContent(cache, signer)
	app.SetIngestion(ingest)
	data := bytes.Repeat([]byte("range-boundary-test:"), 900000)
	u, err := ingest.Create(ctx, p, project.ID, "movie.bin", int64(len(data)), checksum(data))
	if err != nil {
		t.Fatal(err)
	}
	for index, offset := 0, 0; offset < len(data); index, offset = index+1, offset+int(domain.UploadPartSize) {
		part := data[offset:min(offset+int(domain.UploadPartSize), len(data))]
		if err = ingest.PutPart(ctx, p, u.ID, index, checksum(part), bytes.NewReader(part)); err != nil {
			t.Fatal(err)
		}
	}
	a, err := ingest.Complete(ctx, p, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a worker crash after claiming; stale workers cannot write after lease recovery.
	stale := sec.ID()
	if _, err = repo.ClaimJob(ctx, a.ID, stale); err != nil {
		t.Fatal(err)
	}
	if worked, err := ingest.ProcessNext(ctx); !worked || err != nil {
		t.Fatal("duplicate claim not ignored")
	}
	repo.Pool.Exec(ctx, "UPDATE asset_jobs SET lease_until=now()-interval '1 second' WHERE asset_id=$1", a.ID)
	ingest.Dispatch(ctx)
	if _, err = ingest.ProcessNext(ctx); err != nil {
		t.Fatal(err)
	}
	if err = repo.SaveChunk(ctx, a.ID, stale, domain.Chunk{Index: 0}); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("stale lease accepted")
	}
	_, key, err := app.CreateKey(ctx, p, project.ID, "read content", []string{"assets:read", "assets:sign"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.New(app, "http://localhost:8080", false, slog.New(slog.NewTextHandler(io.Discard, nil)))
	get := func(method, path, rangeValue, secret string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		if rangeValue != "" {
			req.Header.Set("Range", rangeValue)
		}
		if secret != "" {
			req.Header.Set("Authorization", "Bearer "+secret)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}
	path := "/v1/assets/" + a.ID + "/content"
	if w := get("GET", path, "", ""); w.Code != 401 {
		t.Fatal("private content exposed", w.Code)
	}
	start := int(domain.StorageChunkSize) - 17
	end := start + 53
	w := get("GET", path, "bytes="+itoa(start)+"-"+itoa(end), key)
	if w.Code != 206 || !bytes.Equal(w.Body.Bytes(), data[start:end+1]) {
		t.Fatal("cross-chunk range incorrect", w.Code, w.Body.String())
	}
	if w = get("HEAD", path, "", key); w.Code != 200 || w.Body.Len() != 0 || w.Header().Get("Content-Length") != itoa(len(data)) {
		t.Fatal("HEAD incorrect")
	}
	if w = get("GET", path, "bytes=-11", key); w.Code != 206 || !bytes.Equal(w.Body.Bytes(), data[len(data)-11:]) {
		t.Fatal("suffix range incorrect")
	}
	if w = get("GET", path, "bytes="+itoa(len(data))+"-", key); w.Code != 416 || w.Header().Get("Content-Range") == "" {
		t.Fatal("invalid range accepted")
	}
	token, err := ingest.SignedLink(ctx, p, a.ID, 900)
	if err != nil {
		t.Fatal(err)
	}
	signed := path + "?token=" + token
	if w = get("GET", signed, "bytes=0-8", ""); w.Code != 206 {
		t.Fatal("signed capability rejected", w.Code)
	}
	expired, _ := signer.Sign(domain.ContentClaims{AssetID: a.ID, Version: 1, Expires: time.Now().Add(-time.Second).Unix()})
	if w = get("GET", path+"?token="+expired, "", ""); w.Code != 401 {
		t.Fatal("expired link accepted")
	}
	if _, err = ingest.SignedLink(ctx, p, a.ID, 86401); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("excessive lifetime accepted")
	}
	// Revoked API keys are denied even when their ciphertext has been cached.
	keys, _ := app.Keys(ctx, p, project.ID)
	if err = app.RevokeKey(ctx, p, project.ID, keys[0].ID); err != nil {
		t.Fatal(err)
	}
	if w = get("GET", path, "bytes=0-1", key); w.Code != 401 {
		t.Fatal("revoked key reached cache")
	}
	public := "public"
	if err = ingest.Update(ctx, p, a.ID, nil, &public); err != nil {
		t.Fatal(err)
	}
	if w = get("GET", path, "bytes=0-3", ""); w.Code != 206 {
		t.Fatal("public content unavailable")
	}
	if w = get("GET", signed, "bytes=0-1", ""); w.Code != 401 {
		t.Fatal("visibility change did not revoke links")
	}
	// Remove the primary and cache. Authenticated decryption must use the verified backup.
	stored, _ := repo.Asset(ctx, a.ID)
	for _, c := range stored.Chunks {
		ref, _ := crypto.OpenReference(a.ID, c.Index, c.Reference)
		if err = primary.Delete(ctx, ref); err != nil {
			t.Fatal(err)
		}
	}
	ingest.ConfigureContent(&filesystem.Cache{Root: filepath.Join(root, "empty-cache"), Capacity: 64 << 20, ReservePercent: &zero}, signer)
	if w = get("GET", path, "bytes=0-30", ""); w.Code != 206 || !bytes.Equal(w.Body.Bytes(), data[:31]) {
		t.Fatal("backup fallback failed", w.Code)
	}
	if err = ingest.Delete(ctx, p, a.ID); err != nil {
		t.Fatal(err)
	}
	if w = get("GET", path, "", ""); w.Code != 404 {
		t.Fatal("deleted asset still public")
	}
	ingest.Dispatch(ctx)
	if _, err = ingest.ProcessNext(ctx); err != nil {
		t.Fatal(err)
	}
	if err = ingest.Restore(ctx, p, a.ID); err != nil {
		t.Fatal("restore failed", err)
	}
	if w = get("GET", path, "bytes=0-16", ""); w.Code != 206 || !bytes.Equal(w.Body.Bytes(), data[:17]) {
		t.Fatal("restored bytes differ")
	}
	if w = get("GET", signed, "bytes=0-1", ""); w.Code != 401 {
		t.Fatal("old capability survived restore")
	}
	if err = ingest.Restore(ctx, domain.Principal{KeyID: sec.ID(), ProjectID: project.ID}, a.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("non-admin restore accepted")
	}
	if err = ingest.Delete(ctx, p, a.ID); err != nil {
		t.Fatal(err)
	}
	repo.Pool.Exec(ctx, "UPDATE assets SET deleted_at=now()-interval '31 days' WHERE id=$1", a.ID)
	if err = ingest.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(backup.Root, "assets", a.ID)); !os.IsNotExist(err) {
		t.Fatal("expired backup retained")
	}
	if err = ingest.Restore(ctx, p, a.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("expired backup restored")
	}
	if strings.Contains(w.Body.String(), "wrapped_key") {
		t.Fatal("internal metadata escaped")
	}
}
func itoa(n int) string { return fmt.Sprint(n) }
