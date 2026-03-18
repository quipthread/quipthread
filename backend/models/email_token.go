package models

import "time"

type EmailToken struct {
	Token     string
	UserID    string
	Type      string // "verification" | "password_reset"
	ExpiresAt time.Time
}
