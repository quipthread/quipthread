package models

import "time"

type Comment struct {
	ID           string    `json:"id"`
	SiteID       string    `json:"site_id"`
	PageID       string    `json:"page_id"`
	PageURL      string    `json:"page_url,omitempty"`
	PageTitle    string    `json:"page_title,omitempty"`
	ParentID     string    `json:"parent_id,omitempty"`
	UserID       string    `json:"user_id"`
	Content      string    `json:"content"`
	Status       string    `json:"status"` // pending | approved | rejected
	Imported     bool      `json:"imported"`
	DisqusAuthor string    `json:"disqus_author,omitempty"`
	AuthorName   string    `json:"author_name,omitempty"`
	AuthorAvatar string    `json:"author_avatar,omitempty"`
	Upvotes      int       `json:"upvotes"`
	UserVoted    bool      `json:"user_voted"`
	Flags        int       `json:"flags,omitempty"`
	UserFlagged  bool      `json:"user_flagged"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
