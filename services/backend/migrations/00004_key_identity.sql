-- +goose Up
CREATE TABLE installation_keys (key_name text PRIMARY KEY, fingerprint text NOT NULL);
-- +goose Down
DROP TABLE installation_keys;
