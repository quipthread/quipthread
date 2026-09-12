-- +goose Up
-- Parent IDs are referenced by channel rows. This guard is explicit because
-- SQLite foreign-key enforcement is connection-local and an ID update must
-- never strand children when a pooled connection has foreign_keys disabled.
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
DROP TRIGGER IF EXISTS notification_outbox_parent_id_guard;
