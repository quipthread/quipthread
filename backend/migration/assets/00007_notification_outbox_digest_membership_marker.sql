-- +goose Up
-- Existing site-digest parents are intentionally marked uninitialized. Their
-- historical comments cannot be assigned safely from timestamps alone and
-- require an explicit reconciliation before materialization or finalization.
ALTER TABLE notification_outbox
    ADD COLUMN digest_membership_version INTEGER NOT NULL DEFAULT 0
    CHECK (digest_membership_version IN (0, 1));

-- +goose Down
ALTER TABLE notification_outbox DROP COLUMN digest_membership_version;
