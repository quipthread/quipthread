package models

import "time"

type User struct {
	ID            string    `json:"id"`
	DisplayName   string    `json:"display_name"`
	Email         string    `json:"email,omitempty"`
	AvatarURL     string    `json:"avatar_url,omitempty"`
	Role          string    `json:"role"` // commenter | admin
	Banned        bool      `json:"banned"`
	ShadowBanned  bool      `json:"shadow_banned"`
	EmailVerified bool      `json:"email_verified"`
	CreatedAt     time.Time `json:"created_at"`
}
