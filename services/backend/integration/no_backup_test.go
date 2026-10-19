package integration

import (
	"assethub/internal/adapters/outbound/cryptography"
	"assethub/internal/adapters/outbound/filesystem"
	"assethub/internal/adapters/outbound/redisqueue"
	"assethub/internal/adapters/outbound/runtimehealth"
	"assethub/internal/adapters/outbound/security"
	"assethub/internal/adapters/outbound/validation"
	"assethub/internal/application/usecases"
	"assethub/internal/domain"
	"bytes"
	"context"
	"errors"
	"github.com/redis/go-redis/v9"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestIngestionWithoutBackup(t *testing.T) {
	raw := os.Getenv("TEST_REDIS_URL")
	if raw == "" {
		t.Skip("dedicated TEST_REDIS_URL required")
	}
	opts, err := redis.ParseURL(raw)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(opts)
	defer client.Close()
	ctx := context.Background()
	if err = client.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	defer client.Del(ctx, "assethub:upload:pending")
	repo := isolatedRepository(t)
	app := usecases.New(repo, security.Security{}, security.Clock{}, healthy{})
	if err = app.Bootstrap(ctx, "primary-only@example.test", "Admin", "integration-password-123"); err != nil {
		t.Fatal(err)
	}
	user, _, _, err := app.Login(ctx, "primary-only@example.test", "integration-password-123")
	if err != nil {
		t.Fatal(err)
	}
	principal := domain.Principal{User: user}
	project, err := app.CreateProject(ctx, principal, "Primary only", "", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	zero := uint64(0)
	stage := &filesystem.Staging{Root: filepath.Join(root, "staging"), ReservePercent: &zero}
	primary := &failingPrimary{Local: filesystem.Local{Root: filepath.Join(root, "primary"), ReservePercent: &zero}, FailAt: 2}
	crypto, _ := cryptography.New(bytes.Repeat([]byte{7}, 32))
	newIngestion := func() *usecases.Ingestion {
		ingest := usecases.NewIngestion(app, repo, stage, crypto, primary, nil, &redisqueue.Queue{Client: client}, 2<<30, validation.FileInspector{})
		signer, _ := security.NewSigner(bytes.Repeat([]byte{8}, 32))
		ingest.ConfigureContent(nil, signer)
		app.SetIngestion(ingest)
		return ingest
	}
	ingest := newIngestion()
	if ingest.BackupEnabled() || ingest.BackupRetentionDays() != 0 {
		t.Fatal("advertised unavailable backups")
	}
	data := bytes.Repeat([]byte{37}, int(domain.StorageChunkSize)+101)
	upload, err := ingest.Create(ctx, principal, project.ID, "primary.bin", int64(len(data)), checksum(data))
	if err != nil {
		t.Fatal(err)
	}
	for index, offset := 0, 0; offset < len(data); index, offset = index+1, offset+int(domain.UploadPartSize) {
		part := data[offset:min(offset+int(domain.UploadPartSize), len(data))]
		if err = ingest.PutPart(ctx, principal, upload.ID, index, checksum(part), bytes.NewReader(part)); err != nil {
			t.Fatal(err)
		}
	}
	asset, err := ingest.Complete(ctx, principal, upload.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = ingest.Dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	if worked, err := ingest.ProcessNext(ctx); !worked || err != nil {
		t.Fatal("retry processing", err)
	}
	stored, err := repo.Asset(ctx, asset.ID)
	if err != nil || stored.Status != "queued" || stored.ErrorCode != "provider_retry" || len(stored.Chunks) != 2 {
		t.Fatal("missing durable retry checkpoint", stored.Status, err)
	}
	// Restart the use case and lose a completed checkpoint: the protected primary reference recovers it.
	if err = os.Remove(filepath.Join(stage.Root, upload.ID, "checkpoints", "0.enc")); err != nil {
		t.Fatal(err)
	}
	ingest = newIngestion()
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
		t.Fatal("retry uploaded completed chunk again", stored.Status, primary.Puts, err)
	}
	token, err := ingest.SignedLink(ctx, principal, asset.ID, 900)
	if err != nil {
		t.Fatal(err)
	}
	authorized, err := ingest.AuthorizeContent(ctx, domain.Principal{}, asset.ID, token)
	if err != nil {
		t.Fatal(err)
	}
	start, end := domain.StorageChunkSize-10, domain.StorageChunkSize+10
	reader, err := ingest.OpenContent(ctx, authorized, start, end)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := io.ReadAll(reader)
	reader.Close()
	if err != nil || !bytes.Equal(actual, data[start:end+1]) {
		t.Fatal("signed range differs", err)
	}
	if _, err = repo.Pool.Exec(ctx, "UPDATE uploads SET ended_at=now()-interval '4 minutes',transfer_until=now()-interval '1 second' WHERE id=$1", upload.ID); err != nil {
		t.Fatal(err)
	}
	if err = ingest.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(stage.Root, upload.ID)); !os.IsNotExist(err) {
		t.Fatal("temporary checkpoints retained", err)
	}
	if _, err = os.Stat(filepath.Join(root, "backup")); !os.IsNotExist(err) {
		t.Fatal("backup directory fabricated")
	}
	// Primary corruption must not be hidden by an unavailable backup or temporary upload copy.
	primary.Corrupt = true
	reader, err = ingest.OpenContent(ctx, authorized, 0, 10)
	if err == nil {
		_, err = io.ReadAll(reader)
		reader.Close()
	}
	if !errors.Is(err, domain.ErrUnavailable) && !errors.Is(err, domain.ErrIntegrity) {
		t.Fatal("primary corruption accepted", err)
	}
	primary.Corrupt = false
	if err = ingest.Delete(ctx, principal, asset.ID); err != nil {
		t.Fatal(err)
	}
	if err = ingest.Restore(ctx, principal, asset.ID); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("restore advertised without backup", err)
	}
	h := &runtimehealth.Health{Redis: client, Provider: primary, BackupDisabled: true}
	found := false
	for _, component := range h.Check(ctx) {
		if component.Component == "backup" {
			found = component.Status == "disabled"
		}
	}
	if !found {
		t.Fatal("health concealed disabled backup")
	}
}
