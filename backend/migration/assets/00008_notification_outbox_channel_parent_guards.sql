-- +goose Up
-- Channel initialization and reparenting must not bypass a finalized parent.
-- These guards are database-level because callers may use pooled connections.
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

-- +goose Down
DROP TRIGGER IF EXISTS notification_outbox_channels_reparent_guard;
DROP TRIGGER IF EXISTS notification_outbox_channels_sent_parent_guard;
