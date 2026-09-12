package db

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/quipthread/quipthread/models"
)

const (
	DefaultSiteDigestIntervalSeconds = 5 * 60
	MinSiteDigestIntervalSeconds     = 60
	MaxSiteDigestIntervalSeconds     = 24 * 60 * 60
	NotificationOutboxKindSiteDigest = "site_digest"
)

var (
	ErrInvalidPendingComment     = errors.New("pending comment: invalid comment state")
	ErrInvalidSiteDigestInterval = errors.New("site digest: invalid notification interval")
	ErrDigestOutboxNotPending    = errors.New("site digest: existing outbox is not pending")
)

// CreatePendingCommentWithNotification is the dedicated cloud comment-create
// transaction. It stores no notification payload: the outbox row identifies a
// site and completed digest window only.
func (s *sqlStore) CreatePendingCommentWithNotification(c *models.Comment) error {
	if c == nil || c.Status != "pending" {
		return ErrInvalidPendingComment
	}

	now := time.Now().UTC()
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	outboxID := uuid.NewString()

	// The ID, outbox ID, and both timestamps are deliberately initialized before
	// the retry loop. A retry is the same logical create operation, not a new
	// comment.
	return retryNotificationOutboxBusy(func() error {
		return s.createPendingCommentWithNotificationOnce(c, outboxID, now)
	})
}

func (s *sqlStore) createPendingCommentWithNotificationOnce(c *models.Comment, outboxID string, now time.Time) error {
	tx, err := s.db.Begin() //nolint:noctx // DB layer; full context threading deferred
	if err != nil {
		return fmt.Errorf("begin pending comment transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after successful commit

	intervalSeconds, err := siteDigestInterval(tx, c.SiteID)
	if err != nil {
		return fmt.Errorf("read site digest interval: %w", err)
	}
	windowStart, windowEnd, err := siteDigestWindow(c.CreatedAt, intervalSeconds)
	if err != nil {
		return err
	}
	outbox := &models.NotificationOutbox{
		ID:                      outboxID,
		SiteID:                  c.SiteID,
		Kind:                    NotificationOutboxKindSiteDigest,
		AggregateKey:            fmt.Sprintf("site-digest:%d:%d", intervalSeconds, windowStart),
		Status:                  models.NotificationOutboxPending,
		DigestMembershipVersion: 1,
		AvailableAt:             windowEnd,
		CreatedAt:               c.CreatedAt,
		UpdatedAt:               now,
	}

	if err := insertCommentTx(tx, c); err != nil {
		return fmt.Errorf("insert pending comment: %w", err)
	}
	canonicalOutbox, err := ensurePendingDigestOutboxTx(tx, outbox)
	if err != nil {
		return fmt.Errorf("enqueue pending comment digest: %w", err)
	}
	if err := insertDigestMembershipTx(tx, canonicalOutbox.ID, c.ID); err != nil {
		return fmt.Errorf("record pending comment digest membership: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit pending comment transaction: %w", err)
	}
	return nil
}

func siteDigestInterval(tx *sql.Tx, siteID string) (int64, error) {
	var configured sql.NullInt64
	if err := tx.QueryRow(`SELECT notify_interval FROM sites WHERE id = ?`, siteID).Scan(&configured); err != nil { //nolint:noctx // Legacy comment-create API supplies no caller context.
		return 0, err
	}
	if !configured.Valid {
		return DefaultSiteDigestIntervalSeconds, nil
	}
	if configured.Int64 < MinSiteDigestIntervalSeconds || configured.Int64 > MaxSiteDigestIntervalSeconds {
		return 0, fmt.Errorf("%w: %d seconds (allowed %d-%d)", ErrInvalidSiteDigestInterval, configured.Int64, MinSiteDigestIntervalSeconds, MaxSiteDigestIntervalSeconds)
	}
	return configured.Int64, nil
}

func siteDigestWindow(createdAt time.Time, intervalSeconds int64) (int64, time.Time, error) {
	if intervalSeconds < MinSiteDigestIntervalSeconds || intervalSeconds > MaxSiteDigestIntervalSeconds {
		return 0, time.Time{}, fmt.Errorf("%w: %d seconds (allowed %d-%d)", ErrInvalidSiteDigestInterval, intervalSeconds, MinSiteDigestIntervalSeconds, MaxSiteDigestIntervalSeconds)
	}
	createdUnix := createdAt.UTC().Unix()
	remainder := createdUnix % intervalSeconds
	if remainder < 0 {
		remainder += intervalSeconds
	}
	if createdUnix < math.MinInt64+remainder {
		return 0, time.Time{}, fmt.Errorf("%w: window start underflow", ErrInvalidSiteDigestInterval)
	}
	windowStart := createdUnix - remainder
	if windowStart > math.MaxInt64-intervalSeconds {
		return 0, time.Time{}, fmt.Errorf("%w: window end overflow", ErrInvalidSiteDigestInterval)
	}
	return windowStart, time.Unix(windowStart+intervalSeconds, 0).UTC(), nil
}

func ensurePendingDigestOutboxTx(tx *sql.Tx, item *models.NotificationOutbox) (*models.NotificationOutbox, error) {
	var canonical models.NotificationOutbox
	err := tx.QueryRow( //nolint:noctx // Legacy comment-create API supplies no caller context.
		`SELECT id, status, digest_membership_version FROM notification_outbox WHERE site_id = ? AND kind = ? AND aggregate_key = ?`,
		item.SiteID, item.Kind, item.AggregateKey,
	).Scan(&canonical.ID, &canonical.Status, &canonical.DigestMembershipVersion)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil && canonical.DigestMembershipVersion != 1 {
		return nil, ErrDigestMembershipUninitialized
	}
	if err == nil && canonical.Status != models.NotificationOutboxPending {
		return nil, fmt.Errorf("%w: status=%s", ErrDigestOutboxNotPending, canonical.Status)
	}
	if errors.Is(err, sql.ErrNoRows) {
		if err := insertNotificationOutboxTx(tx, item); err != nil {
			return nil, err
		}
		if err := tx.QueryRow( //nolint:noctx // Legacy comment-create API supplies no caller context.
			`SELECT id, status, digest_membership_version FROM notification_outbox WHERE site_id = ? AND kind = ? AND aggregate_key = ?`,
			item.SiteID, item.Kind, item.AggregateKey,
		).Scan(&canonical.ID, &canonical.Status, &canonical.DigestMembershipVersion); err != nil {
			return nil, err
		}
		if canonical.Status != models.NotificationOutboxPending {
			return nil, fmt.Errorf("%w: status=%s", ErrDigestOutboxNotPending, canonical.Status)
		}
	}
	return &canonical, nil
}

func insertDigestMembershipTx(tx *sql.Tx, outboxID, commentID string) error {
	if strings.TrimSpace(outboxID) == "" || strings.TrimSpace(commentID) == "" {
		return fmt.Errorf("%w: outbox and comment IDs are required", ErrInvalidPendingComment)
	}
	_, err := tx.Exec( //nolint:noctx // Legacy comment-create API supplies no caller context.
		`INSERT INTO notification_outbox_comments (outbox_id, comment_id)
		 VALUES (?, ?)
		 ON CONFLICT(outbox_id, comment_id) DO NOTHING`, outboxID, commentID)
	return err
}

func insertCommentTx(tx *sql.Tx, c *models.Comment) error {
	_, err := tx.Exec( //nolint:noctx // Legacy comment-create API supplies no caller context.
		`INSERT INTO comments
		 (id, site_id, page_id, page_url, page_title, parent_id, user_id, content, status, imported, disqus_author, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.SiteID, c.PageID, c.PageURL, c.PageTitle,
		nullStr(c.ParentID), c.UserID, c.Content, c.Status,
		boolInt(c.Imported), nullStr(c.DisqusAuthor), c.CreatedAt, c.UpdatedAt,
	)
	return err
}

func insertNotificationOutboxTx(tx *sql.Tx, item *models.NotificationOutbox) error {
	if err := validateNotificationOutboxForEnqueue(item); err != nil {
		return err
	}
	_, err := tx.Exec( //nolint:noctx // Legacy comment-create API supplies no caller context.
		`INSERT INTO notification_outbox
		 (id, site_id, kind, aggregate_key, status, available_at, attempts, last_error, created_at, updated_at, digest_membership_version)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(site_id, kind, aggregate_key) DO NOTHING`,
		item.ID, item.SiteID, item.Kind, item.AggregateKey, item.Status,
		item.AvailableAt.UTC(), item.Attempts, item.LastError,
		item.CreatedAt.UTC(), item.UpdatedAt.UTC(), item.DigestMembershipVersion,
	)
	return err
}
