// Development-only browser fixture. This executable is never included in release images.
package main

import (
	"assethub/internal/adapters/inbound/httpapi"
	"assethub/internal/adapters/outbound/cryptography"
	"assethub/internal/adapters/outbound/filesystem"
	"assethub/internal/adapters/outbound/postgres"
	"assethub/internal/adapters/outbound/redisqueue"
	"assethub/internal/adapters/outbound/runtimehealth"
	"assethub/internal/adapters/outbound/security"
	"assethub/internal/adapters/outbound/validation"
	"assethub/internal/application/ports"
	"assethub/internal/application/usecases"
	"assethub/migrations"
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/redis/go-redis/v9"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func run() error {
	base := os.Getenv("TEST_DATABASE_URL")
	u, err := url.Parse(base)
	if err != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") || u.Scheme != "postgres" {
		return fmt.Errorf("preview requires a loopback TEST_DATABASE_URL for a disposable database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, base)
	if err != nil {
		return fmt.Errorf("test database unavailable")
	}
	defer admin.Close()
	sec := security.Security{}
	name := "assethub_e2e_" + strings.ReplaceAll(sec.ID(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		return fmt.Errorf("cannot create disposable database")
	}
	defer func() { _, _ = admin.Exec(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)") }()
	u.Path = "/" + name
	db, err := sql.Open("pgx", u.String())
	if err != nil {
		return err
	}
	defer db.Close()
	goose.SetBaseFS(migrations.Files)
	if err = goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err = goose.UpContext(ctx, db, "."); err != nil {
		return err
	}
	repo, err := postgres.Open(ctx, u.String())
	if err != nil {
		return err
	}
	defer repo.Pool.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:56380", DB: 12})
	defer redisClient.Close()
	fixtureRoot, err := os.MkdirTemp("", "assethub-browser-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(fixtureRoot)
	reservePercent := uint64(0) // Development fixture on a shared Windows disk; production defaults to a 10% reserve.
	stage := &filesystem.Staging{ReservePercent: &reservePercent, Root: filepath.Join(fixtureRoot, "staging")}
	os.MkdirAll(stage.Root, 0700)
	var backup ports.Backup
	backupRoot := filepath.Join(fixtureRoot, "backup")
	if os.Getenv("BACKUP_ENABLED") != "false" {
		backup = &filesystem.Backup{Root: backupRoot, StagingRoot: stage.Root, ReservePercent: &reservePercent}
		os.MkdirAll(backupRoot, 0700)
		os.WriteFile(filepath.Join(backupRoot, ".assethub-backup"), []byte("development fixture"), 0600)
	}
	primary := &filesystem.Local{ReservePercent: &reservePercent, Root: filepath.Join(fixtureRoot, "primary")}
	health := &runtimehealth.Health{Redis: redisClient, BackupRoot: backupRoot, Provider: primary, Backup: backup, BackupDisabled: backup == nil}
	app := usecases.New(repo, sec, security.Clock{}, health)
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		return err
	}
	crypto, err := cryptography.New(key)
	if err != nil {
		return err
	}
	ingest := usecases.NewIngestion(app, repo, stage, crypto, primary, backup, &redisqueue.Queue{Client: redisClient}, 20<<30, validation.FileInspector{})
	app.SetIngestion(ingest)
	signer, _ := security.NewSigner(key)
	ingest.ConfigureContent(&filesystem.Cache{Root: filepath.Join(fixtureRoot, "cache"), Capacity: 10 << 30, ReservePercent: &reservePercent}, signer)
	if err = app.Bootstrap(ctx, "browser@example.test", "Test Administrator", "local-browser-test-password"); err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	api := httpapi.New(app, "http://127.0.0.1:18080", false, log)
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	mux := http.NewServeMux()
	// Test-only loopback shutdown permits orderly cleanup on Windows as well.
	mux.HandleFunc("POST /__fixture/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-AssetHub-Fixture") != "stop" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		stop()
	})
	mux.Handle("/v1/", api)
	mux.Handle("/healthz", api)
	mux.Handle("/readyz", api)
	mux.Handle("/", http.FileServer(http.Dir(filepath.Join("..", "..", "apps", "web", "out"))))
	server := &http.Server{Addr: "127.0.0.1:18080", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	workerDone := make(chan struct{})
	defer func() { stop(); <-workerDone }()
	go func() {
		defer close(workerDone)
		for signalCtx.Err() == nil {
			if ingest.Dispatch(signalCtx) == nil {
				_, _ = ingest.ProcessNext(signalCtx)
			}
			select {
			case <-signalCtx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}()
	go func() {
		<-signalCtx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()
	fmt.Println("Disposable browser-test workspace: http://127.0.0.1:18080")
	err = server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Preview failed:", err)
		os.Exit(1)
	}
}
