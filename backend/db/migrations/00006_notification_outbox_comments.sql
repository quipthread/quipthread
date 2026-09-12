-- +goose Up
-- Existing digest outbox rows predate immutable membership. They are left
-- untouched; materialization must reject them until a reconciliation process
-- explicitly supplies membership rows.
CREATE TABLE IF NOT EXISTS notification_outbox_comments (
    outbox_id  TEXT NOT NULL,
    comment_id TEXT NOT NULL CHECK (length(trim(comment_id)) > 0 AND length(CAST(comment_id AS BLOB)) <= 255),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (outbox_id, comment_id),
    UNIQUE (comment_id),
    FOREIGN KEY (outbox_id) REFERENCES notification_outbox(id) ON DELETE CASCADE,
    FOREIGN KEY (comment_id) REFERENCES comments(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_notification_outbox_comments_comment
    ON notification_outbox_comments(comment_id);

-- Foreign-key enforcement is connection-local in SQLite. These explicit
-- triggers preserve parent and comment integrity for every pooled connection.
-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS notification_outbox_comments_parent_insert
BEFORE INSERT ON notification_outbox_comments
WHEN NOT EXISTS (SELECT 1 FROM notification_outbox WHERE id = NEW.outbox_id)
BEGIN
    SELECT RAISE(ABORT, 'notification outbox parent not found');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS notification_outbox_comments_comment_insert
BEFORE INSERT ON notification_outbox_comments
WHEN NOT EXISTS (SELECT 1 FROM comments WHERE id = NEW.comment_id)
BEGIN
    SELECT RAISE(ABORT, 'notification outbox comment not found');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS notification_outbox_comments_update_guard
BEFORE UPDATE ON notification_outbox_comments
BEGIN
    SELECT RAISE(ABORT, 'notification outbox comment membership is immutable');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS notification_outbox_comments_parent_id_guard
BEFORE UPDATE OF id ON notification_outbox
WHEN OLD.id <> NEW.id
 AND EXISTS (SELECT 1 FROM notification_outbox_comments WHERE outbox_id = OLD.id)
BEGIN
    SELECT RAISE(ABORT, 'cannot update notification outbox parent with comment memberships');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS notification_outbox_comments_comment_id_guard
BEFORE UPDATE OF id ON comments
WHEN OLD.id <> NEW.id
 AND EXISTS (SELECT 1 FROM notification_outbox_comments WHERE comment_id = OLD.id)
BEGIN
    SELECT RAISE(ABORT, 'cannot update comment with digest membership');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS notification_outbox_comments_parent_delete
AFTER DELETE ON notification_outbox
BEGIN
    DELETE FROM notification_outbox_comments WHERE outbox_id = OLD.id;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS notification_outbox_comments_comment_delete
AFTER DELETE ON comments
BEGIN
    DELETE FROM notification_outbox_comments WHERE comment_id = OLD.id;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS notification_outbox_comments_comment_delete;
DROP TRIGGER IF EXISTS notification_outbox_comments_comment_id_guard;
DROP TRIGGER IF EXISTS notification_outbox_comments_parent_delete;
DROP TRIGGER IF EXISTS notification_outbox_comments_parent_id_guard;
DROP TRIGGER IF EXISTS notification_outbox_comments_update_guard;
DROP TRIGGER IF EXISTS notification_outbox_comments_comment_insert;
DROP TRIGGER IF EXISTS notification_outbox_comments_parent_insert;
DROP INDEX IF EXISTS idx_notification_outbox_comments_comment;
DROP TABLE IF EXISTS notification_outbox_comments;
