package db

import (
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
	ListComments(siteID, pageID string, page, pageSize int) ([]*models.Comment, int, error)
	ListAdminComments(siteID, status string, page, pageSize int) ([]*models.Comment, int, error)
	CreateComment(c *models.Comment) error
	UpdateComment(c *models.Comment) error
	DeleteComment(id string) error
	CountApprovedCommentsByUser(userID, siteID string) (int, error)
	ImportComments(siteID string, comments []*models.Comment) (int, error)
	ExportComments(siteID string, filter ExportFilter) ([]*models.Comment, error)

	// Users
	GetUser(id string) (*models.User, error)
	UpsertUser(u *models.User) error
	UpdateUser(u *models.User) error
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
	UpdateSiteLastNotifiedAt(siteID string, t time.Time) error

	// Pending comment queries (used by notification dispatcher)
	CountPendingComments(siteID string) (int, error)
	ListPendingComments(siteID string) ([]*models.Comment, error)

	// Approval tokens
	GetApprovalToken(token string) (*models.ApprovalToken, error)
	CreateApprovalToken(t *models.ApprovalToken) error
	DeleteApprovalToken(token string) error

	// Analytics
	// tier: 0=starter, 1=pro, 2=business. siteID="" means all sites (business only).
	GetAnalytics(siteID string, from time.Time, limit int, tier int) (*models.AnalyticsResult, error)

	// Blocked terms (moderation rules)
	ListBlockedTerms() ([]*models.BlockedTerm, error)
	AddBlockedTerm(term string) (*models.BlockedTerm, error)
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
