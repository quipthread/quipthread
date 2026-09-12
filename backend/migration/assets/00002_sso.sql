-- +goose Up
ALTER TABLE sites ADD COLUMN sso_secret TEXT;

-- +goose Down
-- SQLite does not support DROP COLUMN on older versions; no rollback provided.
