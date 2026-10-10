package integration

import (
	"assethub/internal/adapters/inbound/httpapi"
	"assethub/internal/adapters/outbound/cryptography"
	"assethub/internal/adapters/outbound/filesystem"
	"assethub/internal/adapters/outbound/postgres"
	"assethub/internal/adapters/outbound/redisqueue"
	"assethub/internal/adapters/outbound/security"
	"assethub/internal/adapters/outbound/validation"
	"assethub/internal/application/usecases"
	"assethub/internal/domain"
	"assethub/migrations"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
	"github.com/redis/go-redis/v9"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type failingPrimary struct {
	filesystem.Local
	FailAt  int
	Puts    int
	Corrupt bool
}

func (p *failingPrimary) Put(ctx context.Context, name string, data []byte) ([]byte, error) {
	p.Puts++
	if p.FailAt > 0 && p.Puts == p.FailAt {
		return nil, &domain.StorageError{Code: "provider_retry", RetryAfter: time.Second}
	}
	return p.Local.Put(ctx, name, data)
}
func (p *failingPrimary) Open(ctx context.Context, ref []byte) ([]byte, error) {
	data, err := p.Local.Open(ctx, ref)
	if p.Corrupt && len(data) > 0 {
		data[0] ^= 1
	}
	return data, err
}

func isolatedRepository(t *testing.T) *postgres.Repository {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	name := "assethub_ingest_" + strings.ReplaceAll((security.Security{}).ID(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal("cannot create test database")
	}
	t.Cleanup(func() {
		_, err := admin.Exec(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)")
		if err != nil {
			t.Error("database cleanup failed")
		}
		admin.Close()
	})
	u, _ := url.Parse(base)
	u.Path = "/" + name
	db, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	goose.SetBaseFS(migrations.Files)
	goose.SetDialect("postgres")
	if err = goose.UpContext(ctx, db, "."); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	repo, err := postgres.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(repo.Pool.Close)
	return repo
}
func checksum(data []byte) string { hash := sha256.Sum256(data); return hex.EncodeToString(hash[:]) }

func TestDurableIngestion(t *testing.T) {
	rawRedis := os.Getenv("TEST_REDIS_URL")
	if rawRedis == "" {
		t.Skip("TEST_REDIS_URL required for real queue recovery tests")
	}
	options, err := redis.ParseURL(rawRedis)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(options)
	defer client.Close()
	ctx := context.Background()
	if err = client.Ping(ctx).Err(); err != nil {
		t.Fatal("test Redis unavailable")
	}
	// Only the dedicated TEST_REDIS_URL instance is used; never flush Redis or unrelated keys.
	defer client.Del(ctx, "assethub:upload:pending")
	repo := isolatedRepository(t)
	sec := security.Security{}
	app := usecases.New(repo, sec, security.Clock{}, healthy{})
	if err = app.Bootstrap(ctx, "upload@example.test", "Upload Admin", "integration-password-123"); err != nil {
		t.Fatal(err)
	}
	user, _, _, err := app.Login(ctx, "upload@example.test", "integration-password-123")
	if err != nil {
		t.Fatal(err)
	}
	p := domain.Principal{User: user}
	project, err := app.CreateProject(ctx, p, "Upload integration", "", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	backupRoot := filepath.Join(root, "backup")
	os.MkdirAll(backupRoot, 0700)
	marker := filepath.Join(backupRoot, ".assethub-backup")
	os.WriteFile(marker, []byte("development fixture"), 0600)
	reservePercent := uint64(0) // Development fixture on a shared Windows disk; production defaults to a 10% reserve.
	stage := &filesystem.Staging{ReservePercent: &reservePercent, Root: filepath.Join(root, "staging")}
	backup := &filesystem.Backup{Root: backupRoot, StagingRoot: stage.Root, ReservePercent: &reservePercent}
	crypto, _ := cryptography.New(bytes.Repeat([]byte{6}, 32))
	primary := &failingPrimary{Local: filesystem.Local{ReservePercent: &reservePercent, Root: filepath.Join(root, "primary")}}
	queue := &redisqueue.Queue{Client: client}
	ingest := usecases.NewIngestion(app, repo, stage, crypto, primary, backup, queue, 2<<30, validation.FileInspector{})
	app.SetIngestion(ingest)
	data := bytes.Repeat([]byte("binary-content!"), int(domain.StorageChunkSize)/15+100)
	data = append(data, []byte("tail")...)
	upload, err := ingest.Create(ctx, p, project.ID, "archive.bin", int64(len(data)), checksum(data))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ingest.Complete(ctx, p, upload.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("partial upload completed", err)
	}
	for index, offset := 0, 0; offset < len(data); index, offset = index+1, offset+int(domain.UploadPartSize) {
		end := min(offset+int(domain.UploadPartSize), len(data))
		part := data[offset:end]
		if err = ingest.PutPart(ctx, p, upload.ID, index, checksum(part), bytes.NewReader(part)); err != nil {
			t.Fatal(err)
		}
		if err = ingest.PutPart(ctx, p, upload.ID, index, checksum(part), bytes.NewReader(part)); err != nil {
			t.Fatal("identical retry failed", err)
		}
	}
	different := bytes.Repeat([]byte{3}, int(domain.UploadPartSize))
	if err = ingest.PutPart(ctx, p, upload.ID, 0, checksum(different), bytes.NewReader(different)); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("different retry accepted")
	}
	asset, err := ingest.Complete(ctx, p, upload.ID)
	if err != nil {
		t.Fatal(err)
	}
	again, err := ingest.Complete(ctx, p, upload.ID)
	if err != nil || again.ID != asset.ID {
		t.Fatal("completion not idempotent")
	}
	// Redis loss between commit and execution must recover from PostgreSQL.
	client.Del(ctx, "assethub:upload:pending")
	if err = ingest.Dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	primary.FailAt = 2
	if worked, err := ingest.ProcessNext(ctx); !worked || err != nil {
		t.Fatal("first processing failed", err)
	}
	stored, err := repo.Asset(ctx, asset.ID)
	if err != nil || stored.Status != "queued" || stored.ErrorCode != "provider_retry" || len(stored.Chunks) != 2 {
		t.Fatal("retry checkpoint missing", stored.Status, err)
	}
	if _, err = repo.Pool.Exec(ctx, "UPDATE asset_jobs SET next_attempt=now() WHERE asset_id=$1", asset.ID); err != nil {
		t.Fatal(err)
	}
	if err = ingest.Dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = ingest.ProcessNext(ctx); err != nil {
		t.Fatal(err)
	}
	stored, err = repo.Asset(ctx, asset.ID)
	if err != nil || stored.Status != "ready" || primary.Puts != 3 {
		t.Fatal("completed chunk was uploaded again", stored.Status, primary.Puts, err)
	}
	key, _ := crypto.UnwrapKey(asset.ID, stored.WrappedKey)
	recovered := []byte{}
	for _, chunk := range stored.Chunks {
		raw, err := crypto.OpenReference(asset.ID, chunk.Index, chunk.Reference)
		if err != nil {
			t.Fatal(err)
		}
		cipher, err := primary.Open(ctx, raw)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(cipher, []byte("binary-content!")) {
			t.Fatal("plaintext exposed")
		}
		copy, err := backup.OpenChunk(asset.ID, chunk)
		if err != nil || !bytes.Equal(copy, cipher) {
			t.Fatal("backup differs")
		}
		plain, err := crypto.Decrypt(asset.ID, chunk.Index, key, copy)
		if err != nil {
			t.Fatal(err)
		}
		recovered = append(recovered, plain...)
	}
	if !bytes.Equal(recovered, data) {
		t.Fatal("chunk boundary byte integrity failed")
	}
	if info, err := os.Stat(filepath.Join(backupRoot, "assets", asset.ID, "manifest.enc")); err != nil || info.Size() < 1 {
		t.Fatal("recovery manifest missing")
	}
	repo.Pool.Exec(ctx, "UPDATE uploads SET ended_at=now()-interval '4 minutes' WHERE id=$1", upload.ID)
	if err = ingest.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(stage.Root, upload.ID)); !os.IsNotExist(err) {
		t.Fatal("ready plaintext staging retained")
	}
	// A missing independent backup prevents acceptance and readiness.
	os.Remove(marker)
	if _, err = ingest.Create(ctx, p, project.ID, "blocked", 3, ""); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("missing backup accepted")
	}
	os.WriteFile(marker, []byte("fixture"), 0600)
	small := []byte("a small file")
	u, err := ingest.Create(ctx, p, project.ID, "small.txt", int64(len(small)), "")
	if err != nil {
		t.Fatal(err)
	}
	ingest.PutPart(ctx, p, u.ID, 0, checksum(small), bytes.NewReader(small))
	a, err := ingest.Complete(ctx, p, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(marker)
	ingest.ProcessNext(ctx)
	s, _ := repo.Asset(ctx, a.ID)
	if s.Status == "ready" || s.ErrorCode != "backup_unavailable" {
		t.Fatal("asset ready without backup", s.Status, s.ErrorCode)
	}
	os.WriteFile(marker, []byte("fixture"), 0600)
	repo.Pool.Exec(ctx, "UPDATE asset_jobs SET next_attempt=now() WHERE asset_id=$1", a.ID)
	primary.Corrupt = true
	ingest.Dispatch(ctx)
	ingest.ProcessNext(ctx)
	s, _ = repo.Asset(ctx, a.ID)
	if s.Status != "failed" || s.ErrorCode != "integrity_failed" {
		t.Fatal("corrupted primary was accepted")
	}
	primary.Corrupt = false
	if err = ingest.Retry(ctx, p, a.ID); err != nil {
		t.Fatal(err)
	}
	ingest.Dispatch(ctx)
	ingest.ProcessNext(ctx)
	s, _ = repo.Asset(ctx, a.ID)
	if s.Status != "ready" {
		t.Fatal("failed asset retry did not recover")
	}
	// Limits, quota reservations, cancellation and project scope are enforced server-side.
	quotaProject, _ := app.CreateProject(ctx, p, "Quota", "", 1<<20)
	reserved, err := ingest.Create(ctx, p, quotaProject.ID, "reserved", 1<<20, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ingest.Create(ctx, p, quotaProject.ID, "overflow", 1, ""); !errors.Is(err, domain.ErrCapacity) {
		t.Fatal("reserved quota ignored")
	}
	if err = ingest.Cancel(ctx, p, reserved.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = ingest.Create(ctx, p, quotaProject.ID, "released", 1, ""); err != nil {
		t.Fatal("cancel did not release quota")
	}
	foreign := domain.Principal{KeyID: sec.ID(), ProjectID: quotaProject.ID, Scopes: []string{"assets:read", "assets:write"}}
	if _, err = ingest.Get(ctx, foreign, u.ID, "read"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("cross-project session disclosed")
	}
	readOnly := domain.Principal{KeyID: sec.ID(), ProjectID: project.ID, Scopes: []string{"assets:read"}}
	// Apply search and effective asset status before paging, retaining project isolation.
	listed, err := ingest.List(ctx, p, "SMALL.TXT", "ready", project.ID, 1, 0)
	if err != nil || len(listed) != 1 || listed[0].ID != u.ID {
		t.Fatal("upload search and asset status", err, listed)
	}
	listed, err = ingest.List(ctx, p, "SMALL.TXT", "ready", project.ID, 1, 1)
	if err != nil || len(listed) != 0 {
		t.Fatal("upload matching offset", err, listed)
	}
	listed, err = ingest.List(ctx, foreign, "small", "ready", project.ID, 50, 0)
	if err != nil || len(listed) != 0 {
		t.Fatal("upload filter crossed key project boundary", err, listed)
	}
	listed, err = ingest.List(ctx, readOnly, "reserved", "cancelled", quotaProject.ID, 50, 0)
	if err != nil || len(listed) != 0 {
		t.Fatal("upload filter leaked another project's transfer", err, listed)
	}
	listed, err = ingest.List(ctx, p, "reserved", "cancelled", quotaProject.ID, 50, 0)
	if err != nil || len(listed) != 1 || listed[0].ID != reserved.ID {
		t.Fatal("upload transfer status filter", err, listed)
	}
	if _, err = ingest.Create(ctx, readOnly, project.ID, "denied", 1, ""); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("read key uploaded")
	}
	if _, err = ingest.Create(ctx, p, project.ID, "too big", domain.MaxAssetSize+1, ""); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("oversized asset accepted")
	}
	// The HTTP adapter accepts a real multipart upload and checks cookie CSRF for parts.
	handler := httpapi.New(app, "http://localhost:8080", false, slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, secret, _ := app.CreateKey(ctx, p, project.ID, "integration writer", []string{"assets:write", "assets:read"}, nil)
	req := httptest.NewRequest("POST", "/v1/uploads", strings.NewReader(`{"project_id":"`+project.ID+`","name":"HTTP upload","size_bytes":3}`))
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var httpUpload domain.Upload
	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	httpUpload.ID = result["id"].(string)
	req = httptest.NewRequest("PUT", "/v1/uploads/"+httpUpload.ID+"/parts/0", strings.NewReader("abc"))
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("X-Content-SHA256", checksum([]byte("abc")))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	req = httptest.NewRequest("POST", "/v1/uploads/"+httpUpload.ID+"/complete", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	var leaked map[string]any
	json.Unmarshal(w.Body.Bytes(), &leaked)
	if _, ok := leaked["wrapped_key"]; ok {
		t.Fatal("key envelope exposed")
	}
}
