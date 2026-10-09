package integration

import (
	"assethub/internal/adapters/outbound/cryptography"
	"assethub/internal/adapters/outbound/filesystem"
	"assethub/internal/adapters/outbound/postgres"
	"assethub/internal/adapters/outbound/redisqueue"
	"assethub/internal/adapters/outbound/security"
	"assethub/internal/adapters/outbound/telegram"
	"assethub/internal/adapters/outbound/validation"
	"assethub/internal/application/usecases"
	"assethub/internal/domain"
	"bytes"
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPostgresSnapshotAndOfflineKeyDrill(t *testing.T) {
	dumpBinary, restoreBinary := os.Getenv("TEST_PG_DUMP_BINARY"), os.Getenv("TEST_PG_RESTORE_BINARY")
	if dumpBinary == "" || restoreBinary == "" {
		t.Skip("PostgreSQL client binaries required for a real dump/restore drill")
	}
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	defer client.Del(ctx, "assethub:upload:pending")
	repo := isolatedRepository(t)
	if err = repo.BindMasterKey(ctx, "test-master-fingerprint"); err != nil {
		t.Fatal(err)
	}
	if err = repo.BindMasterKey(ctx, "test-master-fingerprint"); err != nil {
		t.Fatal("same master key refused", err)
	}
	if err = repo.BindMasterKey(ctx, "wrong-master-fingerprint"); err != domain.ErrConflict {
		t.Fatal("changed master key accepted", err)
	}
	sec := security.Security{}
	app := usecases.New(repo, sec, security.Clock{}, healthy{})
	if err = app.Bootstrap(ctx, "drill@example.test", "Drill Admin", "integration-password-123"); err != nil {
		t.Fatal(err)
	}
	user, _, _, err := app.Login(ctx, "drill@example.test", "integration-password-123")
	if err != nil {
		t.Fatal(err)
	}
	p := domain.Principal{User: user}
	project, err := app.CreateProject(ctx, p, "Recovery drill", "", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	zero := uint64(0)
	stage := &filesystem.Staging{Root: filepath.Join(root, "staging"), ReservePercent: &zero}
	backup := &filesystem.Backup{Root: filepath.Join(root, "backup"), StagingRoot: stage.Root, ReservePercent: &zero}
	os.MkdirAll(backup.Root, 0700)
	os.WriteFile(filepath.Join(backup.Root, ".assethub-backup"), []byte("fixture"), 0600)
	master := bytes.Repeat([]byte{17}, 32)
	crypto, _ := cryptography.New(master)
	primary := &filesystem.Local{Root: filepath.Join(root, "primary"), ReservePercent: &zero}
	queue := &redisqueue.Queue{Client: client}
	ingest := usecases.NewIngestion(app, repo, stage, crypto, primary, backup, queue, 20<<30, validation.FileInspector{})
	ingest.ConfigureContent(nil, nil)
	app.SetIngestion(ingest)
	original := []byte("Recovery drill: PostgreSQL snapshot plus independently retained ciphertext and offline master key.")
	u, err := ingest.Create(ctx, p, project.ID, "recovery.txt", int64(len(original)), checksum(original))
	if err != nil {
		t.Fatal(err)
	}
	if err = ingest.PutPart(ctx, p, u.ID, 0, checksum(original), bytes.NewReader(original)); err != nil {
		t.Fatal(err)
	}
	asset, err := ingest.Complete(ctx, p, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ingest.ProcessNext(ctx); err != nil {
		t.Fatal(err)
	}
	// Passwords are supplied using a restricted pgpass file, never process arguments.
	conn := repo.Pool.Config().ConnConfig
	escape := strings.NewReplacer("\\", "\\\\", ":", "\\:")
	pass := filepath.Join(root, "pgpass")
	line := escape.Replace(conn.Host) + ":" + itoa(int(conn.Port)) + ":*:" + escape.Replace(conn.User) + ":" + escape.Replace(conn.Password) + "\n"
	if err = os.WriteFile(pass, []byte(line), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(root, "snapshot.dump")
	args := []string{"--host", conn.Host, "--port", itoa(int(conn.Port)), "--username", conn.User, "--no-password", "--no-owner", "--no-acl"}
	cmd := exec.CommandContext(ctx, dumpBinary, append(args, "--format=custom", "--file", snapshot, "--dbname", conn.Database)...)
	cmd.Env = append(os.Environ(), "PGPASSFILE="+pass)
	if output, err := cmd.CombinedOutput(); err != nil {
		_ = output
		t.Fatal("pg_dump failed")
	}
	cloneName := "assethub_restore_" + strings.ReplaceAll(sec.ID(), "-", "")
	if _, err = repo.Pool.Exec(ctx, "CREATE DATABASE "+cloneName); err != nil {
		t.Fatal("cannot create drill database")
	}
	defer repo.Pool.Exec(context.Background(), "DROP DATABASE "+cloneName+" WITH (FORCE)")
	cmd = exec.CommandContext(ctx, restoreBinary, append(args, "--exit-on-error", "--dbname", cloneName, snapshot)...)
	cmd.Env = append(os.Environ(), "PGPASSFILE="+pass)
	if output, err := cmd.CombinedOutput(); err != nil {
		_ = output
		t.Fatal("pg_restore failed")
	}
	cloneConfig := repo.Pool.Config().Copy()
	cloneConfig.ConnConfig.Database = cloneName
	pool, err := pgxpool.NewWithConfig(ctx, cloneConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	clone := &postgres.Repository{Pool: pool}
	restoredApp := usecases.New(clone, sec, security.Clock{}, healthy{})
	offlineCrypto, _ := cryptography.New(append([]byte(nil), master...))
	restored := usecases.NewIngestion(restoredApp, clone, stage, offlineCrypto, telegram.Unconfigured{}, backup, queue, 20<<30, validation.FileInspector{})
	restored.ConfigureContent(nil, nil)
	metadata, err := restored.AuthorizeContent(ctx, p, asset.ID, "")
	if err != nil {
		t.Fatal("restored authorization failed", err)
	}
	reader, err := restored.OpenContent(ctx, metadata, 0, metadata.Size-1)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := io.ReadAll(reader)
	reader.Close()
	if err != nil || !bytes.Equal(actual, original) {
		t.Fatal("database/key/backup recovery changed bytes")
	}
	wrongCrypto, _ := cryptography.New(bytes.Repeat([]byte{18}, 32))
	wrong := usecases.NewIngestion(restoredApp, clone, stage, wrongCrypto, telegram.Unconfigured{}, backup, queue, 20<<30, validation.FileInspector{})
	wrong.ConfigureContent(nil, nil)
	if _, err = wrong.OpenContent(ctx, metadata, 0, metadata.Size-1); err == nil {
		t.Fatal("wrong offline key decrypted recovered data")
	}
}
