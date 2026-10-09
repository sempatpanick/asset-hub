package postgres

import (
	"assethub/internal/domain"
	"context"
)

// Fingerprints prevent accidental replacement of the sole v1 wrapping key.
// They are never exposed through API projections. Rotation needs a deliberate rewrap migration.
func (r *Repository) BindMasterKey(ctx context.Context, fingerprint string) error {
	var stored string
	err := r.Pool.QueryRow(ctx, "INSERT INTO installation_keys(key_name,fingerprint) VALUES('asset-master-v1',$1) ON CONFLICT(key_name) DO UPDATE SET fingerprint=installation_keys.fingerprint RETURNING fingerprint", fingerprint).Scan(&stored)
	if err != nil {
		return err
	}
	if stored != fingerprint {
		return domain.ErrConflict
	}
	return nil
}
