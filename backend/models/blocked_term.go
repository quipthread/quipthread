package models

import "time"

type BlockedTerm struct {
	ID        string    `json:"id"`
	Term      string    `json:"term"`
	IsRegex   bool      `json:"is_regex"`
	CreatedAt time.Time `json:"created_at"`
}
