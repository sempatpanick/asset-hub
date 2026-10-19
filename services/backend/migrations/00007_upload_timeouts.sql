-- +goose Up
ALTER TABLE workspace_settings
 ADD COLUMN upload_request_timeout_seconds integer NOT NULL DEFAULT 120 CHECK (upload_request_timeout_seconds BETWEEN 30 AND 1800),
 ADD COLUMN upload_response_timeout_seconds integer NOT NULL DEFAULT 150 CHECK (upload_response_timeout_seconds BETWEEN 60 AND 1830),
 ADD COLUMN upload_session_lifetime_hours integer NOT NULL DEFAULT 24 CHECK (upload_session_lifetime_hours BETWEEN 1 AND 168),
 ADD CONSTRAINT upload_response_grace CHECK (upload_response_timeout_seconds >= upload_request_timeout_seconds + 30);
ALTER TABLE uploads ADD COLUMN transfer_until timestamptz;

-- +goose Down
ALTER TABLE uploads DROP COLUMN transfer_until;
ALTER TABLE workspace_settings
 DROP CONSTRAINT upload_response_grace,
 DROP COLUMN upload_request_timeout_seconds,
 DROP COLUMN upload_response_timeout_seconds,
 DROP COLUMN upload_session_lifetime_hours;
