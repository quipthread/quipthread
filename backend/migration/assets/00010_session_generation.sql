-- +goose Up
ALTER TABLE users ADD COLUMN dashboard_session_generation INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN embed_session_generation INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite does not support DROP COLUMN on older versions; no rollback provided.
