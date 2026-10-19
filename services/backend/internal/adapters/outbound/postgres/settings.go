package postgres

import (
	"assethub/internal/domain"
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const uploadSettingsColumns = "upload_concurrency,upload_request_timeout_seconds,upload_response_timeout_seconds,upload_session_lifetime_hours"

func scanUploadSettings(row scanner) (domain.UploadSettings, error) {
	var s domain.UploadSettings
	err := row.Scan(&s.Concurrency, &s.RequestTimeoutSeconds, &s.ResponseTimeoutSeconds, &s.SessionLifetimeHours)
	return s, dbError(err)
}

func (r *Repository) UploadSettings(ctx context.Context) (domain.UploadSettings, error) {
	return scanUploadSettings(r.Pool.QueryRow(ctx, "SELECT "+uploadSettingsColumns+" FROM workspace_settings WHERE singleton"))
}

func (r *Repository) UpdateUploadSettings(ctx context.Context, p domain.Principal, patch domain.UploadSettingsPatch) error {
	return r.transaction(ctx, func(tx pgx.Tx) error {
		current, err := scanUploadSettings(tx.QueryRow(ctx, "SELECT "+uploadSettingsColumns+" FROM workspace_settings WHERE singleton FOR UPDATE"))
		if err != nil {
			return err
		}
		updated, err := current.Apply(patch)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "UPDATE workspace_settings SET upload_concurrency=$1,upload_request_timeout_seconds=$2,upload_response_timeout_seconds=$3,upload_session_lifetime_hours=$4 WHERE singleton", updated.Concurrency, updated.RequestTimeoutSeconds, updated.ResponseTimeoutSeconds, updated.SessionLifetimeHours); err != nil {
			return err
		}
		return audit(ctx, tx, p, "settings.updated", "", "workspace", fmt.Sprintf("Upload settings: concurrency %d; request %ds; response %ds; session %dh", updated.Concurrency, updated.RequestTimeoutSeconds, updated.ResponseTimeoutSeconds, updated.SessionLifetimeHours))
	})
}
