package runtimehealth

import (
	"assethub/internal/adapters/outbound/filesystem"
	"assethub/internal/application/ports"
	"assethub/internal/domain"
	"context"
	"github.com/redis/go-redis/v9"
	"os"
	"path/filepath"
)

type Health struct {
	Redis                  *redis.Client
	BackupRoot             string
	Provider               ports.StorageProvider
	Backup                 ports.Backup
	StagingRoot, CacheRoot string
}

func (h *Health) Ready(ctx context.Context) error { return h.Redis.Ping(ctx).Err() }
func (h *Health) Check(ctx context.Context) []domain.Health {
	r := domain.Health{Component: "redis", Status: "healthy", Detail: "Queue dependency reachable; PostgreSQL retains unfinished jobs"}
	if h.Ready(ctx) != nil {
		r.Status = "unavailable"
		r.Detail = "Queue dependency unreachable"
	}
	b := domain.Health{Component: "backup", Status: "unavailable", Detail: "Provision a separate backup mount and .assethub-backup marker"}
	if info, err := os.Stat(filepath.Join(h.BackupRoot, ".assethub-backup")); err == nil && info.Mode().IsRegular() {
		b.Status = "configured"
		b.Detail = "Marker present; independent mount must be verified by operator"
	}
	if h.Backup != nil {
		if h.Backup.Check() != nil {
			b.Status = "unavailable"
			b.Detail = "Required backup marker or independent filesystem unavailable"
		} else {
			b.Status = "healthy"
			b.Detail = "Backup filesystem checks passed"
		}
	}
	provider := domain.Health{Component: "telegram", Status: "unavailable", Detail: "Configure storage credentials"}
	if h.Provider != nil {
		provider = h.Provider.Health(ctx)
	}
	worker := domain.Health{Component: "worker", Status: "healthy", Detail: "Outbox dispatcher heartbeat present"}
	if h.Redis.Get(ctx, "assethub:worker:heartbeat").Err() != nil {
		worker.Status = "unavailable"
		worker.Detail = "No recent worker heartbeat; durable jobs remain in PostgreSQL"
	}
	values := []domain.Health{provider, r, worker, b}
	if h.StagingRoot != "" {
		values = append(values, filesystem.DiskHealth(h.StagingRoot, "staging"))
	}
	return values
}
