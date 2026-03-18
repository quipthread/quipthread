package exporter

import (
	"encoding/csv"
	"io"
	"regexp"

	"github.com/quipthread/quipthread/models"
)

var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

// WriteCSV writes comments as CSV (UTF-8) to w. HTML tags are stripped from
// the content column — sufficient for Tiptap's output.
func WriteCSV(w io.Writer, comments []*models.Comment) error {
	cw := csv.NewWriter(w)

	if err := cw.Write([]string{
		"id", "site_id", "page_id", "page_url", "page_title",
		"parent_id", "author_name", "status", "content", "created_at", "updated_at",
	}); err != nil {
		return err
	}

	for _, c := range comments {
		record := []string{
			c.ID,
			c.SiteID,
			c.PageID,
			c.PageURL,
			c.PageTitle,
			c.ParentID,
			c.AuthorName,
			c.Status,
			htmlTagRe.ReplaceAllString(c.Content, ""),
			c.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			c.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}

	cw.Flush()
	return cw.Error()
}
