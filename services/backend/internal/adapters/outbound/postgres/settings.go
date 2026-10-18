package postgres

import (
	"assethub/internal/domain"
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) UploadConcurrency(ctx context.Context) (int, error) {
	var limit int
	err := r.Pool.QueryRow(ctx, "SELECT upload_concurrency FROM workspace_settings WHERE singleton").Scan(&limit)
	return limit, dbError(err)
}

func (r *Repository) UpdateUploadConcurrency(ctx context.Context, p domain.Principal, limit int) error {
	return r.transaction(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "UPDATE workspace_settings SET upload_concurrency=$1 WHERE singleton", limit); err != nil {
			return err
		}
		return audit(ctx, tx, p, "settings.updated", "", "workspace", fmt.Sprintf("Concurrent upload limit set to %d", limit))
	})
}
