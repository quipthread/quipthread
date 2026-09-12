-- +goose Up
-- Notification delivery state is intentionally tenant-local. No notification
-- payload, comment content, token, secret, or provider credential is stored.
CREATE TABLE IF NOT EXISTS notification_outbox (
    id             TEXT NOT NULL PRIMARY KEY CHECK (length(trim(id)) > 0 AND length(CAST(id AS BLOB)) <= 255),
    site_id        TEXT NOT NULL CHECK (length(trim(site_id)) > 0 AND length(CAST(site_id AS BLOB)) <= 255),
    kind           TEXT NOT NULL CHECK (length(trim(kind)) > 0 AND length(CAST(kind AS BLOB)) <= 255),
    aggregate_key  TEXT NOT NULL CHECK (length(trim(aggregate_key)) > 0 AND length(CAST(aggregate_key AS BLOB)) <= 255),
    status         TEXT NOT NULL DEFAULT 'pending'
                   CHECK (status IN ('pending', 'leased', 'sent')),
    available_at   DATETIME NOT NULL,
    lease_until    DATETIME,
    lease_owner    TEXT NOT NULL DEFAULT '' CHECK (length(CAST(lease_owner AS BLOB)) <= 255),
    attempts       INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error     TEXT NOT NULL DEFAULT '' CHECK (length(CAST(last_error AS BLOB)) <= 1024),
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(site_id, kind, aggregate_key),
    CHECK (
        (status = 'leased' AND lease_until IS NOT NULL AND lease_owner <> '') OR
        (status <> 'leased' AND lease_until IS NULL AND lease_owner = '')
    )
);

CREATE INDEX IF NOT EXISTS idx_notification_outbox_claim
    ON notification_outbox(status, available_at, created_at, id);

-- +goose Down
DROP INDEX IF EXISTS idx_notification_outbox_claim;
DROP TABLE IF EXISTS notification_outbox;
