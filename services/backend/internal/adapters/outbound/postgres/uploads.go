package postgres

import (
	"assethub/internal/domain"
	"context"
	"github.com/jackc/pgx/v5"
	"time"
)

func (r *Repository) ReserveUpload(ctx context.Context, p domain.Principal, u domain.Upload, capacity int64) error {
	return r.transaction(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(874302)"); err != nil {
			return err
		}
		var quota, used, staged int64
		if err := tx.QueryRow(ctx, "SELECT quota_bytes FROM projects WHERE id=$1 FOR UPDATE", u.ProjectID).Scan(&quota); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT COALESCE((SELECT sum(size_bytes) FROM assets WHERE project_id=$1 AND status NOT IN ('deleted','deleting')),0)::bigint+COALESCE((SELECT sum(size_bytes) FROM uploads WHERE project_id=$1 AND (status='completing' OR (status='receiving' AND expires_at>now()))),0)::bigint`, u.ProjectID).Scan(&used); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(size_bytes),0)::bigint FROM uploads WHERE cleaned_at IS NULL`).Scan(&staged); err != nil {
			return err
		}
		if u.Size > quota-used || u.Size > capacity-staged {
			return domain.ErrCapacity
		}
		_, err := tx.Exec(ctx, `INSERT INTO uploads(id,project_id,owner,name,size_bytes,expected_sha256,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, u.ID, u.ProjectID, actor(p), u.Name, u.Size, u.ExpectedSHA256, u.ExpiresAt)
		if err != nil {
			return err
		}
		return audit(ctx, tx, p, "upload.created", u.ProjectID, u.ID, "Upload capacity reserved")
	})
}

type scanner interface{ Scan(...any) error }

func scanUpload(row scanner) (domain.Upload, error) {
	var u domain.Upload
	err := row.Scan(&u.ID, &u.ProjectID, &u.Name, &u.Size, &u.ExpectedSHA256, &u.Status, &u.AssetID, &u.Owner, &u.ExpiresAt, &u.CreatedAt, &u.AssetStatus, &u.ErrorCode, &u.UploadedChunks)
	u.Parts = []domain.UploadPart{}
	return u, dbError(err)
}

const uploadColumns = `u.id::text,u.project_id::text,u.name,u.size_bytes,u.expected_sha256,u.status,COALESCE(u.asset_id::text,''),u.owner,u.expires_at,u.created_at,COALESCE((SELECT status FROM assets WHERE id=u.asset_id),''),COALESCE((SELECT error_code FROM assets WHERE id=u.asset_id),''),(SELECT count(*) FROM asset_chunks WHERE asset_id=u.asset_id AND reference IS NOT NULL)`

func (r *Repository) Upload(ctx context.Context, id string) (domain.Upload, error) {
	u, err := scanUpload(r.Pool.QueryRow(ctx, "SELECT "+uploadColumns+" FROM uploads u WHERE u.id::text=$1", id))
	if err != nil {
		return u, err
	}
	rows, err := r.Pool.Query(ctx, "SELECT part_index,size_bytes,sha256 FROM upload_parts WHERE upload_id=$1 ORDER BY part_index", id)
	if err != nil {
		return u, err
	}
	defer rows.Close()
	for rows.Next() {
		var p domain.UploadPart
		if err = rows.Scan(&p.Index, &p.Size, &p.SHA256); err != nil {
			return u, err
		}
		u.Parts = append(u.Parts, p)
	}
	return u, rows.Err()
}
func (r *Repository) Uploads(ctx context.Context, p domain.Principal, limit, offset int) ([]domain.Upload, error) {
	rows, err := r.Pool.Query(ctx, "SELECT "+uploadColumns+" FROM uploads u JOIN projects p ON p.id=u.project_id WHERE "+access+" ORDER BY u.created_at DESC,u.id DESC LIMIT $4 OFFSET $5", p.IsAdmin(), p.ProjectID, p.User.ID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Upload{}
	for rows.Next() {
		u, err := scanUpload(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
func (r *Repository) RecordPart(ctx context.Context, id string, p domain.UploadPart) error {
	return r.transaction(ctx, func(tx pgx.Tx) error {
		var status string
		var expires time.Time
		if err := tx.QueryRow(ctx, "SELECT status,expires_at FROM uploads WHERE id=$1 FOR UPDATE", id).Scan(&status, &expires); err != nil {
			return err
		}
		if status != "receiving" || !expires.After(time.Now()) {
			return domain.ErrConflict
		}
		var hash string
		err := tx.QueryRow(ctx, "INSERT INTO upload_parts VALUES($1,$2,$3,$4) ON CONFLICT(upload_id,part_index) DO UPDATE SET sha256=upload_parts.sha256 RETURNING sha256", id, p.Index, p.Size, p.SHA256).Scan(&hash)
		if err != nil {
			return err
		}
		if hash != p.SHA256 {
			return domain.ErrConflict
		}
		return nil
	})
}
func (r *Repository) BeginCompletion(ctx context.Context, id string) (domain.Upload, error) {
	err := r.transaction(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE uploads SET status='completing',completion_until=now()+interval '15 minutes' WHERE id=$1 AND expires_at>now() AND (status='receiving' OR (status='completing' AND completion_until<now()))`, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return domain.ErrConflict
		}
		return nil
	})
	if err != nil {
		return domain.Upload{}, err
	}
	return r.Upload(ctx, id)
}
func (r *Repository) ResetCompletion(ctx context.Context, id string) error {
	_, err := r.Pool.Exec(ctx, "UPDATE uploads SET status='receiving',completion_until=NULL WHERE id=$1 AND status='completing'", id)
	return err
}
func (r *Repository) CompleteUpload(ctx context.Context, p domain.Principal, u domain.Upload, a domain.StoredAsset) error {
	return r.transaction(ctx, func(tx pgx.Tx) error {
		var status string
		if err := tx.QueryRow(ctx, "SELECT status FROM uploads WHERE id=$1 FOR UPDATE", u.ID).Scan(&status); err != nil {
			return err
		}
		if status != "completing" {
			return domain.ErrConflict
		}
		_, err := tx.Exec(ctx, `INSERT INTO assets(id,project_id,name,mime,size_bytes,sha256,status,upload_id,provider,wrapped_key) VALUES($1,$2,$3,$4,$5,$6,'queued',$7,$8,$9)`, a.ID, u.ProjectID, u.Name, a.MIME, u.Size, a.SHA256, u.ID, a.Provider, a.WrappedKey)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "UPDATE uploads SET status='queued',asset_id=$2,completion_until=NULL WHERE id=$1", u.ID, a.ID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO asset_jobs(asset_id) VALUES($1)", a.ID); err != nil {
			return err
		}
		return audit(ctx, tx, p, "asset.queued", u.ProjectID, a.ID, "Validated upload committed with durable job")
	})
}
func (r *Repository) CancelUpload(ctx context.Context, p domain.Principal, id string) error {
	return r.transaction(ctx, func(tx pgx.Tx) error {
		var project string
		err := tx.QueryRow(ctx, `UPDATE uploads SET status='cancelled',ended_at=now() WHERE id=$1 AND (status='receiving' OR (status='completing' AND completion_until<now())) RETURNING project_id::text`, id).Scan(&project)
		if err == pgx.ErrNoRows {
			return domain.ErrConflict
		}
		if err != nil {
			return err
		}
		return audit(ctx, tx, p, "upload.cancelled", project, id, "Capacity released")
	})
}
func (r *Repository) Asset(ctx context.Context, id string) (domain.StoredAsset, error) {
	var a domain.StoredAsset
	err := r.Pool.QueryRow(ctx, `SELECT id::text,project_id::text,name,mime,size_bytes,sha256,status,visibility,created_at,COALESCE(upload_id::text,''),provider,wrapped_key,error_code,COALESCE((SELECT attempts FROM asset_jobs WHERE asset_id=assets.id),0),content_version,deleted_at,backup_purged FROM assets WHERE id::text=$1`, id).Scan(&a.ID, &a.ProjectID, &a.Name, &a.MIME, &a.Size, &a.SHA256, &a.Status, &a.Visibility, &a.CreatedAt, &a.UploadID, &a.Provider, &a.WrappedKey, &a.ErrorCode, &a.Attempts, &a.Version, &a.DeletedAt, &a.BackupPurged)
	if err != nil {
		return a, dbError(err)
	}
	rows, err := r.Pool.Query(ctx, "SELECT chunk_index,plain_size,cipher_size,sha256,reference,purge_status FROM asset_chunks WHERE asset_id=$1 ORDER BY chunk_index", id)
	if err != nil {
		return a, err
	}
	defer rows.Close()
	for rows.Next() {
		var c domain.Chunk
		if err = rows.Scan(&c.Index, &c.PlainSize, &c.CipherSize, &c.SHA256, &c.Reference, &c.PurgeStatus); err != nil {
			return a, err
		}
		a.Chunks = append(a.Chunks, c)
	}
	return a, rows.Err()
}
func (r *Repository) DueJobs(ctx context.Context, limit int) ([]string, error) {
	rows, err := r.Pool.Query(ctx, `SELECT j.asset_id::text FROM asset_jobs j JOIN assets a ON a.id=j.asset_id WHERE (a.status IN ('queued','uploading','verifying') OR (a.status='deleted' AND j.kind='delete')) AND next_attempt<=now() AND (lease_until IS NULL OR lease_until<now()) ORDER BY next_attempt LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
func (r *Repository) ClaimJob(ctx context.Context, id, token string) (domain.StoredAsset, error) {
	err := r.transaction(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE asset_jobs SET lease_token=$2,lease_until=now()+interval '10 minutes',attempts=attempts+1 WHERE asset_id=$1 AND next_attempt<=now() AND (lease_until IS NULL OR lease_until<now()) AND EXISTS(SELECT 1 FROM assets WHERE id=$1 AND (status IN ('queued','uploading','verifying') OR (status='deleted' AND asset_jobs.kind='delete')))`, id, token)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return domain.ErrConflict
		}
		_, err = tx.Exec(ctx, "UPDATE assets SET status='uploading',error_code='' WHERE id=$1 AND status<>'deleted'", id)
		return err
	})
	if err != nil {
		return domain.StoredAsset{}, err
	}
	return r.Asset(ctx, id)
}
func lockLease(ctx context.Context, tx pgx.Tx, id, token string) error {
	var idOut string
	err := tx.QueryRow(ctx, "UPDATE asset_jobs SET lease_until=now()+interval '10 minutes' WHERE asset_id=$1 AND lease_token=$2 AND lease_until>now() RETURNING asset_id::text", id, token).Scan(&idOut)
	if err == pgx.ErrNoRows {
		return domain.ErrConflict
	}
	return err
}
func (r *Repository) SaveChunk(ctx context.Context, id, token string, c domain.Chunk) error {
	return r.transaction(ctx, func(tx pgx.Tx) error {
		if err := lockLease(ctx, tx, id, token); err != nil {
			return err
		}
		var hash string
		err := tx.QueryRow(ctx, `INSERT INTO asset_chunks(asset_id,chunk_index,plain_size,cipher_size,sha256,reference) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(asset_id,chunk_index) DO UPDATE SET reference=COALESCE(EXCLUDED.reference,asset_chunks.reference) RETURNING sha256`, id, c.Index, c.PlainSize, c.CipherSize, c.SHA256, c.Reference).Scan(&hash)
		if err != nil {
			return err
		}
		if hash != c.SHA256 {
			return domain.ErrIntegrity
		}
		return nil
	})
}
func (r *Repository) ReadyAsset(ctx context.Context, id, token string) error {
	return r.transaction(ctx, func(tx pgx.Tx) error {
		if err := lockLease(ctx, tx, id, token); err != nil {
			return err
		}
		var expected, count int
		var bytes int64
		var project, upload string
		if err := tx.QueryRow(ctx, "SELECT ((size_bytes+16777215)/16777216)::int,project_id::text,upload_id::text FROM assets WHERE id=$1", id).Scan(&expected, &project, &upload); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, "SELECT count(*) FILTER(WHERE reference IS NOT NULL),COALESCE(sum(plain_size),0)::bigint FROM asset_chunks WHERE asset_id=$1", id).Scan(&count, &bytes); err != nil {
			return err
		}
		var size int64
		if err := tx.QueryRow(ctx, "SELECT size_bytes FROM assets WHERE id=$1", id).Scan(&size); err != nil {
			return err
		}
		if count != expected || bytes != size {
			return domain.ErrIntegrity
		}
		if _, err := tx.Exec(ctx, "UPDATE assets SET status='ready',error_code='' WHERE id=$1", id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE uploads SET status='complete',ended_at=now() WHERE id=$1", upload); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM asset_jobs WHERE asset_id=$1", id); err != nil {
			return err
		}
		return audit(ctx, tx, domain.Principal{User: domain.User{Email: "worker"}}, "asset.ready", project, id, "Encrypted primary chunks and independent backup verified")
	})
}
func (r *Repository) FailJob(ctx context.Context, id, token, code string, delay time.Duration, permanent bool) error {
	return r.transaction(ctx, func(tx pgx.Tx) error {
		if err := lockLease(ctx, tx, id, token); err != nil {
			return err
		}
		status := "queued"
		var kind string
		if err := tx.QueryRow(ctx, "SELECT kind FROM asset_jobs WHERE asset_id=$1", id).Scan(&kind); err != nil {
			return err
		}
		if kind == "delete" {
			status = "deleted"
		}
		if permanent && kind != "delete" {
			status = "failed"
		}
		if _, err := tx.Exec(ctx, "UPDATE assets SET status=$2,error_code=$3 WHERE id=$1", id, status, code); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE asset_jobs SET lease_token=NULL,lease_until=NULL,next_attempt=now()+($2::double precision * interval '1 second') WHERE asset_id=$1", id, delay.Seconds()); err != nil {
			return err
		}
		var project string
		if err := tx.QueryRow(ctx, "SELECT project_id::text FROM assets WHERE id=$1", id).Scan(&project); err != nil {
			return err
		}
		return audit(ctx, tx, domain.Principal{User: domain.User{Email: "worker"}}, "asset.job_failed", project, id, code)
	})
}
func (r *Repository) RetryAsset(ctx context.Context, p domain.Principal, id string) error {
	return r.transaction(ctx, func(tx pgx.Tx) error {
		var project string
		err := tx.QueryRow(ctx, "UPDATE assets SET status='queued',error_code='' WHERE id=$1 AND status='failed' RETURNING project_id::text", id).Scan(&project)
		if err == pgx.ErrNoRows {
			return domain.ErrConflict
		}
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO asset_jobs(asset_id) VALUES($1) ON CONFLICT(asset_id) DO UPDATE SET attempts=0,next_attempt=now(),lease_token=NULL,lease_until=NULL", id); err != nil {
			return err
		}
		return audit(ctx, tx, p, "asset.retried", project, id, "Job requeued")
	})
}
func (r *Repository) CleanupUploads(ctx context.Context) ([]string, error) {
	if _, err := r.Pool.Exec(ctx, "UPDATE uploads SET status='expired',ended_at=now() WHERE status IN ('receiving','completing') AND expires_at<now() AND (completion_until IS NULL OR completion_until<now())"); err != nil {
		return nil, err
	}
	rows, err := r.Pool.Query(ctx, "SELECT id::text FROM uploads WHERE status IN ('expired','cancelled','complete') AND cleaned_at IS NULL AND ended_at<now()-interval '3 minutes' LIMIT 200")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
func (r *Repository) MarkCleaned(ctx context.Context, id string) error {
	_, err := r.Pool.Exec(ctx, "UPDATE uploads SET cleaned_at=now() WHERE id=$1 AND status IN ('expired','cancelled','complete')", id)
	return err
}
