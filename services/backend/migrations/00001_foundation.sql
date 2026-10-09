-- +goose Up
CREATE TABLE users (id uuid PRIMARY KEY,email text UNIQUE NOT NULL CHECK(email=lower(email)),name text NOT NULL,role text NOT NULL CHECK(role IN ('admin','member')),password_hash text NOT NULL,disabled boolean NOT NULL DEFAULT false,created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE invitations (id uuid PRIMARY KEY,email text NOT NULL,role text NOT NULL CHECK(role IN ('admin','member')),token_hash text UNIQUE NOT NULL,expires_at timestamptz NOT NULL,accepted_at timestamptz);
CREATE TABLE sessions (token_hash text PRIMARY KEY,user_id uuid NOT NULL REFERENCES users(id),csrf text NOT NULL,expires_at timestamptz NOT NULL);
CREATE INDEX sessions_expiry ON sessions(expires_at);
CREATE TABLE projects (id uuid PRIMARY KEY,name text NOT NULL,description text NOT NULL DEFAULT '',quota_bytes bigint NOT NULL CHECK(quota_bytes>0),created_by uuid NOT NULL REFERENCES users(id),created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE project_members (project_id uuid NOT NULL REFERENCES projects(id),user_id uuid NOT NULL REFERENCES users(id),role text NOT NULL CHECK(role IN ('manager','editor','viewer')),PRIMARY KEY(project_id,user_id));
CREATE TABLE api_keys (id uuid PRIMARY KEY,project_id uuid NOT NULL REFERENCES projects(id),name text NOT NULL,prefix text NOT NULL,token_hash text UNIQUE NOT NULL,scopes text[] NOT NULL,expires_at timestamptz,revoked_at timestamptz,created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE assets (id uuid PRIMARY KEY,project_id uuid NOT NULL REFERENCES projects(id),name text NOT NULL,mime text NOT NULL DEFAULT 'application/octet-stream',size_bytes bigint NOT NULL CHECK(size_bytes>=0),sha256 text NOT NULL DEFAULT '',status text NOT NULL CHECK(status IN ('staged','queued','uploading','verifying','ready','failed','deleting','deleted')),visibility text NOT NULL DEFAULT 'private' CHECK(visibility IN ('private','public')),created_at timestamptz NOT NULL DEFAULT now(),deleted_at timestamptz);
CREATE INDEX assets_project_time ON assets(project_id,created_at DESC,id DESC);
CREATE INDEX assets_state ON assets(status);
CREATE TABLE audit_events (id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,actor_id text NOT NULL,action text NOT NULL,project_id uuid REFERENCES projects(id),object_id text NOT NULL DEFAULT '',detail text NOT NULL DEFAULT '',created_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX audit_project_time ON audit_events(project_id,created_at DESC,id DESC);
-- +goose Down
DROP TABLE audit_events,assets,api_keys,project_members,projects,sessions,invitations,users;
