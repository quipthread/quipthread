package exporter

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/quipthread/quipthread/models"
)

// WriteNative streams comments as Quipthread native JSON to w.
// The output is directly re-importable via importer.ParseNative.
func WriteNative(w io.Writer, comments []*models.Comment, site *models.Site) error {
	exportedAt := time.Now().UTC().Format(time.RFC3339)

	preamble := fmt.Sprintf(
		`{"version":1,"site":{"id":%s,"domain":%s},"exported_at":%s,"comments":[`,
		jsonStr(site.ID), jsonStr(site.Domain), jsonStr(exportedAt),
	)
	if _, err := io.WriteString(w, preamble); err != nil {
		return err
	}

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)

	for i, c := range comments {
		if i > 0 {
			if _, err := io.WriteString(w, ","); err != nil {
				return err
			}
		}
		obj := nativeComment(c)
		if err := enc.Encode(obj); err != nil {
			return err
		}
	}

	_, err := io.WriteString(w, "]}")
	return err
}

type nativeCommentJSON struct {
	ID           string    `json:"id"`
	PageID       string    `json:"page_id"`
	PageURL      string    `json:"page_url,omitempty"`
	PageTitle    string    `json:"page_title,omitempty"`
	ParentID     string    `json:"parent_id,omitempty"`
	AuthorName   string    `json:"author_name,omitempty"`
	AuthorAvatar string    `json:"author_avatar,omitempty"`
	Content      string    `json:"content"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
}

func nativeComment(c *models.Comment) nativeCommentJSON {
	return nativeCommentJSON{
		ID:           c.ID,
		PageID:       c.PageID,
		PageURL:      c.PageURL,
		PageTitle:    c.PageTitle,
		ParentID:     c.ParentID,
		AuthorName:   c.AuthorName,
		AuthorAvatar: c.AuthorAvatar,
		Content:      c.Content,
		Status:       c.Status,
		CreatedAt:    c.CreatedAt.UTC(),
	}
}

// jsonStr returns a JSON-encoded string literal (including surrounding quotes).
func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
