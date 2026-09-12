-- +goose Up
-- Per-channel state is tenant-local and stores delivery state only. Payloads,
-- recipients, credentials, tokens, and comment content remain outside this table.
CREATE TABLE IF NOT EXISTS notification_outbox_channels (
    id             TEXT NOT NULL PRIMARY KEY CHECK (length(trim(id)) > 0 AND length(CAST(id AS BLOB)) <= 255),
    outbox_id      TEXT NOT NULL,
    channel        TEXT NOT NULL CHECK (length(trim(channel)) > 0 AND length(CAST(channel AS BLOB)) <= 255),
    status         TEXT NOT NULL DEFAULT 'pending'
                   CHECK (status IN ('pending', 'leased', 'sent')),
    available_at   DATETIME NOT NULL,
    lease_until    DATETIME,
    lease_owner    TEXT NOT NULL DEFAULT '' CHECK (length(CAST(lease_owner AS BLOB)) <= 255),
    attempts       INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error     TEXT NOT NULL DEFAULT '' CHECK (length(CAST(last_error AS BLOB)) <= 1024),
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(outbox_id, channel),
    FOREIGN KEY (outbox_id) REFERENCES notification_outbox(id) ON DELETE CASCADE,
    CHECK (
        (status = 'leased' AND lease_until IS NOT NULL AND lease_owner <> '') OR
        (status <> 'leased' AND lease_until IS NULL AND lease_owner = '')
    )
);

CREATE INDEX IF NOT EXISTS idx_notification_outbox_channels_claim
    ON notification_outbox_channels(outbox_id, status, available_at, created_at, id);

-- Foreign-key enforcement is connection-local in SQLite. These explicit
-- triggers preserve the same parent-integrity guarantee for pooled SQLite and
-- libSQL connections whose PRAGMA foreign_keys setting is not inherited.
-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS notification_outbox_channels_parent_insert
BEFORE INSERT ON notification_outbox_channels
WHEN NOT EXISTS (SELECT 1 FROM notification_outbox WHERE id = NEW.outbox_id)
BEGIN
    SELECT RAISE(ABORT, 'notification outbox parent not found');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS notification_outbox_channels_parent_update
BEFORE UPDATE OF outbox_id ON notification_outbox_channels
WHEN NOT EXISTS (SELECT 1 FROM notification_outbox WHERE id = NEW.outbox_id)
BEGIN
    SELECT RAISE(ABORT, 'notification outbox parent not found');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS notification_outbox_channels_parent_delete
AFTER DELETE ON notification_outbox
BEGIN
    DELETE FROM notification_outbox_channels WHERE outbox_id = OLD.id;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS notification_outbox_channels_parent_delete;
DROP TRIGGER IF EXISTS notification_outbox_channels_parent_update;
DROP TRIGGER IF EXISTS notification_outbox_channels_parent_insert;
DROP INDEX IF EXISTS idx_notification_outbox_channels_claim;
DROP TABLE IF EXISTS notification_outbox_channels;
