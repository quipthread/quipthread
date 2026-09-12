package cloud

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/quipthread/quipthread/models"
)

// Site registry lifecycle states. The registry is dark-deployment control-plane
// infrastructure: rows are reserved/activated ahead of any production routing.
const (
	SiteStatusReserved = "reserved" // claimed by an account, not yet serving
	SiteStatusActive   = "active"   // registered and serving
	SiteStatusInactive = "inactive" // deactivated but ownership retained
)

// Account provisioning states are deliberately separate from site-registry
// lifecycle states. Only ready accounts may be used as tenant locators.
const (
	AccountProvisioning = "provisioning"
	AccountUnknown      = "unknown"
	AccountReady        = "ready"
)

var (
	// ErrSiteRegistryConflict is returned when a site is already owned by a
	// different account. Existing ownership is never modified.
	ErrSiteRegistryConflict = errors.New("site registry: site already registered to a different account")
	// ErrSiteRegistryNotFound is returned when a lifecycle transition targets
	// a site that has no registry entry.
	ErrSiteRegistryNotFound = errors.New("site registry: entry not found")
	// ErrApprovalTokenConflict is returned when creating an approval-token
	// locator whose hash already exists. The existing row is never modified.
	ErrApprovalTokenConflict = errors.New("approval token: hash already exists")
	// ErrApprovalTokenNotFound is returned when deleting/consuming an
	// approval-token locator that does not exist (or was already consumed).
	ErrApprovalTokenNotFound       = errors.New("approval token: entry not found")
	ErrInvalidAccountLocatorCursor = errors.New("account locator: invalid cursor")
	ErrInvalidAccountLocatorLimit  = errors.New("account locator: invalid page size")
	ErrInvalidAccountID            = errors.New("account locator: invalid account ID")
	ErrProvisioningInProgress      = errors.New("account provisioning is already claimed")
	ErrProvisioningClaimLost       = errors.New("account provisioning claim is no longer current")
)

type TeamMember struct {
	ID          string     `json:"id"`
	AccountID   string     `json:"account_id"` // owner's account ID
	Email       string     `json:"email"`
	Role        string     `json:"role"` // "admin"
	InviteToken string     `json:"invite_token"`
	Accepted    bool       `json:"accepted"`
	InvitedAt   time.Time  `json:"invited_at"`
	AcceptedAt  *time.Time `json:"accepted_at"`
}

type Account struct {
	ID                         string
	Email                      string
	PasswordHash               string
	EmailVerified              bool
	Plan                       string // hobby, starter, pro, business
	DBType                     string // sqlite, turso
	DBURL                      string // path or turso URL
	ProvisioningStatus         string // provisioning, unknown, ready
	ProvisioningTarget         string // stable Turso database name
	ProvisioningOwner          string // opaque durable claim owner token
	ProvisioningGeneration     int64
	ProvisioningLeaseExpiresAt time.Time
	StripeCustomerID           string // set when user completes Stripe checkout
	CreatedAt                  time.Time
}

// AccountLocator is the complete control-plane projection needed to open a
// tenant store. It intentionally contains no personal, billing, or
// authentication data.
type AccountLocator struct {
	ID     string
	DBType string
	DBURL  string
}

type ProvisioningClaim struct {
	Account    *Account
	Won        bool
	Generation int64
}

const DefaultProvisioningLease = 5 * time.Minute

const (
	MaxAccountLocatorPageSize    = 100
	MaxAccountLocatorCursorBytes = 255
)

// ValidateAccountID is the shared control-plane account-ID invariant used by
// both locator production and dispatcher page validation.
func ValidateAccountID(id string) error {
	if strings.TrimSpace(id) != id || id == "" || len(id) > MaxAccountLocatorCursorBytes {
		return ErrInvalidAccountID
	}
	return nil
}

type OAuthLink struct {
	AccountID      string
	Provider       string // github, google
	ProviderUserID string
	Email          string
}

type EmailToken struct {
	AccountID string
	Token     string
	ExpiresAt time.Time
	Purpose   string // verify, reset
}

// SiteRegistryEntry is a master control-plane record mapping a site to the
// account that owns it, independent of per-tenant databases.
type SiteRegistryEntry struct {
	SiteID    string
	AccountID string
	Status    string // SiteStatusReserved, SiteStatusActive, SiteStatusInactive
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ApprovalToken is a cloud control-plane routing locator for a comment-approval
// bearer token. Only the HMAC-SHA-256 hash of the token is stored; raw tokens
// never reach the database and are never returned by this struct. The locator
// deliberately carries no comment reference: comment association lives only in
// the customer's tenant store alongside the raw token. Central-locator cleanup
// after a successful tenant mutation is best-effort and owned by a later
// controller lane, not by this data layer.
type ApprovalToken = models.ApprovalTokenLocator

type Store interface {
	// Account management
	CreateAccount(acc *Account) error
	// CreateOrGetProvisioningAccount atomically reserves an account by email.
	// It returns the existing row when another request already reserved it.
	CreateOrGetProvisioningAccount(ctx context.Context, acc *Account) (*Account, error)
	ClaimAccountProvisioning(ctx context.Context, id, owner string, now time.Time, lease time.Duration) (*ProvisioningClaim, error)
	FinalizeAccountProvisioning(ctx context.Context, id, owner string, generation int64, now time.Time, status, target, dbURL string) error
	// ListAccounts enumerates every managed account in deterministic order
	// (ascending ID). Used by control-plane maintenance jobs such as the
	// site-registry reconciler to scan every tenant.
	ListAccounts() ([]*Account, error)
	// ListAccountLocators returns at most limit eligible tenant-opening
	// projections with IDs strictly greater than cursor, ordered by ID. The
	// query honors ctx. An empty next cursor is the explicit end-of-results
	// value.
	ListAccountLocators(ctx context.Context, cursor string, limit int) (locators []*AccountLocator, nextCursor string, err error)
	GetAccountByID(id string) (*Account, error)
	GetAccountByEmail(email string) (*Account, error)
	GetAccountByStripeCustomerID(stripeCustomerID string) (*Account, error)
	UpdateAccountEmailVerified(id string) error
	UpdateAccountPassword(id string, passwordHash string) error
	UpdateAccountPlan(id string, plan, dbType, dbURL string) error
	UpdateAccountStripeCustomer(id, stripeCustomerID string) error

	// OAuth linking
	GetOAuthLink(provider, providerUserID string) (*OAuthLink, error)
	CreateOAuthLink(link *OAuthLink) error

	// Email tokens
	CreateEmailToken(tok *EmailToken) error
	GetEmailToken(token string) (*EmailToken, error)
	DeleteEmailToken(token string) error

	// Team members (Business+ cloud feature)
	CreateTeamMember(m *TeamMember) error
	ListTeamMembers(accountID string) ([]*TeamMember, error)
	GetTeamMemberByToken(token string) (*TeamMember, error)
	GetTeamMemberByEmail(accountID, email string) (*TeamMember, error)
	GetAcceptedInviteByEmail(email string) (*TeamMember, error)
	AcceptTeamMember(token string) error
	DeleteTeamMember(accountID, memberID string) error
	CountTeamMembers(accountID string) (int, error)

	// Site registry (dark-deployment control plane; not traffic-facing yet).
	// Reserve is idempotent for the same site+account and returns
	// ErrSiteRegistryConflict for a different account without mutating
	// existing ownership.
	ReserveSiteRegistration(siteID, accountID string, now time.Time) error
	ActivateSiteRegistration(siteID, accountID string, now time.Time) error
	GetSiteRegistration(siteID string) (*SiteRegistryEntry, error)
	GetActiveSiteRegistration(siteID string) (*SiteRegistryEntry, error)
	DeactivateSiteRegistration(siteID, accountID string, now time.Time) error
	DeleteSiteRegistration(siteID, accountID string) error
	ListSiteRegistrations() ([]*SiteRegistryEntry, error)

	// Approval-token locators (cloud control plane). Only HMAC-SHA-256 token
	// hashes are accepted and stored; raw tokens, comment content, and comment
	// IDs never reach control-plane persistence. Create returns
	// ErrApprovalTokenConflict when the hash already exists; Delete returns
	// ErrApprovalTokenNotFound for unknown hashes; Get returns (nil, nil) when
	// no locator matches.
	CreateApprovalToken(tok *ApprovalToken) error
	GetApprovalToken(tokenHash string) (*ApprovalToken, error)
	CreateApprovalTokenContext(ctx context.Context, tok *ApprovalToken) error
	GetApprovalTokenContext(ctx context.Context, tokenHash string) (*ApprovalToken, error)
	DeleteApprovalToken(tokenHash string) error
	ListApprovalTokens() ([]*ApprovalToken, error)
}
