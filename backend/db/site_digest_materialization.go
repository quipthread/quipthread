package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/quipthread/quipthread/models"
)

var (
	ErrInvalidSiteDigestParent       = errors.New("site digest: invalid parent outbox")
	ErrDigestMembershipUninitialized = errors.New("site digest: membership is uninitialized")
	ErrDigestMembershipMalformed     = errors.New("site digest: membership is malformed")
	ErrDigestAggregateMalformed      = errors.New("site digest: aggregate key is malformed")
	ErrDigestChannelsNotComplete     = errors.New("site digest: channels are not complete")
)

// MaterializeClaimedSiteDigest reads only the claimed tenant-local parent,
// site, and comments named by immutable membership rows. It intentionally has
// no time-window fallback: old or malformed digests must be reconciled before
// they can be materialized.
func (s *sqlStore) MaterializeClaimedSiteDigest(ctx context.Context, outboxID, owner string, generation int, now time.Time) (*models.SiteDigestMaterialization, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: context is required", ErrInvalidSiteDigestParent)
	}
	if err := validateNotificationOutboxChannelKey(outboxID, "outbox_id"); err != nil {
		return nil, err
	}
	if err := validateOwnerAndID("digest-materialization", owner); err != nil {
		return nil, err
	}
	if generation <= 0 {
		return nil, fmt.Errorf("%w: claim generation must be positive", ErrInvalidNotificationOutbox)
	}
	if now.IsZero() {
		return nil, fmt.Errorf("%w: materialization time is required", ErrInvalidNotificationOutbox)
	}
	return retryNotificationOutboxBusyContextResult(ctx, func() (*models.SiteDigestMaterialization, error) {
		return s.materializeClaimedSiteDigestOnce(ctx, outboxID, owner, generation, now.UTC())
	})
}

func (s *sqlStore) materializeClaimedSiteDigestOnce(ctx context.Context, outboxID, owner string, generation int, now time.Time) (*models.SiteDigestMaterialization, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin site digest materialization: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after successful commit

	parent, digestSpec, err := claimedSiteDigestParentTx(ctx, tx, outboxID, owner, generation, now)
	if err != nil {
		return nil, err
	}
	site, err := scanSite(tx.QueryRowContext(ctx,
		`SELECT id, owner_id, domain, theme, notify_interval, created_at, last_notified_at, NULL
		 FROM sites WHERE id = ?`, parent.SiteID))
	if err != nil {
		return nil, fmt.Errorf("read site digest site: %w", err)
	}
	if site == nil {
		return nil, fmt.Errorf("%w: site %q not found", ErrDigestMembershipMalformed, parent.SiteID)
	}

	comments, err := loadSiteDigestCommentsTx(ctx, tx, parent, digestSpec)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit site digest materialization: %w", err)
	}
	return &models.SiteDigestMaterialization{Outbox: parent, Site: site, Comments: comments}, nil
}

func loadSiteDigestCommentsTx(ctx context.Context, tx *sql.Tx, parent *models.NotificationOutbox, digestSpec siteDigestSpec) ([]*models.Comment, error) {
	membershipRows, err := tx.QueryContext(ctx,
		`SELECT m.comment_id
		 FROM notification_outbox_comments m
		 LEFT JOIN comments c ON c.id = m.comment_id
		 WHERE m.outbox_id = ?
		 ORDER BY c.created_at ASC, c.id ASC, m.comment_id ASC`, parent.ID)
	if err != nil {
		return nil, fmt.Errorf("read site digest membership: %w", err)
	}
	commentIDs := make([]string, 0)
	for membershipRows.Next() {
		var commentID string
		if err := membershipRows.Scan(&commentID); err != nil {
			membershipRows.Close() //nolint:errcheck,gosec // cleanup after scan failure
			return nil, fmt.Errorf("scan site digest membership: %w", err)
		}
		commentIDs = append(commentIDs, commentID)
	}
	if err := membershipRows.Err(); err != nil {
		membershipRows.Close() //nolint:errcheck,gosec // cleanup after rows failure
		return nil, fmt.Errorf("read site digest membership: %w", err)
	}
	if err := membershipRows.Close(); err != nil {
		return nil, fmt.Errorf("close site digest membership: %w", err)
	}
	comments := make([]*models.Comment, 0, len(commentIDs))
	for _, commentID := range commentIDs {
		comment, err := scanCommentWithAuthor(tx.QueryRowContext(ctx,
			`SELECT c.id, c.site_id, c.page_id, c.page_url, c.page_title, c.parent_id,
			        c.user_id, c.content, c.status, c.imported, c.disqus_author, c.created_at, c.updated_at,
			        u.display_name, u.avatar_url
			 FROM comments c LEFT JOIN users u ON u.id = c.user_id
			 WHERE c.id = ?`, commentID))
		if err != nil {
			return nil, fmt.Errorf("read mapped digest comment %q: %w", commentID, err)
		}
		if comment == nil || strings.TrimSpace(comment.ID) == "" || comment.SiteID != parent.SiteID {
			return nil, fmt.Errorf("%w: comment %q is missing or cross-site", ErrDigestMembershipMalformed, commentID)
		}
		if comment.Status != "pending" && comment.Status != "approved" && comment.Status != "rejected" {
			return nil, fmt.Errorf("%w: comment %q has invalid status", ErrDigestMembershipMalformed, commentID)
		}
		commentWindowStart, _, windowErr := siteDigestWindow(comment.CreatedAt, digestSpec.interval)
		if windowErr != nil || commentWindowStart != digestSpec.windowStart {
			return nil, fmt.Errorf("%w: comment %q is outside the parent window", ErrDigestMembershipMalformed, commentID)
		}
		if comment.Status == "pending" {
			comments = append(comments, comment)
		}
	}
	return comments, nil
}

type siteDigestSpec struct {
	interval    int64
	windowStart int64
	windowEnd   time.Time
}

func parseSiteDigestAggregateKey(key string) (siteDigestSpec, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 3 || parts[0] != "site-digest" {
		return siteDigestSpec{}, fmt.Errorf("%w: %q", ErrDigestAggregateMalformed, key)
	}
	interval, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || strconv.FormatInt(interval, 10) != parts[1] {
		return siteDigestSpec{}, fmt.Errorf("%w: interval", ErrDigestAggregateMalformed)
	}
	windowStart, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || strconv.FormatInt(windowStart, 10) != parts[2] {
		return siteDigestSpec{}, fmt.Errorf("%w: window", ErrDigestAggregateMalformed)
	}
	computedStart, windowEnd, err := siteDigestWindow(time.Unix(windowStart, 0), interval)
	if err != nil || computedStart != windowStart {
		return siteDigestSpec{}, fmt.Errorf("%w: interval/window relation", ErrDigestAggregateMalformed)
	}
	return siteDigestSpec{interval: interval, windowStart: windowStart, windowEnd: windowEnd}, nil
}

func claimedSiteDigestParentTx(ctx context.Context, tx *sql.Tx, outboxID, owner string, generation int, now time.Time) (*models.NotificationOutbox, siteDigestSpec, error) {
	parent, err := scanNotificationOutbox(tx.QueryRowContext(ctx,
		`SELECT id, site_id, kind, aggregate_key, status, available_at,
		        lease_until, lease_owner, attempts, last_error, created_at, updated_at,
		        digest_membership_version
		 FROM notification_outbox WHERE id = ?`, outboxID))
	if err != nil {
		return nil, siteDigestSpec{}, fmt.Errorf("read site digest parent: %w", err)
	}
	if parent == nil {
		return nil, siteDigestSpec{}, ErrNotificationOutboxNotFound
	}
	if parent.Kind != NotificationOutboxKindSiteDigest || strings.TrimSpace(parent.SiteID) == "" {
		return nil, siteDigestSpec{}, ErrInvalidSiteDigestParent
	}
	spec, err := parseSiteDigestAggregateKey(parent.AggregateKey)
	if err != nil {
		return nil, siteDigestSpec{}, err
	}
	if !parent.AvailableAt.Equal(spec.windowEnd) {
		return nil, siteDigestSpec{}, fmt.Errorf("%w: available_at does not equal window end", ErrDigestAggregateMalformed)
	}
	if parent.DigestMembershipVersion != 1 {
		return nil, siteDigestSpec{}, ErrDigestMembershipUninitialized
	}
	if parent.Status != models.NotificationOutboxLeased || parent.LeaseOwner != owner || parent.Attempts != generation || parent.LeaseUntil == nil || !parent.LeaseUntil.After(now) {
		return nil, siteDigestSpec{}, ErrNotificationOutboxNotOwned
	}
	return parent, spec, nil
}

// FinalizeClaimedSiteDigest fences the parent lease and marks it sent only
// after every persisted child channel is sent or skipped, or as a no-op when the valid
// initialized digest has no currently pending mapped comments and no channels.
// Child rows are terminal once sent, so the predicate remains safe with
// concurrent channel acknowledgements.
func (s *sqlStore) FinalizeClaimedSiteDigest(ctx context.Context, outboxID, owner string, generation int, now time.Time) error {
	if ctx == nil {
		return fmt.Errorf("%w: context is required", ErrInvalidSiteDigestParent)
	}
	if err := validateNotificationOutboxChannelKey(outboxID, "outbox_id"); err != nil {
		return err
	}
	if err := validateOwnerAndID("digest-finalization", owner); err != nil {
		return err
	}
	if generation <= 0 {
		return fmt.Errorf("%w: claim generation must be positive", ErrInvalidNotificationOutbox)
	}
	if now.IsZero() {
		return fmt.Errorf("%w: finalization time is required", ErrInvalidNotificationOutbox)
	}
	return retryNotificationOutboxBusyContext(ctx, func() error {
		return s.finalizeClaimedSiteDigestOnce(ctx, outboxID, owner, generation, now.UTC())
	})
}

func (s *sqlStore) finalizeClaimedSiteDigestOnce(ctx context.Context, outboxID, owner string, generation int, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin site digest finalization: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after successful commit
	parent, digestSpec, err := claimedSiteDigestParentTx(ctx, tx, outboxID, owner, generation, now)
	if err != nil {
		return err
	}
	var siteExists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM sites WHERE id = ?`, parent.SiteID).Scan(&siteExists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: site %q not found", ErrDigestMembershipMalformed, parent.SiteID)
		}
		return fmt.Errorf("check site digest site: %w", err)
	}
	pendingComments, err := loadSiteDigestCommentsTx(ctx, tx, parent, digestSpec)
	if err != nil {
		return err
	}
	var total, incomplete int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(CASE WHEN status NOT IN (?, ?) THEN 1 ELSE 0 END), 0)
		 FROM notification_outbox_channels WHERE outbox_id = ?`,
		models.NotificationOutboxChannelSent, models.NotificationOutboxChannelSkipped, outboxID).Scan(&total, &incomplete); err != nil {
		return fmt.Errorf("count site digest channels: %w", err)
	}
	if total == 0 && len(pendingComments) > 0 {
		return ErrDigestChannelsNotComplete
	}
	if total > 0 && incomplete != 0 {
		return fmt.Errorf("%w: %d channels remain", ErrDigestChannelsNotComplete, incomplete)
	}
	result, err := tx.ExecContext(ctx,
		`UPDATE notification_outbox
		 SET status = ?, lease_until = NULL, lease_owner = '', last_error = '', updated_at = ?
		 WHERE id = ? AND kind = ? AND status = ? AND lease_owner = ? AND attempts = ? AND lease_until > ?
		   AND NOT EXISTS (SELECT 1 FROM notification_outbox_channels WHERE outbox_id = ? AND status NOT IN (?, ?))`,
		models.NotificationOutboxSent, now, outboxID, NotificationOutboxKindSiteDigest,
		models.NotificationOutboxLeased, owner, generation, now,
		outboxID, models.NotificationOutboxChannelSent, models.NotificationOutboxChannelSkipped)
	if err != nil {
		return fmt.Errorf("finalize site digest: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return ErrNotificationOutboxNotOwned
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit site digest finalization: %w", err)
	}
	return nil
}
