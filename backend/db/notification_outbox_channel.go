package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/quipthread/quipthread/models"
)

const NotificationOutboxMaxChannelCount = NotificationOutboxMaxClaimLimit

var (
	// These aliases keep channel transitions compatible with the parent
	// outbox's ownership and not-found error contract.
	ErrNotificationOutboxChannelNotFound = ErrNotificationOutboxNotFound
	ErrNotificationOutboxChannelNotOwned = ErrNotificationOutboxNotOwned
	ErrInvalidNotificationOutboxChannel  = ErrInvalidNotificationOutbox
	ErrNotificationOutboxFinalized       = errors.New("notification outbox: parent is finalized")
)

// EnsureNotificationOutboxChannels creates pending child state without
// changing an existing channel. The operation is idempotent by (outbox,
// channel), and all requested rows are created or observed in one transaction.
func (s *sqlStore) EnsureNotificationOutboxChannels(outboxID string, channels []string) ([]*models.NotificationOutboxChannel, error) {
	return s.EnsureNotificationOutboxChannelsContext(context.Background(), outboxID, channels)
}

// EnsureNotificationOutboxChannelsContext idempotently creates child rows
// using the caller's cancellation context.
func (s *sqlStore) EnsureNotificationOutboxChannelsContext(ctx context.Context, outboxID string, channels []string) ([]*models.NotificationOutboxChannel, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: context is required", ErrInvalidNotificationOutbox)
	}
	if err := validateNotificationOutboxChannelKey(outboxID, "outbox_id"); err != nil {
		return nil, err
	}
	if len(channels) == 0 || len(channels) > NotificationOutboxMaxChannelCount {
		return nil, fmt.Errorf("%w: channel count must be between 1 and %d", ErrInvalidNotificationOutbox, NotificationOutboxMaxChannelCount)
	}

	unique := make([]string, 0, len(channels))
	seen := make(map[string]struct{}, len(channels))
	for _, channel := range channels {
		if err := validateNotificationOutboxChannelKey(channel, "channel"); err != nil {
			return nil, err
		}
		if _, exists := seen[channel]; exists {
			continue
		}
		seen[channel] = struct{}{}
		unique = append(unique, channel)
	}

	now := time.Now().UTC()
	rows := make([]*models.NotificationOutboxChannel, len(unique))
	for i, channel := range unique {
		rows[i] = &models.NotificationOutboxChannel{
			ID:          uuid.NewString(),
			OutboxID:    outboxID,
			Channel:     channel,
			Status:      models.NotificationOutboxChannelPending,
			AvailableAt: now,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
	}

	return retryNotificationOutboxBusyContextResult(ctx, func() ([]*models.NotificationOutboxChannel, error) {
		return s.ensureNotificationOutboxChannelsOnce(ctx, outboxID, rows)
	})
}

func (s *sqlStore) ensureNotificationOutboxChannelsOnce(ctx context.Context, outboxID string, requested []*models.NotificationOutboxChannel) ([]*models.NotificationOutboxChannel, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin ensure notification channels: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after successful commit

	if err := requireMutableNotificationOutboxTx(ctx, tx, outboxID); err != nil {
		return nil, err
	}
	for _, item := range requested {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO notification_outbox_channels
			 (id, outbox_id, channel, status, available_at, lease_until, lease_owner, attempts, last_error, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, NULL, '', 0, '', ?, ?)
			 ON CONFLICT(outbox_id, channel) DO NOTHING`,
			item.ID, item.OutboxID, item.Channel, item.Status,
			item.AvailableAt.UTC(), item.CreatedAt.UTC(), item.UpdatedAt.UTC(),
		)
		if err != nil {
			return nil, fmt.Errorf("ensure notification channel %q: %w", item.Channel, err)
		}
	}

	canonical := make([]*models.NotificationOutboxChannel, 0, len(requested))
	for _, item := range requested {
		row := tx.QueryRowContext(ctx,
			`SELECT id, outbox_id, channel, status, available_at,
			        lease_until, lease_owner, attempts, last_error, created_at, updated_at
			 FROM notification_outbox_channels
			 WHERE outbox_id = ? AND channel = ?`, item.OutboxID, item.Channel)
		channel, err := scanNotificationOutboxChannel(row)
		if err != nil {
			return nil, fmt.Errorf("read notification channel %q: %w", item.Channel, err)
		}
		if channel == nil {
			return nil, fmt.Errorf("read notification channel %q: %w", item.Channel, ErrNotificationOutboxChannelNotFound)
		}
		canonical = append(canonical, channel)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit ensure notification channels: %w", err)
	}
	return canonical, nil
}

func requireMutableNotificationOutboxTx(ctx context.Context, tx *sql.Tx, outboxID string) error {
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM notification_outbox WHERE id = ?`, outboxID).Scan(&status); errors.Is(err, sql.ErrNoRows) {
		return ErrNotificationOutboxNotFound
	} else if err != nil {
		return fmt.Errorf("check notification outbox parent: %w", err)
	}
	if status == models.NotificationOutboxSent {
		return ErrNotificationOutboxFinalized
	}
	return nil
}

func (s *sqlStore) ListNotificationOutboxChannels(outboxID string) ([]*models.NotificationOutboxChannel, error) {
	return s.ListNotificationOutboxChannelsContext(context.Background(), outboxID)
}

// ListNotificationOutboxChannelsContext lists child rows using the caller's
// cancellation context.
func (s *sqlStore) ListNotificationOutboxChannelsContext(ctx context.Context, outboxID string) ([]*models.NotificationOutboxChannel, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: context is required", ErrInvalidNotificationOutbox)
	}
	if err := validateNotificationOutboxChannelKey(outboxID, "outbox_id"); err != nil {
		return nil, err
	}
	return retryNotificationOutboxBusyContextResult(ctx, func() ([]*models.NotificationOutboxChannel, error) {
		rows, err := s.db.QueryContext(ctx,
			`SELECT id, outbox_id, channel, status, available_at,
			        lease_until, lease_owner, attempts, last_error, created_at, updated_at
			 FROM notification_outbox_channels
			 WHERE outbox_id = ? ORDER BY channel ASC`, outboxID)
		if err != nil {
			return nil, fmt.Errorf("list notification channels: %w", err)
		}
		defer rows.Close() //nolint:errcheck // read-only cleanup
		channels := make([]*models.NotificationOutboxChannel, 0)
		for rows.Next() {
			channel, scanErr := scanNotificationOutboxChannel(rows)
			if scanErr != nil {
				return nil, fmt.Errorf("scan notification channel: %w", scanErr)
			}
			channels = append(channels, channel)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("read notification channels: %w", err)
		}
		return channels, nil
	})
}

func (s *sqlStore) GetNotificationOutboxChannel(id string) (*models.NotificationOutboxChannel, error) {
	if err := validateNotificationOutboxChannelKey(id, "id"); err != nil {
		return nil, err
	}
	return retryNotificationOutboxBusyResult(func() (*models.NotificationOutboxChannel, error) {
		return scanNotificationOutboxChannel(s.db.QueryRow( //nolint:noctx // DB layer; full context threading deferred
			`SELECT id, outbox_id, channel, status, available_at,
			        lease_until, lease_owner, attempts, last_error, created_at, updated_at
			 FROM notification_outbox_channels WHERE id = ?`, id))
	})
}

func (s *sqlStore) GetNotificationOutboxChannelByKey(outboxID, channel string) (*models.NotificationOutboxChannel, error) {
	if err := validateNotificationOutboxChannelKey(outboxID, "outbox_id"); err != nil {
		return nil, err
	}
	if err := validateNotificationOutboxChannelKey(channel, "channel"); err != nil {
		return nil, err
	}
	return retryNotificationOutboxBusyResult(func() (*models.NotificationOutboxChannel, error) {
		return scanNotificationOutboxChannel(s.db.QueryRow( //nolint:noctx // DB layer; full context threading deferred
			`SELECT id, outbox_id, channel, status, available_at,
			        lease_until, lease_owner, attempts, last_error, created_at, updated_at
			 FROM notification_outbox_channels
			 WHERE outbox_id = ? AND channel = ?`, outboxID, channel))
	})
}

// ClaimNotificationOutboxChannels leases only pending or expired leased
// channels belonging to the specified parent. Sent channels are never
// eligible, so a retry of one channel cannot resend another channel.
func (s *sqlStore) ClaimNotificationOutboxChannels(outboxID, owner string, now time.Time, leaseFor time.Duration, limit int) ([]*models.NotificationOutboxChannel, error) {
	return s.ClaimNotificationOutboxChannelsContext(context.Background(), outboxID, owner, now, leaseFor, limit)
}

// ClaimNotificationOutboxChannelsContext leases child rows using the caller's
// cancellation context.
func (s *sqlStore) ClaimNotificationOutboxChannelsContext(ctx context.Context, outboxID, owner string, now time.Time, leaseFor time.Duration, limit int) ([]*models.NotificationOutboxChannel, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: context is required", ErrInvalidNotificationOutbox)
	}
	if err := validateNotificationOutboxChannelKey(outboxID, "outbox_id"); err != nil {
		return nil, err
	}
	if err := validateOwnerAndID("channel-claim", owner); err != nil {
		return nil, err
	}
	if leaseFor <= 0 {
		return nil, fmt.Errorf("%w: lease duration must be positive", ErrInvalidNotificationOutbox)
	}
	if limit <= 0 || limit > NotificationOutboxMaxClaimLimit {
		return nil, fmt.Errorf("%w: claim limit must be between 1 and %d", ErrInvalidNotificationOutbox, NotificationOutboxMaxClaimLimit)
	}
	return retryNotificationOutboxBusyContextResult(ctx, func() ([]*models.NotificationOutboxChannel, error) {
		return s.claimNotificationOutboxChannelsOnce(ctx, outboxID, owner, now.UTC(), leaseFor, limit)
	})
}

func (s *sqlStore) claimNotificationOutboxChannelsOnce(ctx context.Context, outboxID, owner string, now time.Time, leaseFor time.Duration, limit int) ([]*models.NotificationOutboxChannel, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin claim notification channels: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after successful commit
	if err := requireNotificationOutboxTx(ctx, tx, outboxID); err != nil {
		return nil, err
	}

	now = now.UTC()
	leaseUntil := normalizeNotificationOutboxLeaseTime(now.Add(leaseFor))
	leaseUntilSQL := notificationOutboxLeaseSQLTime(leaseUntil)
	nowLeaseSQL := notificationOutboxLeaseSQLTime(now)
	rows, err := tx.QueryContext(ctx,
		`UPDATE notification_outbox_channels
		 SET status = ?, lease_until = ?, lease_owner = ?, attempts = attempts + 1, updated_at = ?
		 WHERE id IN (
		   SELECT id FROM notification_outbox_channels
		   WHERE outbox_id = ? AND ((status = ? AND available_at <= ?)
		      OR (status = ? AND lease_until IS NOT NULL AND lease_until <= ?))
		   ORDER BY available_at ASC, created_at ASC, channel ASC, id ASC
		   LIMIT ?
		 )
		 RETURNING id, outbox_id, channel, status, available_at,
		           lease_until, lease_owner, attempts, last_error, created_at, updated_at`,
		models.NotificationOutboxChannelLeased, leaseUntilSQL, owner, now,
		outboxID, models.NotificationOutboxChannelPending, now,
		models.NotificationOutboxChannelLeased, nowLeaseSQL, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("claim notification channels: %w", err)
	}
	claimed := make([]*models.NotificationOutboxChannel, 0, limit)
	for rows.Next() {
		channel, scanErr := scanNotificationOutboxChannel(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan claimed notification channel: %w", scanErr)
		}
		claimed = append(claimed, channel)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read claimed notification channels: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close claimed notification channels: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit claim notification channels: %w", err)
	}
	sort.Slice(claimed, func(i, j int) bool {
		if claimed[i].AvailableAt.Equal(claimed[j].AvailableAt) {
			if claimed[i].CreatedAt.Equal(claimed[j].CreatedAt) {
				if claimed[i].Channel == claimed[j].Channel {
					return claimed[i].ID < claimed[j].ID
				}
				return claimed[i].Channel < claimed[j].Channel
			}
			return claimed[i].CreatedAt.Before(claimed[j].CreatedAt)
		}
		return claimed[i].AvailableAt.Before(claimed[j].AvailableAt)
	})
	return claimed, nil
}

func (s *sqlStore) AckNotificationOutboxChannel(id, owner string, generation int, now time.Time) error {
	return s.AckNotificationOutboxChannelContext(context.Background(), id, owner, generation, now)
}

// AckNotificationOutboxChannelContext marks a leased child sent using the
// caller's cancellation context and exact lease generation.
func (s *sqlStore) AckNotificationOutboxChannelContext(ctx context.Context, id, owner string, generation int, now time.Time) error {
	if ctx == nil {
		return fmt.Errorf("%w: context is required", ErrInvalidNotificationOutbox)
	}
	if err := validateChannelTransition(id, owner, generation, now); err != nil {
		return err
	}
	now = now.UTC()
	if err := s.requireActiveNotificationOutboxChannelLeaseContext(ctx, id, owner, generation, now); err != nil {
		return err
	}
	return retryNotificationOutboxBusyContext(ctx, func() error {
		res, err := s.db.ExecContext(ctx,
			`UPDATE notification_outbox_channels
			 SET status = ?, lease_until = NULL, lease_owner = '', last_error = '', updated_at = ?
			 WHERE id = ? AND status = ? AND lease_owner = ? AND attempts = ? AND lease_until > ?`,
			models.NotificationOutboxChannelSent, now.UTC(), id,
			models.NotificationOutboxChannelLeased, owner, generation, notificationOutboxLeaseSQLTime(now),
		)
		if err != nil {
			return fmt.Errorf("ack notification channel: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 1 {
			return nil
		}
		return s.notificationOutboxChannelOwnershipErrorContext(ctx, id)
	})
}

func (s *sqlStore) RetryNotificationOutboxChannel(id, owner string, generation int, availableAt, now time.Time, lastError string) error {
	return s.RetryNotificationOutboxChannelContext(context.Background(), id, owner, generation, availableAt, now, lastError)
}

// RetryNotificationOutboxChannelContext returns a leased child to pending
// using the caller's cancellation context and exact lease generation.
func (s *sqlStore) RetryNotificationOutboxChannelContext(ctx context.Context, id, owner string, generation int, availableAt, now time.Time, lastError string) error {
	if ctx == nil {
		return fmt.Errorf("%w: context is required", ErrInvalidNotificationOutbox)
	}
	if err := validateChannelTransition(id, owner, generation, now); err != nil {
		return err
	}
	if availableAt.IsZero() || !availableAt.After(now) {
		return fmt.Errorf("%w: retry availability must be after retry time", ErrInvalidNotificationOutbox)
	}
	if len(lastError) > NotificationOutboxMaxErrorBytes {
		return fmt.Errorf("%w: last error exceeds %d bytes", ErrInvalidNotificationOutbox, NotificationOutboxMaxErrorBytes)
	}
	now = now.UTC()
	if err := s.requireActiveNotificationOutboxChannelLeaseContext(ctx, id, owner, generation, now); err != nil {
		return err
	}
	return retryNotificationOutboxBusyContext(ctx, func() error {
		res, err := s.db.ExecContext(ctx,
			`UPDATE notification_outbox_channels
			 SET status = ?, available_at = ?, lease_until = NULL, lease_owner = '', last_error = ?, updated_at = ?
			 WHERE id = ? AND status = ? AND lease_owner = ? AND attempts = ? AND lease_until > ?`,
			models.NotificationOutboxChannelPending, availableAt.UTC(), lastError, now.UTC(), id,
			models.NotificationOutboxChannelLeased, owner, generation, notificationOutboxLeaseSQLTime(now),
		)
		if err != nil {
			return fmt.Errorf("retry notification channel: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 1 {
			return nil
		}
		return s.notificationOutboxChannelOwnershipErrorContext(ctx, id)
	})
}

// requireActiveNotificationOutboxChannelLeaseContext performs the boundary
// check in Go as well as in the UPDATE predicate. This avoids a driver-specific
// datetime representation turning an exact expiry into a successful write.
// The SQL predicate remains the authoritative race fence.
func (s *sqlStore) requireActiveNotificationOutboxChannelLeaseContext(ctx context.Context, id, owner string, generation int, now time.Time) error {
	var leaseUntil sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT lease_until
		FROM notification_outbox_channels
		WHERE id = ? AND status = ? AND lease_owner = ? AND attempts = ?`,
		id, models.NotificationOutboxChannelLeased, owner, generation).Scan(&leaseUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return s.notificationOutboxChannelOwnershipErrorContext(ctx, id)
	}
	if err != nil {
		return fmt.Errorf("check notification channel lease: %w", err)
	}
	if !leaseUntil.Valid || !normalizeNotificationOutboxLeaseTime(leaseUntil.Time).After(now) {
		return s.notificationOutboxChannelOwnershipErrorContext(ctx, id)
	}
	return nil
}

const notificationOutboxLeaseTimePrecision = time.Microsecond

func normalizeNotificationOutboxLeaseTime(value time.Time) time.Time {
	return value.UTC().Truncate(notificationOutboxLeaseTimePrecision)
}

func notificationOutboxLeaseSQLTime(value time.Time) string {
	// Keep caller precision for the comparison boundary. Lease values are
	// persisted at canonical precision, but a timestamp one nanosecond before
	// expiry must remain distinguishable from expiry itself.
	return value.UTC().Format("2006-01-02 15:04:05.999999999-07:00")
}

// SkipNotificationOutboxChannels marks outstanding digest child rows skipped
// only when the claimed initialized parent has no currently pending mapped
// comments. Sent and already-skipped rows are preserved.
func (s *sqlStore) SkipNotificationOutboxChannels(outboxID, owner string, generation int, now time.Time) error {
	return s.SkipNotificationOutboxChannelsContext(context.Background(), outboxID, owner, generation, now)
}

// SkipNotificationOutboxChannelsContext atomically marks pending or leased
// child rows skipped under the parent owner/generation/expiry fence.
func (s *sqlStore) SkipNotificationOutboxChannelsContext(ctx context.Context, outboxID, owner string, generation int, now time.Time) error {
	if ctx == nil {
		return fmt.Errorf("%w: context is required", ErrInvalidNotificationOutbox)
	}
	if err := validateNotificationOutboxChannelKey(outboxID, "outbox_id"); err != nil {
		return err
	}
	if err := validateOwnerAndID("channel-skip", owner); err != nil {
		return err
	}
	if generation <= 0 {
		return fmt.Errorf("%w: claim generation must be positive", ErrInvalidNotificationOutbox)
	}
	if now.IsZero() {
		return fmt.Errorf("%w: transition time is required", ErrInvalidNotificationOutbox)
	}
	return retryNotificationOutboxBusyContext(ctx, func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin skip notification channels: %w", err)
		}
		defer tx.Rollback() //nolint:errcheck // no-op after successful commit
		parent, digestSpec, err := claimedSiteDigestParentTx(ctx, tx, outboxID, owner, generation, now.UTC())
		if err != nil {
			return err
		}
		pending, err := loadSiteDigestCommentsTx(ctx, tx, parent, digestSpec)
		if err != nil {
			return err
		}
		if len(pending) != 0 {
			return ErrDigestChannelsNotComplete
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE notification_outbox_channels
			 SET status = ?, lease_until = NULL, lease_owner = '', last_error = '', updated_at = ?
			 WHERE outbox_id = ? AND status IN (?, ?)`,
			models.NotificationOutboxChannelSkipped, now.UTC(), outboxID,
			models.NotificationOutboxChannelPending, models.NotificationOutboxChannelLeased); err != nil {
			return fmt.Errorf("skip notification channels: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit skipped notification channels: %w", err)
		}
		return nil
	})
}

func requireNotificationOutboxTx(ctx context.Context, tx *sql.Tx, outboxID string) error {
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM notification_outbox WHERE id = ?`, outboxID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return ErrNotificationOutboxNotFound
	} else if err != nil {
		return fmt.Errorf("check notification outbox parent: %w", err)
	}
	return nil
}

func (s *sqlStore) notificationOutboxChannelOwnershipErrorContext(ctx context.Context, id string) error {
	var status, owner string
	err := s.db.QueryRowContext(ctx, `SELECT status, lease_owner FROM notification_outbox_channels WHERE id = ?`, id).Scan(&status, &owner)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotificationOutboxChannelNotFound
	}
	if err != nil {
		return fmt.Errorf("check notification channel lease: %w", err)
	}
	return ErrNotificationOutboxChannelNotOwned
}

func validateChannelTransition(id, owner string, generation int, now time.Time) error {
	if err := validateOwnerAndID(id, owner); err != nil {
		return err
	}
	if generation <= 0 {
		return fmt.Errorf("%w: claim generation must be positive", ErrInvalidNotificationOutbox)
	}
	if now.IsZero() {
		return fmt.Errorf("%w: transition time is required", ErrInvalidNotificationOutbox)
	}
	return nil
}

func validateNotificationOutboxChannelKey(value, name string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%w: %s is required", ErrInvalidNotificationOutbox, name)
	}
	if len(value) > NotificationOutboxMaxKeyBytes {
		return fmt.Errorf("%w: %s exceeds %d bytes", ErrInvalidNotificationOutbox, name, NotificationOutboxMaxKeyBytes)
	}
	return nil
}

func scanNotificationOutboxChannel(s scanner) (*models.NotificationOutboxChannel, error) {
	item := &models.NotificationOutboxChannel{}
	var leaseUntil sql.NullTime
	err := s.Scan(
		&item.ID, &item.OutboxID, &item.Channel, &item.Status, &item.AvailableAt,
		&leaseUntil, &item.LeaseOwner, &item.Attempts, &item.LastError,
		&item.CreatedAt, &item.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if leaseUntil.Valid {
		normalized := normalizeNotificationOutboxLeaseTime(leaseUntil.Time)
		item.LeaseUntil = &normalized
	}
	return item, nil
}
