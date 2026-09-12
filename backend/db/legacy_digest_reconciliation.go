package db

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/quipthread/quipthread/models"
)

var (
	ErrLegacyDigestNotFound       = errors.New("legacy site digest: not found")
	ErrLegacyDigestConflict       = errors.New("legacy site digest: explicit mapping conflicts")
	ErrLegacyDigestMalformed      = errors.New("legacy site digest: malformed state")
	ErrLegacyDigestNotClaimed     = errors.New("legacy site digest: explicit claim required")
	ErrLegacyDigestCommentInvalid = errors.New("legacy site digest: explicit comment mapping is invalid")
)

// ListUninitializedSiteDigestsContext inventories only tenant-local digest
// identity and delivery metadata. It never infers membership from timestamps.
func (s *sqlStore) ListUninitializedSiteDigestsContext(ctx context.Context) ([]*models.NotificationOutbox, error) {
	if ctx == nil {
		return nil, errors.New("legacy site digest: context is required")
	}
	return retryNotificationOutboxBusyContextResult(ctx, func() ([]*models.NotificationOutbox, error) {
		rows, err := s.db.QueryContext(ctx, `
			SELECT id, site_id, kind, aggregate_key, status, available_at,
			       lease_until, lease_owner, attempts, last_error, created_at, updated_at,
			       digest_membership_version
			FROM notification_outbox
			WHERE kind = ? AND digest_membership_version = 0
			ORDER BY created_at ASC, id ASC`, NotificationOutboxKindSiteDigest)
		if err != nil {
			return nil, err
		}
		defer rows.Close() //nolint:errcheck // read-only cleanup
		out := make([]*models.NotificationOutbox, 0)
		for rows.Next() {
			item, err := scanNotificationOutbox(rows)
			if err != nil {
				return nil, err
			}
			out = append(out, item)
		}
		return out, rows.Err()
	})
}

// ClaimLegacySiteDigestContext claims exactly one identified legacy parent.
// It prevents an operator-scoped reconciliation from leasing unrelated work.
func (s *sqlStore) ClaimLegacySiteDigestContext(ctx context.Context, outboxID, owner string, now time.Time, leaseFor time.Duration) (*models.NotificationOutbox, error) {
	if ctx == nil || strings.TrimSpace(outboxID) != outboxID || outboxID == "" || strings.TrimSpace(owner) != owner || owner == "" || now.IsZero() || leaseFor <= 0 {
		return nil, ErrLegacyDigestNotClaimed
	}
	return retryNotificationOutboxBusyContextResult(ctx, func() (*models.NotificationOutbox, error) {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback() //nolint:errcheck // no-op after successful commit
		item, err := scanNotificationOutbox(tx.QueryRowContext(ctx, `
			SELECT id, site_id, kind, aggregate_key, status, available_at,
			       lease_until, lease_owner, attempts, last_error, created_at, updated_at,
			       digest_membership_version
			FROM notification_outbox WHERE id = ?`, outboxID))
		if err != nil {
			return nil, err
		}
		if item == nil {
			return nil, ErrLegacyDigestNotFound
		}
		if item.Kind != NotificationOutboxKindSiteDigest || item.DigestMembershipVersion != 0 {
			return nil, ErrLegacyDigestConflict
		}
		leaseUntil := now.UTC().Add(leaseFor)
		result, err := tx.ExecContext(ctx, `
			UPDATE notification_outbox
			SET status = ?, lease_until = ?, lease_owner = ?, attempts = attempts + 1, updated_at = ?
			WHERE id = ? AND kind = ? AND digest_membership_version = 0 AND
			      ((status = ?) OR
			       (status = ? AND lease_until IS NOT NULL AND lease_until <= ?))`,
			models.NotificationOutboxLeased, leaseUntil, owner, now.UTC(), outboxID,
			NotificationOutboxKindSiteDigest, models.NotificationOutboxPending,
			models.NotificationOutboxLeased, now.UTC())
		if err != nil {
			return nil, err
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return nil, ErrLegacyDigestNotClaimed
		}
		claimed, err := scanNotificationOutbox(tx.QueryRowContext(ctx, `
			SELECT id, site_id, kind, aggregate_key, status, available_at,
			       lease_until, lease_owner, attempts, last_error, created_at, updated_at,
			       digest_membership_version
			FROM notification_outbox WHERE id = ?`, outboxID))
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return claimed, nil
	})
}

// ReconcileLegacySiteDigestContext records an explicit immutable membership
// list and initializes the digest marker. No comment is selected by time.
// Repeating the same explicit mapping is an idempotent no-op; a different
// mapping fails closed.
func (s *sqlStore) ReconcileLegacySiteDigestContext(ctx context.Context, outboxID, owner string, generation int, commentIDs []string, now time.Time) error {
	if ctx == nil || now.IsZero() || strings.TrimSpace(outboxID) != outboxID || outboxID == "" || len(commentIDs) > NotificationOutboxMaxClaimLimit {
		return ErrLegacyDigestNotClaimed
	}
	ids, err := explicitLegacyCommentIDs(commentIDs)
	if err != nil {
		return err
	}
	return retryNotificationOutboxBusyContext(ctx, func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback() //nolint:errcheck // no-op after successful commit
		parent, spec, err := legacyDigestParentTx(ctx, tx, outboxID)
		if err != nil {
			return err
		}
		membership, err := legacyDigestMembershipTx(ctx, tx, outboxID)
		if err != nil {
			return err
		}
		if parent.DigestMembershipVersion == 1 {
			if !sameStringSet(membership, ids) {
				return ErrLegacyDigestConflict
			}
			return tx.Commit()
		}
		if parent.Status == models.NotificationOutboxLeased &&
			(strings.TrimSpace(owner) != owner || owner == "" || generation <= 0 || parent.LeaseOwner != owner || parent.Attempts != generation || parent.LeaseUntil == nil || !parent.LeaseUntil.After(now)) {
			return ErrNotificationOutboxNotOwned
		}
		if parent.Status != models.NotificationOutboxPending && parent.Status != models.NotificationOutboxLeased && parent.Status != models.NotificationOutboxSent {
			return ErrLegacyDigestNotClaimed
		}
		if len(membership) != 0 {
			return ErrLegacyDigestMalformed
		}
		for _, commentID := range ids {
			if err := validateLegacyDigestCommentTx(ctx, tx, parent.SiteID, spec, commentID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO notification_outbox_comments (outbox_id, comment_id) VALUES (?, ?)`, outboxID, commentID); err != nil {
				return ErrLegacyDigestConflict
			}
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE notification_outbox_channels
			SET status = ?, available_at = ?, lease_until = NULL, lease_owner = '', last_error = '', updated_at = ?
			WHERE outbox_id = ? AND status IN (?, ?)`,
			models.NotificationOutboxChannelPending, parent.AvailableAt.UTC(), now.UTC(), outboxID,
			models.NotificationOutboxChannelPending, models.NotificationOutboxChannelLeased); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE notification_outbox
			SET status = ?, digest_membership_version = 1, lease_until = NULL, lease_owner = '', last_error = '', updated_at = ?
			WHERE id = ? AND digest_membership_version = 0`, models.NotificationOutboxPending, now.UTC(), outboxID); err != nil {
			return err
		}
		return tx.Commit()
	})
}

// RetireLegacySiteDigestContext explicitly retires a legacy digest as an empty
// mapping. Outstanding channels become skipped while already-sent channels are
// preserved. A leased parent requires the exact owner/generation/expiry fence;
// an already-sent legacy parent is safely idempotent without a lease.
func (s *sqlStore) RetireLegacySiteDigestContext(ctx context.Context, outboxID, owner string, generation int, now time.Time) error {
	if ctx == nil || now.IsZero() || strings.TrimSpace(outboxID) != outboxID || outboxID == "" {
		return ErrLegacyDigestNotClaimed
	}
	return retryNotificationOutboxBusyContext(ctx, func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback() //nolint:errcheck // no-op after successful commit
		parent, _, err := legacyDigestParentTx(ctx, tx, outboxID)
		if err != nil {
			return err
		}
		if parent.DigestMembershipVersion == 1 && parent.Status == models.NotificationOutboxSent {
			return tx.Commit()
		}
		if parent.DigestMembershipVersion != 0 || parent.Kind != NotificationOutboxKindSiteDigest {
			return ErrLegacyDigestConflict
		}
		membership, err := legacyDigestMembershipTx(ctx, tx, outboxID)
		if err != nil {
			return err
		}
		if len(membership) != 0 {
			return ErrLegacyDigestMalformed
		}
		if parent.Status == models.NotificationOutboxLeased {
			if strings.TrimSpace(owner) != owner || owner == "" || generation <= 0 || parent.LeaseOwner != owner || parent.Attempts != generation || parent.LeaseUntil == nil || !parent.LeaseUntil.After(now) {
				return ErrNotificationOutboxNotOwned
			}
		} else if parent.Status != models.NotificationOutboxPending && parent.Status != models.NotificationOutboxSent {
			return ErrLegacyDigestNotClaimed
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE notification_outbox_channels
			SET status = ?, lease_until = NULL, lease_owner = '', last_error = '', updated_at = ?
			WHERE outbox_id = ? AND status IN (?, ?)`,
			models.NotificationOutboxChannelSkipped, now.UTC(), outboxID,
			models.NotificationOutboxChannelPending, models.NotificationOutboxChannelLeased); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE notification_outbox
			SET status = ?, digest_membership_version = 1, lease_until = NULL, lease_owner = '', last_error = '', updated_at = ?
			WHERE id = ? AND digest_membership_version = 0`, models.NotificationOutboxSent, now.UTC(), outboxID); err != nil {
			return err
		}
		return tx.Commit()
	})
}

func legacyDigestParentTx(ctx context.Context, tx *sql.Tx, outboxID string) (*models.NotificationOutbox, siteDigestSpec, error) {
	parent, err := scanNotificationOutbox(tx.QueryRowContext(ctx, `
		SELECT id, site_id, kind, aggregate_key, status, available_at,
		       lease_until, lease_owner, attempts, last_error, created_at, updated_at,
		       digest_membership_version
		FROM notification_outbox WHERE id = ?`, outboxID))
	if err != nil {
		return nil, siteDigestSpec{}, err
	}
	if parent == nil {
		return nil, siteDigestSpec{}, ErrLegacyDigestNotFound
	}
	if parent.Kind != NotificationOutboxKindSiteDigest || parent.SiteID == "" {
		return nil, siteDigestSpec{}, ErrLegacyDigestMalformed
	}
	spec, err := parseSiteDigestAggregateKey(parent.AggregateKey)
	if err != nil || !parent.AvailableAt.Equal(spec.windowEnd) {
		return nil, siteDigestSpec{}, ErrLegacyDigestMalformed
	}
	return parent, spec, nil
}

func legacyDigestMembershipTx(ctx context.Context, tx *sql.Tx, outboxID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT comment_id FROM notification_outbox_comments WHERE outbox_id = ? ORDER BY comment_id`, outboxID)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // read-only cleanup
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func explicitLegacyCommentIDs(commentIDs []string) ([]string, error) {
	ids := make([]string, 0, len(commentIDs))
	seen := make(map[string]struct{}, len(commentIDs))
	for _, id := range commentIDs {
		if strings.TrimSpace(id) != id || id == "" || len(id) > NotificationOutboxMaxKeyBytes {
			return nil, ErrLegacyDigestCommentInvalid
		}
		if _, exists := seen[id]; exists {
			return nil, ErrLegacyDigestConflict
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

func validateLegacyDigestCommentTx(ctx context.Context, tx *sql.Tx, siteID string, spec siteDigestSpec, commentID string) error {
	var commentSite, status string
	var createdAt time.Time
	err := tx.QueryRowContext(ctx, `SELECT site_id, status, created_at FROM comments WHERE id = ?`, commentID).Scan(&commentSite, &status, &createdAt)
	if errors.Is(err, sql.ErrNoRows) || commentSite != siteID || (status != "pending" && status != "approved" && status != "rejected") {
		return ErrLegacyDigestCommentInvalid
	}
	if err != nil {
		return ErrLegacyDigestCommentInvalid
	}
	windowStart, _, err := siteDigestWindow(createdAt, spec.interval)
	if err != nil || windowStart != spec.windowStart {
		return ErrLegacyDigestCommentInvalid
	}
	return nil
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
