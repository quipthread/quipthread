-- +goose Up
-- Add an explicit terminal state for channels that were never needed by an
-- initialized digest. Existing pending/leased/sent rows are preserved.
DROP TRIGGER IF EXISTS notification_outbox_channels_reparent_guard;
DROP TRIGGER IF EXISTS notification_outbox_channels_sent_parent_guard;
DROP TRIGGER IF EXISTS notification_outbox_channels_parent_delete;
DROP TRIGGER IF EXISTS notification_outbox_channels_parent_update;
DROP TRIGGER IF EXISTS notification_outbox_channels_parent_insert;
DROP TRIGGER IF EXISTS notification_outbox_parent_id_guard;

CREATE TABLE notification_outbox_channels_rebuilt (
    id             TEXT NOT NULL PRIMARY KEY CHECK (length(trim(id)) > 0 AND length(CAST(id AS BLOB)) <= 255),
    outbox_id      TEXT NOT NULL,
    channel        TEXT NOT NULL CHECK (length(trim(channel)) > 0 AND length(CAST(channel AS BLOB)) <= 255),
    status         TEXT NOT NULL DEFAULT 'pending'
                   CHECK (status IN ('pending', 'leased', 'sent', 'skipped')),
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

INSERT INTO notification_outbox_channels_rebuilt
    (id, outbox_id, channel, status, available_at, lease_until, lease_owner, attempts, last_error, created_at, updated_at)
SELECT id, outbox_id, channel, status, available_at, lease_until, lease_owner, attempts, last_error, created_at, updated_at
FROM notification_outbox_channels;

DROP INDEX IF EXISTS idx_notification_outbox_channels_claim;
DROP TABLE notification_outbox_channels;
ALTER TABLE notification_outbox_channels_rebuilt RENAME TO notification_outbox_channels;

CREATE INDEX IF NOT EXISTS idx_notification_outbox_channels_claim
    ON notification_outbox_channels(outbox_id, status, available_at, created_at, id);

-- Foreign-key enforcement is connection-local in SQLite. Recreate the
-- explicit pooled-connection integrity triggers after the table rebuild.
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

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS notification_outbox_channels_sent_parent_guard
BEFORE INSERT ON notification_outbox_channels
WHEN EXISTS (SELECT 1 FROM notification_outbox WHERE id = NEW.outbox_id AND status = 'sent')
BEGIN
    SELECT RAISE(ABORT, 'cannot add channel to sent notification outbox');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS notification_outbox_channels_reparent_guard
BEFORE UPDATE OF outbox_id ON notification_outbox_channels
WHEN OLD.outbox_id <> NEW.outbox_id
BEGIN
    SELECT RAISE(ABORT, 'notification outbox channel parent is immutable');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS notification_outbox_parent_id_guard
BEFORE UPDATE OF id ON notification_outbox
WHEN OLD.id <> NEW.id
 AND EXISTS (SELECT 1 FROM notification_outbox_channels WHERE outbox_id = OLD.id)
BEGIN
    SELECT RAISE(ABORT, 'cannot update notification outbox parent with channel children');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS notification_outbox_channels_reparent_guard;
DROP TRIGGER IF EXISTS notification_outbox_channels_sent_parent_guard;
DROP TRIGGER IF EXISTS notification_outbox_channels_parent_delete;
DROP TRIGGER IF EXISTS notification_outbox_channels_parent_update;
DROP TRIGGER IF EXISTS notification_outbox_channels_parent_insert;
DROP TRIGGER IF EXISTS notification_outbox_parent_id_guard;
DROP INDEX IF EXISTS idx_notification_outbox_channels_claim;

CREATE TABLE notification_outbox_channels_rebuilt (
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

INSERT INTO notification_outbox_channels_rebuilt
    (id, outbox_id, channel, status, available_at, lease_until, lease_owner, attempts, last_error, created_at, updated_at)
SELECT id, outbox_id, channel,
       CASE WHEN status = 'skipped' THEN 'sent' ELSE status END,
       available_at, lease_until, lease_owner, attempts, last_error, created_at, updated_at
FROM notification_outbox_channels;

DROP TABLE notification_outbox_channels;
ALTER TABLE notification_outbox_channels_rebuilt RENAME TO notification_outbox_channels;

CREATE INDEX IF NOT EXISTS idx_notification_outbox_channels_claim
    ON notification_outbox_channels(outbox_id, status, available_at, created_at, id);

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

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS notification_outbox_channels_sent_parent_guard
BEFORE INSERT ON notification_outbox_channels
WHEN EXISTS (SELECT 1 FROM notification_outbox WHERE id = NEW.outbox_id AND status = 'sent')
BEGIN
    SELECT RAISE(ABORT, 'cannot add channel to sent notification outbox');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS notification_outbox_channels_reparent_guard
BEFORE UPDATE OF outbox_id ON notification_outbox_channels
WHEN OLD.outbox_id <> NEW.outbox_id
BEGIN
    SELECT RAISE(ABORT, 'notification outbox channel parent is immutable');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS notification_outbox_parent_id_guard
BEFORE UPDATE OF id ON notification_outbox
WHEN OLD.id <> NEW.id
 AND EXISTS (SELECT 1 FROM notification_outbox_channels WHERE outbox_id = OLD.id)
BEGIN
    SELECT RAISE(ABORT, 'cannot update notification outbox parent with channel children');
END;
-- +goose StatementEnd
