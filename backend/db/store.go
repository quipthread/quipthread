package db

import (
	"context"
	"time"

	"github.com/quipthread/quipthread/models"
)

// ExportFilter controls which comments ExportComments returns.
type ExportFilter struct {
	Status string     // "approved" (default) or "all"
	From   *time.Time // inclusive lower bound on created_at
	To     *time.Time // inclusive upper bound on created_at
	PageID string     // empty = all pages
}

// Store is the database abstraction layer. Both SQLiteStore and TursoStore
// implement this interface so the rest of the codebase is driver-agnostic.
type Store interface {
	// Comments
	GetComment(id string) (*models.Comment, error)
	// ListComments returns approved comments for a page. sort is "newest" (default),
	// "oldest", or "top". userID is optional; when non-empty, UserVoted is populated.
	ListComments(siteID, pageID, sort, userID string, page, pageSize int) ([]*models.Comment, int, error)
	ListAdminComments(siteID, status string, page, pageSize int) ([]*models.Comment, int, error)
	CreateComment(c *models.Comment) error
	UpdateComment(c *models.Comment) error
	DeleteComment(id string) error
	CountApprovedCommentsByUser(userID, siteID string) (int, error)
	// FindDuplicateComment returns an existing comment with identical content
	// posted by the same user to the same page and site after the given cutoff
	// time, or nil if none exists.
	FindDuplicateComment(siteID, userID, pageID, content string, since time.Time) (*models.Comment, error)
	ImportComments(siteID string, comments []*models.Comment) (int, error)
	ExportComments(siteID string, filter ExportFilter) ([]*models.Comment, error)
	// ToggleVote adds an upvote for userID on commentID, or removes it if one
	// already exists. Returns the updated upvote count and whether the user now
	// has an active vote.
	ToggleVote(commentID, userID string) (upvotes int, voted bool, err error)
	// ToggleFlag adds a flag for userID on commentID, or removes it if one
	// already exists. Returns the updated flag count and whether the user now
	// has an active flag.
	ToggleFlag(commentID, userID string) (flags int, flagged bool, err error)

	// Users
	GetUser(id string) (*models.User, error)
	GetUserContext(ctx context.Context, id string) (*models.User, error)
	UpsertUser(u *models.User) error
	UpdateUser(u *models.User) error
	BumpSessionGeneration(userID string, audience ...string) error
	ListUsers(page, pageSize int) ([]*models.User, int, error)

	// Identities
	GetIdentity(provider, providerID string) (*models.UserIdentity, error)
	CreateIdentity(identity *models.UserIdentity) error
	UpdateIdentityPassword(identityID, hash string) error

	// Account management
	ListUserIdentities(userID string) ([]*models.UserIdentity, error)
	GetIdentityByUser(userID, provider string) (*models.UserIdentity, error)
	DeleteUserIdentity(userID, provider string) error
	UpdateUserDisplayName(userID, displayName string) error
	UpdateIdentityUsername(userID, provider, username string) error

	// Turnstile keys (account-level, stored on subscriptions row)
	GetTurnstileKeys() (siteKey, secretKey string, err error)
	SetTurnstileKeys(siteKey, secretKey string) error

	// Sites
	GetSite(id string) (*models.Site, error)
	ListSites() ([]*models.Site, error)
	CountSites() (int, error)
	CreateSite(s *models.Site) error
	UpdateSite(s *models.Site) error
	DeleteSite(id string) error
	UpdateSiteLastNotifiedAt(siteID string, t time.Time) error
	// UpdateSiteSSO sets or clears the SSO secret for a site.
	// Pass nil to disable SSO; pass a non-nil pointer to enable/rotate the secret.
	UpdateSiteSSO(siteID string, secret *string) error

	// Pending comment queries (used by notification dispatcher).
	// since filters to comments created after that time; pass time.Time{} for all pending.
	CountPendingComments(siteID string, since time.Time) (int, error)
	ListPendingComments(siteID string, since time.Time) ([]*models.Comment, error)
	// CreatePendingCommentWithNotification atomically inserts a pending comment
	// and its tenant-local site-digest outbox row.
	CreatePendingCommentWithNotification(c *models.Comment) error

	// Tenant-local notification outbox. The outbox contains only work identity
	// and delivery state; notification payloads are resolved by a later worker.
	EnqueueNotificationOutbox(item *models.NotificationOutbox) error
	GetNotificationOutbox(id string) (*models.NotificationOutbox, error)
	// Claim returns rows whose Attempts value is the lease generation. Ack and
	// retry must use that exact generation while the lease remains unexpired.
	ClaimNotificationOutbox(owner string, now time.Time, leaseFor time.Duration, limit int) ([]*models.NotificationOutbox, error)
	ClaimNotificationOutboxContext(ctx context.Context, owner string, now time.Time, leaseFor time.Duration, limit int) ([]*models.NotificationOutbox, error)
	AckNotificationOutbox(id, owner string, claimedGeneration int, now time.Time) error
	RetryNotificationOutbox(id, owner string, claimedGeneration int, availableAt, now time.Time, lastError string) error
	RetryNotificationOutboxContext(ctx context.Context, id, owner string, claimedGeneration int, availableAt, now time.Time, lastError string) error
	ListUninitializedSiteDigests() ([]*models.NotificationOutbox, error)
	ListUninitializedSiteDigestsContext(ctx context.Context) ([]*models.NotificationOutbox, error)
	ClaimLegacySiteDigestContext(ctx context.Context, outboxID, owner string, now time.Time, leaseFor time.Duration) (*models.NotificationOutbox, error)
	ReconcileLegacySiteDigestContext(ctx context.Context, outboxID, owner string, claimedGeneration int, commentIDs []string, now time.Time) error
	RetireLegacySiteDigestContext(ctx context.Context, outboxID, owner string, claimedGeneration int, now time.Time) error
	// EnsureNotificationOutboxChannels idempotently creates pending child
	// records for the supplied channel identifiers and returns their canonical
	// tenant-local state.
	EnsureNotificationOutboxChannels(outboxID string, channels []string) ([]*models.NotificationOutboxChannel, error)
	EnsureNotificationOutboxChannelsContext(ctx context.Context, outboxID string, channels []string) ([]*models.NotificationOutboxChannel, error)
	ListNotificationOutboxChannels(outboxID string) ([]*models.NotificationOutboxChannel, error)
	ListNotificationOutboxChannelsContext(ctx context.Context, outboxID string) ([]*models.NotificationOutboxChannel, error)
	GetNotificationOutboxChannel(id string) (*models.NotificationOutboxChannel, error)
	GetNotificationOutboxChannelByKey(outboxID, channel string) (*models.NotificationOutboxChannel, error)
	ClaimNotificationOutboxChannels(outboxID, owner string, now time.Time, leaseFor time.Duration, limit int) ([]*models.NotificationOutboxChannel, error)
	ClaimNotificationOutboxChannelsContext(ctx context.Context, outboxID, owner string, now time.Time, leaseFor time.Duration, limit int) ([]*models.NotificationOutboxChannel, error)
	AckNotificationOutboxChannel(id, owner string, claimedGeneration int, now time.Time) error
	AckNotificationOutboxChannelContext(ctx context.Context, id, owner string, claimedGeneration int, now time.Time) error
	RetryNotificationOutboxChannel(id, owner string, claimedGeneration int, availableAt, now time.Time, lastError string) error
	RetryNotificationOutboxChannelContext(ctx context.Context, id, owner string, claimedGeneration int, availableAt, now time.Time, lastError string) error
	SkipNotificationOutboxChannels(outboxID, owner string, claimedGeneration int, now time.Time) error
	SkipNotificationOutboxChannelsContext(ctx context.Context, outboxID, owner string, claimedGeneration int, now time.Time) error
	MaterializeClaimedSiteDigest(ctx context.Context, outboxID, owner string, generation int, now time.Time) (*models.SiteDigestMaterialization, error)
	FinalizeClaimedSiteDigest(ctx context.Context, outboxID, owner string, generation int, now time.Time) error
	EnsureMaterializedSiteDigestApprovalTokens(ctx context.Context, materialized *models.SiteDigestMaterialization, opts SiteDigestApprovalTokenOptions) ([]*models.ApprovalToken, error)

	// Approval tokens
	GetApprovalToken(token string) (*models.ApprovalToken, error)
	CreateApprovalToken(t *models.ApprovalToken) error
	DeleteApprovalToken(token string) error
	// ConsumeApprovalToken is the atomic single-use primitive for approval
	// links. In one transaction it validates the raw token against the tenant
	// store, applies the final comment status ("approved" or "rejected"), and
	// deletes the token. Exactly one concurrent caller wins; losers receive
	// ErrApprovalTokenNotFound (unknown/already consumed) and no mutation.
	// Expired tokens fail with ErrApprovalTokenExpired and are left in place.
	// On success the updated comment is returned.
	ConsumeApprovalToken(token, status string, now time.Time) (*models.Comment, error)

	// Analytics
	// tier: 0=starter, 1=pro, 2=business. siteID="" means all sites (business only).
	GetAnalytics(siteID string, from time.Time, limit int, tier int) (*models.AnalyticsResult, error)

	// Blocked terms (moderation rules)
	ListBlockedTerms() ([]*models.BlockedTerm, error)
	AddBlockedTerm(term string, isRegex bool) (*models.BlockedTerm, error)
	DeleteBlockedTerm(id string) error
	BulkAddBlockedTerms(terms []string) (added int, err error)

	// Subscription (cloud billing)
	GetSubscription() (*models.Subscription, error)
	UpsertSubscription(sub *models.Subscription) error
	CountCommentsThisMonth() (int, error)

	// Email tokens (verification + password reset)
	CreateEmailToken(t *models.EmailToken) error
	GetEmailToken(token string) (*models.EmailToken, error)
	DeleteEmailToken(token string) error
	SetEmailVerified(userID string) error
	UpdatePasswordHashByUser(userID, provider, hash string) error

	Close() error
}

// ApprovalTokenLocatorStore is the narrow control-plane sink used by the
// tenant-local digest approval-token operation. The concrete cloud Store
// satisfies this interface without exposing the rest of the control plane to
// the tenant database package.
type ApprovalTokenLocatorStore interface {
	GetApprovalTokenContext(ctx context.Context, tokenHash string) (*models.ApprovalTokenLocator, error)
	CreateApprovalTokenContext(ctx context.Context, locator *models.ApprovalTokenLocator) error
}
