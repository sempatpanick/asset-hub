-- +goose Up
ALTER TABLE api_keys ADD COLUMN encrypted_secret bytea;
-- +goose Down
ALTER TABLE api_keys DROP COLUMN encrypted_secret;
