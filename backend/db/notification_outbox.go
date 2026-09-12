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

const (
	// NotificationOutboxMaxClaimLimit prevents a caller from turning one lease
	// operation into an unbounded scan/update.
	NotificationOutboxMaxClaimLimit = 100
	NotificationOutboxMaxErrorBytes = 1024
	NotificationOutboxMaxKeyBytes   = 255
)

var (
	ErrInvalidNotificationOutbox       = errors.New("notification outbox: invalid input")
	ErrInvalidNotificationOutboxStatus = errors.New("notification outbox: invalid status")
	ErrNotificationOutboxNotFound      = errors.New("notification outbox: not found")
	ErrNotificationOutboxNotOwned      = errors.New("notification outbox: lease owner mismatch")
	// ErrNotificationOutboxOwnerMismatch is an explicit alias for callers that
	// prefer the state-machine terminology.
	ErrNotificationOutboxOwnerMismatch = ErrNotificationOutboxNotOwned
)

const notificationOutboxBusyRetries = 40

func retryNotificationOutboxBusy(fn func() error) error {
	var err error
	for attempt := 0; attempt < notificationOutboxBusyRetries; attempt++ {
		err = fn()
		if !isNotificationOutboxBusy(err) {
			return err
		}
		time.Sleep(5 * time.Millisecond)
	}
	return err
}

func retryNotificationOutboxBusyContext(ctx context.Context, fn func() error) error {
	for attempt := 0; attempt < notificationOutboxBusyRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := fn()
		if !isNotificationOutboxBusy(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
	return fn()
}

func retryNotificationOutboxBusyResult[T any](fn func() (T, error)) (T, error) {
	var result T
	var err error
	for attempt := 0; attempt < notificationOutboxBusyRetries; attempt++ {
		result, err = fn()
		if !isNotificationOutboxBusy(err) {
			return result, err
		}
		time.Sleep(5 * time.Millisecond)
	}
	return result, err
}

func retryNotificationOutboxBusyContextResult[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	var result T
	for attempt := 0; attempt < notificationOutboxBusyRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		result, err := fn()
		if !isNotificationOutboxBusy(err) {
			return result, err
		}
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
	return fn()
}

func isNotificationOutboxBusy(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") ||
		strings.Contains(message, "database table is locked") ||
		strings.Contains(message, "sqlite_busy") ||
		strings.Contains(message, "sqlite busy")
}

func (s *sqlStore) EnqueueNotificationOutbox(item *models.NotificationOutbox) error {
	if err := validateNotificationOutboxForEnqueue(item); err != nil {
		return err
	}

	now := time.Now().UTC()
	if item.ID == "" {
		item.ID = uuid.NewString()
	}
	if item.AvailableAt.IsZero() {
		item.AvailableAt = now
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.Status = models.NotificationOutboxPending
	item.LeaseUntil = nil
	item.LeaseOwner = ""
	item.Attempts = 0
	item.LastError = ""
	item.DigestMembershipVersion = 0
	item.UpdatedAt = now

	err := retryNotificationOutboxBusy(func() error {
		_, err := s.db.Exec( //nolint:noctx // DB layer; full context threading deferred
			`INSERT INTO notification_outbox
			 (id, site_id, kind, aggregate_key, status, available_at, attempts, last_error, created_at, updated_at, digest_membership_version)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(site_id, kind, aggregate_key) DO NOTHING`,
			item.ID, item.SiteID, item.Kind, item.AggregateKey, item.Status,
			item.AvailableAt.UTC(), item.Attempts, item.LastError,
			item.CreatedAt.UTC(), item.UpdatedAt.UTC(), item.DigestMembershipVersion,
		)
		return err
	})
	if err != nil {
		return fmt.Errorf("enqueue notification outbox: %w", err)
	}

	// Read the canonical row back both for idempotent callers and to avoid
	// exposing a caller-supplied ID when another enqueue won the unique key.
	stored, err := s.GetNotificationOutboxByKey(item.SiteID, item.Kind, item.AggregateKey)
	if err != nil {
		return fmt.Errorf("read enqueued notification outbox: %w", err)
	}
	if stored == nil {
		return fmt.Errorf("read enqueued notification outbox: %w", ErrNotificationOutboxNotFound)
	}
	*item = *stored
	return nil
}

func (s *sqlStore) GetNotificationOutbox(id string) (*models.NotificationOutbox, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("%w: id is required", ErrInvalidNotificationOutbox)
	}
	var item *models.NotificationOutbox
	err := retryNotificationOutboxBusy(func() error {
		var err error
		item, err = scanNotificationOutbox(s.db.QueryRow( //nolint:noctx // DB layer; full context threading deferred
			`SELECT id, site_id, kind, aggregate_key, status, available_at,
			        lease_until, lease_owner, attempts, last_error, created_at, updated_at,
			        digest_membership_version
			 FROM notification_outbox WHERE id = ?`, id))
		return err
	})
	return item, err
}

func (s *sqlStore) GetNotificationOutboxByKey(siteID, kind, aggregateKey string) (*models.NotificationOutbox, error) {
	var item *models.NotificationOutbox
	err := retryNotificationOutboxBusy(func() error {
		var err error
		item, err = scanNotificationOutbox(s.db.QueryRow( //nolint:noctx // DB layer; full context threading deferred
			`SELECT id, site_id, kind, aggregate_key, status, available_at,
			        lease_until, lease_owner, attempts, last_error, created_at, updated_at,
			        digest_membership_version
			 FROM notification_outbox
			 WHERE site_id = ? AND kind = ? AND aggregate_key = ?`,
			siteID, kind, aggregateKey))
		return err
	})
	return item, err
}

// ListUninitializedSiteDigests is the pre-enable reconciliation inventory.
// Rows are never mutated or timestamp-backfilled by this method.
func (s *sqlStore) ListUninitializedSiteDigests() ([]*models.NotificationOutbox, error) {
	return s.ListUninitializedSiteDigestsContext(context.Background())
}

// ClaimNotificationOutbox atomically leases up to limit ready rows. Expired
// leases are eligible for the same claim operation, so a crashed worker does
// not require a separate recovery pass. SQLite serializes the UPDATE write;
// the eligibility predicate is part of that UPDATE, preventing duplicate
// concurrent claimants.
func (s *sqlStore) ClaimNotificationOutbox(owner string, now time.Time, leaseFor time.Duration, limit int) ([]*models.NotificationOutbox, error) {
	return s.ClaimNotificationOutboxContext(context.Background(), owner, now, leaseFor, limit)
}

// ClaimNotificationOutboxContext atomically leases ready parent rows using
// the caller's cancellation context.
func (s *sqlStore) ClaimNotificationOutboxContext(ctx context.Context, owner string, now time.Time, leaseFor time.Duration, limit int) ([]*models.NotificationOutbox, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: context is required", ErrInvalidNotificationOutbox)
	}
	if strings.TrimSpace(owner) == "" {
		return nil, fmt.Errorf("%w: owner is required", ErrInvalidNotificationOutbox)
	}
	if leaseFor <= 0 {
		return nil, fmt.Errorf("%w: lease duration must be positive", ErrInvalidNotificationOutbox)
	}
	if limit <= 0 || limit > NotificationOutboxMaxClaimLimit {
		return nil, fmt.Errorf("%w: claim limit must be between 1 and %d", ErrInvalidNotificationOutbox, NotificationOutboxMaxClaimLimit)
	}

	return retryNotificationOutboxBusyContextResult(ctx, func() ([]*models.NotificationOutbox, error) {
		return s.claimNotificationOutboxOnce(ctx, owner, now.UTC(), leaseFor, limit)
	})
}

func (s *sqlStore) claimNotificationOutboxOnce(ctx context.Context, owner string, now time.Time, leaseFor time.Duration, limit int) ([]*models.NotificationOutbox, error) {
	leaseUntil := now.Add(leaseFor)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin claim notification outbox: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after successful commit

	rows, err := tx.QueryContext(ctx,
		`UPDATE notification_outbox
		 SET status = ?, lease_until = ?, lease_owner = ?, attempts = attempts + 1, updated_at = ?
		 WHERE id IN (
		   SELECT id FROM notification_outbox
		   WHERE (status = ? AND available_at <= ?)
		      OR (status = ? AND lease_until IS NOT NULL AND lease_until <= ?)
		   ORDER BY available_at ASC, created_at ASC, id ASC
		   LIMIT ?
		 )
		 RETURNING id, site_id, kind, aggregate_key, status, available_at,
		           lease_until, lease_owner, attempts, last_error, created_at, updated_at,
		           digest_membership_version`,
		models.NotificationOutboxLeased, leaseUntil, owner, now,
		models.NotificationOutboxPending, now,
		models.NotificationOutboxLeased, now, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("claim notification outbox: %w", err)
	}

	claimed := make([]*models.NotificationOutbox, 0, limit)
	for rows.Next() {
		item, scanErr := scanNotificationOutbox(rows)
		if scanErr != nil {
			rows.Close() //nolint:errcheck,gosec // cleanup after scan failure
			return nil, fmt.Errorf("scan claimed notification outbox: %w", scanErr)
		}
		claimed = append(claimed, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close() //nolint:errcheck,gosec // cleanup after rows failure
		return nil, fmt.Errorf("read claimed notification outbox: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close claimed notification outbox: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit claim notification outbox: %w", err)
	}

	// UPDATE ... RETURNING does not promise result ordering. Make the API
	// deterministic even when a driver returns changed rows in rowid order.
	sort.Slice(claimed, func(i, j int) bool {
		if claimed[i].AvailableAt.Equal(claimed[j].AvailableAt) {
			if claimed[i].CreatedAt.Equal(claimed[j].CreatedAt) {
				return claimed[i].ID < claimed[j].ID
			}
			return claimed[i].CreatedAt.Before(claimed[j].CreatedAt)
		}
		return claimed[i].AvailableAt.Before(claimed[j].AvailableAt)
	})
	return claimed, nil
}

func (s *sqlStore) AckNotificationOutbox(id, owner string, generation int, now time.Time) error {
	if err := validateOwnerAndID(id, owner); err != nil {
		return err
	}
	if generation <= 0 {
		return fmt.Errorf("%w: claim generation must be positive", ErrInvalidNotificationOutbox)
	}
	if now.IsZero() {
		return fmt.Errorf("%w: acknowledgement time is required", ErrInvalidNotificationOutbox)
	}
	if item, err := s.GetNotificationOutbox(id); err != nil {
		return err
	} else if item != nil && item.Kind == NotificationOutboxKindSiteDigest {
		return s.FinalizeClaimedSiteDigest(context.Background(), id, owner, generation, now)
	}
	res, err := s.db.Exec( //nolint:noctx // DB layer; full context threading deferred
		`UPDATE notification_outbox
		 SET status = ?, lease_until = NULL, lease_owner = '', last_error = '', updated_at = ?
		 WHERE id = ? AND status = ? AND lease_owner = ? AND attempts = ? AND lease_until > ?
		   AND (kind <> ? OR (digest_membership_version = 1
		       AND EXISTS (SELECT 1 FROM notification_outbox_channels WHERE outbox_id = notification_outbox.id)
		       AND NOT EXISTS (SELECT 1 FROM notification_outbox_channels WHERE outbox_id = notification_outbox.id AND status <> ?)))`,
		models.NotificationOutboxSent, now.UTC(), id,
		models.NotificationOutboxLeased, owner, generation, now.UTC(),
		NotificationOutboxKindSiteDigest, models.NotificationOutboxChannelSent,
	)
	if err != nil {
		return fmt.Errorf("ack notification outbox: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return nil
	}
	return s.notificationOutboxOwnershipError(id)
}

func (s *sqlStore) RetryNotificationOutbox(id, owner string, generation int, availableAt, now time.Time, lastError string) error {
	return s.RetryNotificationOutboxContext(context.Background(), id, owner, generation, availableAt, now, lastError)
}

// RetryNotificationOutboxContext returns a leased parent to pending using the
// caller's cancellation context and exact lease generation.
func (s *sqlStore) RetryNotificationOutboxContext(ctx context.Context, id, owner string, generation int, availableAt, now time.Time, lastError string) error {
	if ctx == nil {
		return fmt.Errorf("%w: context is required", ErrInvalidNotificationOutbox)
	}
	if err := validateOwnerAndID(id, owner); err != nil {
		return err
	}
	if generation <= 0 {
		return fmt.Errorf("%w: claim generation must be positive", ErrInvalidNotificationOutbox)
	}
	if now.IsZero() {
		return fmt.Errorf("%w: retry time is required", ErrInvalidNotificationOutbox)
	}
	if availableAt.IsZero() || !availableAt.After(now) {
		return fmt.Errorf("%w: retry availability must be after retry time", ErrInvalidNotificationOutbox)
	}
	if len(lastError) > NotificationOutboxMaxErrorBytes {
		return fmt.Errorf("%w: last error exceeds %d bytes", ErrInvalidNotificationOutbox, NotificationOutboxMaxErrorBytes)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE notification_outbox
		 SET status = ?, available_at = ?, lease_until = NULL, lease_owner = '', last_error = ?, updated_at = ?
		 WHERE id = ? AND status = ? AND lease_owner = ? AND attempts = ? AND lease_until > ?`,
		models.NotificationOutboxPending, availableAt.UTC(), lastError, now.UTC(), id,
		models.NotificationOutboxLeased, owner, generation, now.UTC(),
	)
	if err != nil {
		return fmt.Errorf("retry notification outbox: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return nil
	}
	return s.notificationOutboxOwnershipErrorContext(ctx, id)
}

func (s *sqlStore) notificationOutboxOwnershipError(id string) error {
	return s.notificationOutboxOwnershipErrorContext(context.Background(), id)
}

func (s *sqlStore) notificationOutboxOwnershipErrorContext(ctx context.Context, id string) error {
	var status, owner string
	err := s.db.QueryRowContext(ctx,
		`SELECT status, lease_owner FROM notification_outbox WHERE id = ?`, id,
	).Scan(&status, &owner)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotificationOutboxNotFound
	}
	if err != nil {
		return fmt.Errorf("check notification outbox lease: %w", err)
	}
	return ErrNotificationOutboxNotOwned
}

func validateNotificationOutboxForEnqueue(item *models.NotificationOutbox) error {
	if item == nil {
		return fmt.Errorf("%w: item is nil", ErrInvalidNotificationOutbox)
	}
	if item.Status != "" && item.Status != models.NotificationOutboxPending {
		return ErrInvalidNotificationOutboxStatus
	}
	if item.Attempts != 0 || item.LeaseUntil != nil || item.LeaseOwner != "" || item.LastError != "" {
		return fmt.Errorf("%w: enqueue state must be empty and pending", ErrInvalidNotificationOutbox)
	}
	if item.DigestMembershipVersion < 0 || item.DigestMembershipVersion > 1 {
		return fmt.Errorf("%w: invalid digest membership version", ErrInvalidNotificationOutbox)
	}
	for name, value := range map[string]string{
		"site_id":       item.SiteID,
		"kind":          item.Kind,
		"aggregate_key": item.AggregateKey,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%w: %s is required", ErrInvalidNotificationOutbox, name)
		}
		if len(value) > NotificationOutboxMaxKeyBytes {
			return fmt.Errorf("%w: %s exceeds %d bytes", ErrInvalidNotificationOutbox, name, NotificationOutboxMaxKeyBytes)
		}
	}
	if item.ID != "" && (strings.TrimSpace(item.ID) == "" || len(item.ID) > NotificationOutboxMaxKeyBytes) {
		return fmt.Errorf("%w: invalid id", ErrInvalidNotificationOutbox)
	}
	return nil
}

func validateOwnerAndID(id, owner string) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(owner) == "" {
		return fmt.Errorf("%w: id and owner are required", ErrInvalidNotificationOutbox)
	}
	if len(owner) > NotificationOutboxMaxKeyBytes {
		return fmt.Errorf("%w: owner exceeds %d bytes", ErrInvalidNotificationOutbox, NotificationOutboxMaxKeyBytes)
	}
	return nil
}

func scanNotificationOutbox(s scanner) (*models.NotificationOutbox, error) {
	item := &models.NotificationOutbox{}
	var leaseUntil sql.NullTime
	err := s.Scan(
		&item.ID, &item.SiteID, &item.Kind, &item.AggregateKey, &item.Status,
		&item.AvailableAt, &leaseUntil, &item.LeaseOwner, &item.Attempts,
		&item.LastError, &item.CreatedAt, &item.UpdatedAt,
		&item.DigestMembershipVersion,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if leaseUntil.Valid {
		item.LeaseUntil = &leaseUntil.Time
	}
	return item, nil
}
