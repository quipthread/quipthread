package importer

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/quipthread/quipthread/models"
	"github.com/quipthread/quipthread/sanitize"
)

// NativeExport is the canonical Quipthread import/export format.
// The same schema is produced by the export endpoint, making it the universal
// adapter — users run a one-time script to convert from any source to this
// format and then import it here.
type NativeExport struct {
	Version  int             `json:"version"`
	Comments []NativeComment `json:"comments"`
}

// NativeComment maps one-to-one with the comments table.
// ID is caller-supplied and stable; it is used as-is for dedup (no prefix).
type NativeComment struct {
	ID           string    `json:"id"`
	PageID       string    `json:"page_id"`
	PageURL      string    `json:"page_url"`
	PageTitle    string    `json:"page_title"`
	ParentID     string    `json:"parent_id"`
	AuthorName   string    `json:"author_name"`
	AuthorAvatar string    `json:"author_avatar"`
	Content      string    `json:"content"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
}

// ParseNative parses the Quipthread native JSON export format.
func ParseNative(r io.Reader) (*Result, error) {
	var export NativeExport
	if err := json.NewDecoder(r).Decode(&export); err != nil {
		return nil, err
	}
	if export.Version != 1 {
		return nil, fmt.Errorf("unsupported native export version %d (expected 1)", export.Version)
	}

	users := make(map[string]*models.User)
	var comments []*models.Comment

	for _, c := range export.Comments {
		authorKey := c.AuthorName
		if authorKey == "" {
			authorKey = "anonymous"
		}
		userID := syntheticUserID("native", authorKey)
		if _, ok := users[userID]; !ok {
			users[userID] = &models.User{
				ID:          userID,
				DisplayName: c.AuthorName,
				AvatarURL:   c.AuthorAvatar,
				Role:        "commenter",
			}
		}

		status := c.Status
		if status == "" {
			status = "approved"
		}

		pageID := c.PageID
		if pageID == "" {
			pageID = pageIDFromURL(c.PageURL)
		}

		comments = append(comments, &models.Comment{
			ID:           c.ID,
			PageID:       pageID,
			PageURL:      c.PageURL,
			PageTitle:    c.PageTitle,
			ParentID:     c.ParentID,
			UserID:       userID,
			Content:      sanitize.CommentHTML(c.Content),
			Status:       status,
			Imported:     true,
			DisqusAuthor: c.AuthorName,
			CreatedAt:    c.CreatedAt,
		})
	}

	result := &Result{Comments: comments}
	for _, u := range users {
		result.Users = append(result.Users, u)
	}
	return result, nil
}
