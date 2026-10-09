-- +goose Up
CREATE TABLE uploads (
 id uuid PRIMARY KEY, project_id uuid NOT NULL REFERENCES projects(id), owner text NOT NULL,
 name text NOT NULL, size_bytes bigint NOT NULL CHECK(size_bytes BETWEEN 1 AND 536870912),
 expected_sha256 text NOT NULL DEFAULT '', status text NOT NULL DEFAULT 'receiving' CHECK(status IN ('receiving','completing','queued','complete','cancelled','expired')),
 asset_id uuid REFERENCES assets(id), expires_at timestamptz NOT NULL,
 completion_until timestamptz, cleaned_at timestamptz, ended_at timestamptz, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX uploads_expiry ON uploads(expires_at) WHERE status IN ('receiving','completing');
CREATE TABLE upload_parts (upload_id uuid REFERENCES uploads(id) ON DELETE CASCADE, part_index integer NOT NULL CHECK(part_index>=0), size_bytes bigint NOT NULL, sha256 text NOT NULL, PRIMARY KEY(upload_id,part_index));
ALTER TABLE assets ADD COLUMN upload_id uuid REFERENCES uploads(id), ADD COLUMN provider text NOT NULL DEFAULT '', ADD COLUMN wrapped_key bytea, ADD COLUMN error_code text NOT NULL DEFAULT '', ADD COLUMN content_version integer NOT NULL DEFAULT 1;
CREATE UNIQUE INDEX asset_upload ON assets(upload_id);
CREATE TABLE asset_chunks (asset_id uuid REFERENCES assets(id), chunk_index integer NOT NULL CHECK(chunk_index>=0), plain_size bigint NOT NULL, cipher_size bigint NOT NULL, sha256 text NOT NULL, reference bytea, PRIMARY KEY(asset_id,chunk_index));
CREATE TABLE asset_jobs (asset_id uuid PRIMARY KEY REFERENCES assets(id), attempts integer NOT NULL DEFAULT 0, next_attempt timestamptz NOT NULL DEFAULT now(), lease_token text, lease_until timestamptz);
CREATE INDEX asset_jobs_due ON asset_jobs(next_attempt);
-- +goose Down
DROP TABLE asset_jobs,asset_chunks;
ALTER TABLE assets DROP COLUMN upload_id, DROP COLUMN provider,DROP COLUMN wrapped_key,DROP COLUMN error_code,DROP COLUMN content_version;
DROP TABLE upload_parts,uploads;
