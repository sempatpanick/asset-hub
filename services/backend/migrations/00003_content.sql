-- +goose Up
ALTER TABLE assets ADD COLUMN backup_purged boolean NOT NULL DEFAULT false;
ALTER TABLE asset_chunks ADD COLUMN purge_status text NOT NULL DEFAULT '' CHECK(purge_status IN ('','purged','unavailable'));
ALTER TABLE asset_jobs ADD COLUMN kind text NOT NULL DEFAULT 'upload' CHECK(kind IN ('upload','delete'));
CREATE INDEX deleted_asset_retention ON assets(deleted_at) WHERE status='deleted' AND NOT backup_purged;
-- +goose Down
DROP INDEX deleted_asset_retention;
ALTER TABLE asset_jobs DROP COLUMN kind;
ALTER TABLE asset_chunks DROP COLUMN purge_status;
ALTER TABLE assets DROP COLUMN backup_purged;
