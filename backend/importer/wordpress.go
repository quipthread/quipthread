package importer

import (
	"encoding/xml"
	"html"
	"io"
	"strings"
	"time"

	"github.com/quipthread/quipthread/models"
)

// WordPress WXR export (RSS-based). The wp: namespace URL varies by export
// version (1.0, 1.1, 1.2), so we match element names by local name only.
// This makes the parser version-agnostic.

type wpRSS struct {
	XMLName xml.Name `xml:"rss"`
	Items   []wpItem `xml:"channel>item"`
}

type wpItem struct {
	Title    string      `xml:"title"`
	Link     string      `xml:"link"`
	Comments []wpComment `xml:"comment"`
}

type wpComment struct {
	ID       string `xml:"comment_id"`
	Author   string `xml:"comment_author"`
	Email    string `xml:"comment_author_email"`
	Date     string `xml:"comment_date"`
	Content  string `xml:"comment_content"`
	Approved string `xml:"comment_approved"`
	Parent   string `xml:"comment_parent"`
	Type     string `xml:"comment_type"`
}

// ParseWordPress parses a WordPress WXR export. Pingbacks, trackbacks, and
// spam comments are skipped. Plain-text content is wrapped in <p> tags.
func ParseWordPress(r io.Reader) (*Result, error) {
	var feed wpRSS
	if err := xml.NewDecoder(r).Decode(&feed); err != nil {
		return nil, err
	}

	users := make(map[string]*models.User)
	var comments []*models.Comment

	for _, item := range feed.Items {
		pageID := pageIDFromURL(item.Link)

		for _, c := range item.Comments {
			// Skip pingbacks, trackbacks, spam, and trashed comments.
			t := strings.ToLower(c.Type)
			if t == "pingback" || t == "trackback" {
				continue
			}
			approved := strings.ToLower(c.Approved)
			if approved == "spam" || approved == "trash" {
				continue
			}

			authorKey := c.Email
			if authorKey == "" {
				authorKey = c.Author
			}
			if authorKey == "" {
				authorKey = "anonymous"
			}
			userID := syntheticUserID("wordpress", authorKey)
			if _, ok := users[userID]; !ok {
				users[userID] = &models.User{
					ID:          userID,
					DisplayName: c.Author,
					Email:       c.Email,
					Role:        "commenter",
				}
			}

			status := "approved"
			if c.Approved == "0" {
				status = "pending"
			}

			// WordPress dates are "2006-01-02 15:04:05" (local time, no TZ).
			createdAt, _ := time.Parse("2006-01-02 15:04:05", c.Date)

			parentID := ""
			if c.Parent != "" && c.Parent != "0" {
				parentID = commentID("wordpress", c.Parent)
			}

			comments = append(comments, &models.Comment{
				ID:           commentID("wordpress", c.ID),
				PageID:       pageID,
				PageURL:      item.Link,
				PageTitle:    item.Title,
				ParentID:     parentID,
				UserID:       userID,
				Content:      maybeWrapHTML(c.Content),
				Status:       status,
				Imported:     true,
				DisqusAuthor: c.Author,
				CreatedAt:    createdAt,
			})
		}
	}

	result := &Result{Comments: comments}
	for _, u := range users {
		result.Users = append(result.Users, u)
	}
	return result, nil
}

// maybeWrapHTML returns s unchanged if it contains HTML tags, or converts
// plain text (double-newline paragraphs) to <p>-wrapped HTML.
func maybeWrapHTML(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if strings.Contains(s, "<") {
		return s
	}
	var sb strings.Builder
	for _, para := range strings.Split(s, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		sb.WriteString("<p>")
		sb.WriteString(html.EscapeString(strings.ReplaceAll(para, "\n", " ")))
		sb.WriteString("</p>")
	}
	return sb.String()
}
