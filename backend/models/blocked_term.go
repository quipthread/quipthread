package models

import "time"

type BlockedTerm struct {
	ID        string    `json:"id"`
	Term      string    `json:"term"`
	CreatedAt time.Time `json:"created_at"`
}
