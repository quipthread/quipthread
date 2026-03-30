package cloud

import "time"

type TeamMember struct {
	ID          string
	AccountID   string // owner's account ID
	Email       string
	Role        string // "admin"
	InviteToken string
	Accepted    bool
	InvitedAt   time.Time
	AcceptedAt  *time.Time
}

type Account struct {
	ID               string
	Email            string
	PasswordHash     string
	EmailVerified    bool
	Plan             string // hobby, starter, pro, business
	DBType           string // sqlite, turso
	DBURL            string // path or turso URL
	StripeCustomerID string // set when user completes Stripe checkout
	CreatedAt        time.Time
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

type Store interface {
	// Account management
	CreateAccount(acc *Account) error
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
}
