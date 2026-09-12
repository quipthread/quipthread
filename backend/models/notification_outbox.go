package models

import "time"

const (
	NotificationOutboxPending = "pending"
	NotificationOutboxLeased  = "leased"
	NotificationOutboxSent    = "sent"
)

// NotificationOutbox is the tenant-local identity and delivery state for one
// notification. It deliberately has no payload: workers resolve all delivery
// data from the owning tenant store when they process the aggregate.
type NotificationOutbox struct {
	ID                      string     `json:"id"`
	SiteID                  string     `json:"site_id"`
	Kind                    string     `json:"kind"`
	AggregateKey            string     `json:"aggregate_key"`
	Status                  string     `json:"status"`
	DigestMembershipVersion int        `json:"digest_membership_version"`
	AvailableAt             time.Time  `json:"available_at"`
	LeaseUntil              *time.Time `json:"lease_until,omitempty"`
	LeaseOwner              string     `json:"lease_owner,omitempty"`
	Attempts                int        `json:"attempts"`
	LastError               string     `json:"last_error,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

// ApprovalTokenLocator is the control-plane projection of a tenant-local
// approval token. It intentionally contains only the HMAC lookup key and
// routing metadata; raw bearer tokens and comment references stay in the
// tenant database.
type ApprovalTokenLocator struct {
	TokenHash string
	AccountID string
	SiteID    string
	ExpiresAt time.Time
	CreatedAt time.Time
}
