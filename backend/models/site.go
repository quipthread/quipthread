package models

import "time"

type Site struct {
	ID             string     `json:"id"`
	OwnerID        string     `json:"owner_id"`
	Domain         string     `json:"domain"`
	Theme          string     `json:"theme"`
	CreatedAt      time.Time  `json:"created_at"`
	LastNotifiedAt *time.Time `json:"last_notified_at,omitempty"`
}

type ApprovalToken struct {
	Token     string    `json:"token"`
	CommentID string    `json:"comment_id"`
	ExpiresAt time.Time `json:"expires_at"`
}
