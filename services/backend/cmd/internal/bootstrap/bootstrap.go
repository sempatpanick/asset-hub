package bootstrap

import (
	"assethub/internal/adapters/outbound/cryptography"
	"assethub/internal/adapters/outbound/filesystem"
	"assethub/internal/adapters/outbound/postgres"
	"assethub/internal/adapters/outbound/redisqueue"
	"assethub/internal/adapters/outbound/runtimehealth"
	"assethub/internal/adapters/outbound/security"
	"assethub/internal/adapters/outbound/telegram"
	"assethub/internal/adapters/outbound/validation"
	"assethub/internal/application/ports"
	"assethub/internal/application/usecases"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/redis/go-redis/v9"
	"log/slog"
	"net/url"
	"os"
	"strconv"
)

type Config struct {
	DiskReservePercent                                                           uint64
	CacheRoot, SignedKey                                                         string
	CacheCapacity                                                                int64
	DatabaseURL, RedisURL, Origin, Address, BackupRoot                           string
	Secure                                                                       bool
	StagingRoot, LocalRoot, Provider, BotToken, ChannelID, BotAPIBase, MasterKey string
	StagingCapacity                                                              int64
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
func Read() (Config, error) {
	c := Config{DatabaseURL: os.Getenv("DATABASE_URL"), RedisURL: env("REDIS_URL", "redis://redis:6379/0"), Origin: env("PUBLIC_ORIGIN", "http://localhost:3005"), Address: env("LISTEN_ADDR", ":8085"), BackupRoot: env("BACKUP_ROOT", "/backup"), Secure: env("APP_ENV", "production") != "development"}
	u, err := url.Parse(c.Origin)
	if err != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || (c.Secure && u.Scheme != "https") {
		return c, fmt.Errorf("PUBLIC_ORIGIN must be an origin; production requires HTTPS")
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL required")
	}
	c.StagingRoot = env("STAGING_ROOT", "/data/staging")
	c.LocalRoot = env("LOCAL_ROOT", "/data/local")
	c.Provider = env("STORAGE_PROVIDER", "telegram")
	c.BotToken = os.Getenv("TELEGRAM_BOT_TOKEN")
	c.ChannelID = os.Getenv("TELEGRAM_CHANNEL_ID")
	c.BotAPIBase = env("TELEGRAM_API_BASE", "https://api.telegram.org")
	c.MasterKey = os.Getenv("ASSET_MASTER_KEY")
	c.DiskReservePercent, err = strconv.ParseUint(env("DISK_RESERVE_PERCENT", "10"), 10, 64)
	if err != nil || c.DiskReservePercent > 50 || (c.Secure && c.DiskReservePercent < 10) {
		return c, fmt.Errorf("DISK_RESERVE_PERCENT must be 10-50 in production, 0-50 in development")
	}
	c.CacheRoot = env("CACHE_ROOT", "/data/cache")
	c.SignedKey = os.Getenv("SIGNED_URL_KEY")
	c.CacheCapacity, err = strconv.ParseInt(env("CACHE_LIMIT_BYTES", "10737418240"), 10, 64)
	if err != nil || c.CacheCapacity < 0 {
		return c, fmt.Errorf("invalid CACHE_LIMIT_BYTES")
	}
	if c.SignedKey != "" {
		key, e := hex.DecodeString(c.SignedKey)
		if e != nil || len(key) != 32 {
			return c, fmt.Errorf("SIGNED_URL_KEY must be 64 hexadecimal characters")
		}
	}
	c.StagingCapacity, err = strconv.ParseInt(env("STAGING_LIMIT_BYTES", "21474836480"), 10, 64)
	if err != nil || c.StagingCapacity < 512<<20 {
		return c, fmt.Errorf("STAGING_LIMIT_BYTES must be at least 512 MiB")
	}
	if c.Provider != "telegram" && (c.Provider != "local" || c.Secure) {
		return c, fmt.Errorf("STORAGE_PROVIDER must be telegram; local is only allowed in development")
	}
	if c.MasterKey != "" {
		key, e := hex.DecodeString(c.MasterKey)
		if e != nil || len(key) != 32 {
			return c, fmt.Errorf("ASSET_MASTER_KEY must be 64 hexadecimal characters")
		}
	}
	if c.Provider == "telegram" && c.BotAPIBase != "https://api.telegram.org" && c.Secure {
		u, e := url.Parse(c.BotAPIBase)
		if e != nil || u.Scheme != "https" {
			return c, fmt.Errorf("custom production Telegram gateway requires HTTPS")
		}
	}
	return c, nil
}
func Wire(ctx context.Context, c Config) (*usecases.Service, func(), error) {
	repo, err := postgres.Open(ctx, c.DatabaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("database unavailable")
	}
	options, err := redis.ParseURL(c.RedisURL)
	if err != nil {
		repo.Pool.Close()
		return nil, nil, fmt.Errorf("invalid REDIS_URL")
	}
	client := redis.NewClient(options)
	cleanup := func() { repo.Pool.Close(); _ = client.Close() }
	if c.MasterKey != "" {
		key, _ := hex.DecodeString(c.MasterKey)
		hash := sha256.Sum256(key)
		clear(key)
		if err = repo.BindMasterKey(ctx, fmt.Sprintf("%x", hash[:])); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("asset master key does not match installation; restore the original key or perform a controlled rewrap migration")
		}
	}
	if err = os.MkdirAll(c.StagingRoot, 0700); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("staging directory unavailable")
	}
	var provider ports.StorageProvider = telegram.Unconfigured{}
	if c.Provider == "local" {
		provider = &filesystem.Local{Root: c.LocalRoot, ReservePercent: &c.DiskReservePercent}
	} else if c.BotToken != "" && c.ChannelID != "" {
		transport, e := telegram.NewBotAPI(c.BotAPIBase, c.BotToken)
		if e != nil {
			cleanup()
			return nil, nil, fmt.Errorf("invalid Telegram configuration")
		}
		provider = &telegram.Provider{Transport: transport, ChannelID: c.ChannelID, Gate: &redisqueue.TelegramGate{Client: client}}
	}
	var crypto ports.Cryptography
	if c.MasterKey != "" && (c.Provider == "local" || (c.BotToken != "" && c.ChannelID != "")) {
		key, _ := hex.DecodeString(c.MasterKey)
		crypto, err = cryptography.New(key)
		if err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("invalid asset key")
		}
	}
	backup := &filesystem.Backup{Root: c.BackupRoot, StagingRoot: c.StagingRoot, RequireIndependent: c.Secure, ReservePercent: &c.DiskReservePercent}
	h := &runtimehealth.Health{Redis: client, BackupRoot: c.BackupRoot, Provider: provider, Backup: backup, StagingRoot: c.StagingRoot, CacheRoot: c.CacheRoot}
	app := usecases.New(repo, security.Security{}, security.Clock{}, h)
	app.SetIngestion(usecases.NewIngestion(app, repo, &filesystem.Staging{Root: c.StagingRoot, ReservePercent: &c.DiskReservePercent}, crypto, provider, backup, &redisqueue.Queue{Client: client}, c.StagingCapacity, validation.FileInspector{}))
	var signer ports.ContentSigner
	if c.SignedKey != "" {
		key, _ := hex.DecodeString(c.SignedKey)
		signer, err = security.NewSigner(key)
		if err != nil {
			cleanup()
			return nil, nil, err
		}
	}
	app.Ingestion().ConfigureContent(&filesystem.Cache{Root: c.CacheRoot, Capacity: c.CacheCapacity, ReservePercent: &c.DiskReservePercent}, signer)
	return app, cleanup, nil
}
func Logger() *slog.Logger { return slog.New(slog.NewJSONHandler(os.Stdout, nil)) }
