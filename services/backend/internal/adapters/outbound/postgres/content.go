package postgres

import (
	"assethub/internal/domain"
	"context"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) UpdateAsset(ctx context.Context, p domain.Principal, id, name, visibility string) error {
	return r.transaction(ctx, func(tx pgx.Tx) error {
		var project string
		err := tx.QueryRow(ctx, "UPDATE assets SET name=$2,visibility=$3,content_version=content_version+1 WHERE id=$1 AND status NOT IN ('deleted','deleting') RETURNING project_id::text", id, name, visibility).Scan(&project)
		if err != nil {
			return err
		}
		return audit(ctx, tx, p, "asset.updated", project, id, "Name or visibility updated; previous signed links revoked")
	})
}
func (r *Repository) DeleteAsset(ctx context.Context, p domain.Principal, id string) error {
	return r.transaction(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT asset_id FROM asset_jobs WHERE asset_id=$1 FOR UPDATE", id); err != nil {
			return err
		}
		var project, upload string
		err := tx.QueryRow(ctx, "UPDATE assets SET status='deleted',deleted_at=now(),content_version=content_version+1 WHERE id=$1 AND status NOT IN ('deleted','deleting') RETURNING project_id::text,upload_id::text", id).Scan(&project, &upload)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO asset_jobs(asset_id,kind) VALUES($1,'delete') ON CONFLICT(asset_id) DO UPDATE SET kind='delete',attempts=0,next_attempt=now(),lease_token=NULL,lease_until=NULL", id); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "UPDATE uploads SET status='cancelled',ended_at=now(),cleaned_at=NULL WHERE id=$1", upload); err != nil {
			return err
		}
		return audit(ctx, tx, p, "asset.deleted", project, id, "Content revoked immediately; backup retained for 30 days")
	})
}
func (r *Repository) RestoreAsset(ctx context.Context, p domain.Principal, id string) error {
	return r.transaction(ctx, func(tx pgx.Tx) error {
		var busy bool
		err := tx.QueryRow(ctx, "SELECT COALESCE(lease_until>now(),false) FROM asset_jobs WHERE asset_id=$1 FOR UPDATE", id).Scan(&busy)
		if err != nil && err != pgx.ErrNoRows {
			return err
		}
		if busy {
			return domain.ErrConflict
		}
		var project string
		var size int64
		if err = tx.QueryRow(ctx, "SELECT project_id::text,size_bytes FROM assets WHERE id=$1 AND status='deleted' AND NOT backup_purged AND deleted_at>now()-interval '30 days' FOR UPDATE", id).Scan(&project, &size); err != nil {
			return err
		}
		var quota, used int64
		if err = tx.QueryRow(ctx, "SELECT quota_bytes FROM projects WHERE id=$1 FOR UPDATE", project).Scan(&quota); err != nil {
			return err
		}
		if err = tx.QueryRow(ctx, "SELECT COALESCE((SELECT sum(size_bytes) FROM assets WHERE project_id=$1 AND status NOT IN ('deleted','deleting')),0)::bigint+COALESCE((SELECT sum(size_bytes) FROM uploads WHERE project_id=$1 AND (status='completing' OR (status='receiving' AND expires_at>now()))),0)::bigint", project).Scan(&used); err != nil {
			return err
		}
		if size > quota-used {
			return domain.ErrCapacity
		}
		if _, err = tx.Exec(ctx, "DELETE FROM asset_jobs WHERE asset_id=$1", id); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "UPDATE assets SET status='ready',deleted_at=NULL,content_version=content_version+1,error_code='' WHERE id=$1", id); err != nil {
			return err
		}
		return audit(ctx, tx, p, "asset.restored", project, id, "Retained backup verified and quota reserved")
	})
}
func (r *Repository) SavePurge(ctx context.Context, id, lease string, index int, status string) error {
	return r.transaction(ctx, func(tx pgx.Tx) error {
		if err := lockLease(ctx, tx, id, lease); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "UPDATE asset_chunks SET purge_status=$3 WHERE asset_id=$1 AND chunk_index=$2", id, index, status)
		return err
	})
}
func (r *Repository) FinishPurge(ctx context.Context, id, lease string) error {
	return r.transaction(ctx, func(tx pgx.Tx) error {
		if err := lockLease(ctx, tx, id, lease); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM asset_jobs WHERE asset_id=$1", id); err != nil {
			return err
		}
		var project string
		var unavailable int
		if err := tx.QueryRow(ctx, "SELECT project_id::text,(SELECT count(*) FROM asset_chunks WHERE asset_id=$1 AND purge_status='unavailable') FROM assets WHERE id=$1", id).Scan(&project, &unavailable); err != nil {
			return err
		}
		detail := "Primary deletion attempted; backup remains retained"
		if unavailable > 0 {
			detail = "Primary deletion incomplete; unavailable remote objects may remain; backup retained"
		}
		return audit(ctx, tx, domain.Principal{User: domain.User{Email: "worker"}}, "asset.purge_attempted", project, id, detail)
	})
}
func (r *Repository) ExpiredBackups(ctx context.Context) ([]string, error) {
	rows, err := r.Pool.Query(ctx, "SELECT id::text FROM assets WHERE status='deleted' AND NOT backup_purged AND deleted_at<=now()-interval '30 days' LIMIT 100")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
func (r *Repository) MarkBackupPurged(ctx context.Context, id string) error {
	_, err := r.Pool.Exec(ctx, "UPDATE assets SET backup_purged=true WHERE id=$1 AND status='deleted' AND deleted_at<=now()-interval '30 days'", id)
	return err
}
