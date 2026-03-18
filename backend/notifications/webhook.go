package notifications

import (
	"context"

	"github.com/quipthread/quipthread/config"
)

// WebhookNotifier POST JSON to a generic HTTP endpoint.
type WebhookNotifier struct {
	cfg *config.Config
}

func NewWebhookNotifier(cfg *config.Config) *WebhookNotifier {
	return &WebhookNotifier{cfg: cfg}
}

func (w *WebhookNotifier) NotifyBatch(ctx context.Context, b Batch) error {
	type commentEntry struct {
		ID         string `json:"id"`
		Author     string `json:"author"`
		Excerpt    string `json:"excerpt"`
		ApproveURL string `json:"approve_url"`
		RejectURL  string `json:"reject_url"`
	}

	entries := make([]commentEntry, 0, len(b.Comments))
	for _, c := range b.Comments {
		author := c.AuthorName
		if author == "" {
			author = "Anonymous"
		}
		entries = append(entries, commentEntry{
			ID:         c.ID,
			Author:     author,
			Excerpt:    commentExcerpt(c.Content, 200),
			ApproveURL: b.ApproveURLs[c.ID],
			RejectURL:  b.RejectURLs[c.ID],
		})
	}

	payload := map[string]interface{}{
		"site_id":       b.Site.ID,
		"site_domain":   b.Site.Domain,
		"pending_count": len(b.Comments),
		"comments":      entries,
	}
	return postJSON(ctx, w.cfg.WebhookURL, nil, payload)
}
