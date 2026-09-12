package models

import "time"

const (
	NotificationOutboxChannelPending = NotificationOutboxPending
	NotificationOutboxChannelLeased  = NotificationOutboxLeased
	NotificationOutboxChannelSent    = NotificationOutboxSent
	NotificationOutboxChannelSkipped = "skipped"
)

// NotificationOutboxChannel is the tenant-local delivery state for one
// channel of a notification outbox item. It deliberately contains no
// recipient, provider, comment, token, or credential data.
type NotificationOutboxChannel struct {
	ID          string     `json:"id"`
	OutboxID    string     `json:"outbox_id"`
	Channel     string     `json:"channel"`
	Status      string     `json:"status"`
	AvailableAt time.Time  `json:"available_at"`
	LeaseUntil  *time.Time `json:"lease_until,omitempty"`
	LeaseOwner  string     `json:"lease_owner,omitempty"`
	Attempts    int        `json:"attempts"`
	LastError   string     `json:"last_error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
