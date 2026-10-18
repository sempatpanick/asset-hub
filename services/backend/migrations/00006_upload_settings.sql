-- +goose Up
CREATE TABLE workspace_settings (
 singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
 upload_concurrency integer NOT NULL DEFAULT 20 CHECK (upload_concurrency BETWEEN 1 AND 20)
);
INSERT INTO workspace_settings (singleton) VALUES (true);

-- +goose Down
DROP TABLE workspace_settings;
