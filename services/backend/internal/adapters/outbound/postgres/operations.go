package postgres

import (
	"assethub/internal/domain"
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) AuditAction(ctx context.Context, p domain.Principal, action, project, id, detail string) error {
	return r.transaction(ctx, func(tx pgx.Tx) error { return audit(ctx, tx, p, action, project, id, detail) })
}

func (r *Repository) JobHealth(ctx context.Context) (domain.Health, error) {
	var pending, leased, failed int64
	err := r.Pool.QueryRow(ctx, "SELECT count(*),count(*) FILTER(WHERE lease_until>now()),(SELECT count(*) FROM assets WHERE status='failed') FROM asset_jobs").Scan(&pending, &leased, &failed)
	h := domain.Health{Component: "jobs", Status: "healthy", Detail: fmt.Sprintf("%d durable jobs; %d leased; %d failed assets", pending, leased, failed)}
	if failed > 0 {
		h.Status = "degraded"
	}
	return h, err
}
