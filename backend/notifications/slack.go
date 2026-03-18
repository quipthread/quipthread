package notifications

import (
	"context"
	"fmt"

	"github.com/quipthread/quipthread/config"
)

// SlackNotifier posts a Block Kit message to a Slack incoming webhook.
type SlackNotifier struct {
	cfg *config.Config
}

func NewSlackNotifier(cfg *config.Config) *SlackNotifier {
	return &SlackNotifier{cfg: cfg}
}

func (s *SlackNotifier) NotifyBatch(ctx context.Context, b Batch) error {
	blocks := []map[string]interface{}{
		{
			"type": "header",
			"text": map[string]interface{}{
				"type": "plain_text",
				"text": fmt.Sprintf("Quipthread: %d comment(s) awaiting approval on %s",
					len(b.Comments), b.Site.Domain),
			},
		},
		{"type": "divider"},
	}

	for _, c := range b.Comments {
		author := c.AuthorName
		if author == "" {
			author = "Anonymous"
		}
		excerpt := commentExcerpt(c.Content, 150)
		approveURL := b.ApproveURLs[c.ID]
		rejectURL := b.RejectURLs[c.ID]

		blocks = append(blocks,
			map[string]interface{}{
				"type": "section",
				"text": map[string]interface{}{
					"type": "mrkdwn",
					"text": fmt.Sprintf("*%s*: %s", author, excerpt),
				},
			},
			map[string]interface{}{
				"type": "actions",
				"elements": []map[string]interface{}{
					{
						"type": "button",
						"text": map[string]interface{}{"type": "plain_text", "text": "Approve"},
						"url":  approveURL,
						"style": "primary",
					},
					{
						"type": "button",
						"text": map[string]interface{}{"type": "plain_text", "text": "Reject"},
						"url":  rejectURL,
						"style": "danger",
					},
				},
			},
		)
	}

	payload := map[string]interface{}{"blocks": blocks}
	return postJSON(ctx, s.cfg.SlackWebhookURL, nil, payload)
}
