package importer

import (
	"encoding/json"
	"io"
	"time"

	"github.com/quipthread/quipthread/models"
)

type remark42Comment struct {
	ID      string `json:"id"`
	PID     string `json:"pid"` // parent ID; "" = top-level
	Text    string `json:"text"`
	User    struct {
		Name    string `json:"name"`
		ID      string `json:"id"`
		Picture string `json:"picture"`
	} `json:"user"`
	Locator struct {
		URL string `json:"url"`
	} `json:"locator"`
	Timestamp time.Time `json:"timestamp"`
	Deleted   bool      `json:"deleted"`
}

// ParseRemark42 parses a Remark42 JSON export (flat array of comment objects).
// Deleted comments are skipped.
func ParseRemark42(r io.Reader) (*Result, error) {
	var raw []remark42Comment
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, err
	}

	users := make(map[string]*models.User)
	var comments []*models.Comment

	for _, c := range raw {
		if c.Deleted {
			continue
		}

		userKey := c.User.ID
		if userKey == "" {
			userKey = c.User.Name
		}
		userID := syntheticUserID("remark42", userKey)
		if _, ok := users[userID]; !ok {
			users[userID] = &models.User{
				ID:          userID,
				DisplayName: c.User.Name,
				AvatarURL:   c.User.Picture,
				Role:        "commenter",
			}
		}

		parentID := ""
		if c.PID != "" {
			parentID = commentID("remark42", c.PID)
		}

		comments = append(comments, &models.Comment{
			ID:           commentID("remark42", c.ID),
			PageID:       pageIDFromURL(c.Locator.URL),
			PageURL:      c.Locator.URL,
			ParentID:     parentID,
			UserID:       userID,
			Content:      c.Text,
			Status:       "approved",
			Imported:     true,
			DisqusAuthor: c.User.Name,
			CreatedAt:    c.Timestamp,
		})
	}

	result := &Result{Comments: comments}
	for _, u := range users {
		result.Users = append(result.Users, u)
	}
	return result, nil
}
