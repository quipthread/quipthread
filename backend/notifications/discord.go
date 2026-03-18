package notifications

import (
	"context"
	"fmt"
	"strings"

	"github.com/quipthread/quipthread/config"
)

// DiscordNotifier posts an embed message to a Discord incoming webhook.
type DiscordNotifier struct {
	cfg *config.Config
}

func NewDiscordNotifier(cfg *config.Config) *DiscordNotifier {
	return &DiscordNotifier{cfg: cfg}
}

func (d *DiscordNotifier) NotifyBatch(ctx context.Context, b Batch) error {
	var sb strings.Builder
	for _, c := range b.Comments {
		author := c.AuthorName
		if author == "" {
			author = "Anonymous"
		}
		excerpt := commentExcerpt(c.Content, 150)
		approveURL := b.ApproveURLs[c.ID]
		rejectURL := b.RejectURLs[c.ID]

		fmt.Fprintf(&sb, "**%s**: %s\n[Approve](%s) | [Reject](%s)\n\n",
			author, excerpt, approveURL, rejectURL)
	}

	payload := map[string]interface{}{
		"embeds": []map[string]interface{}{
			{
				"title":       fmt.Sprintf("%d comment(s) awaiting approval on %s", len(b.Comments), b.Site.Domain),
				"description": sb.String(),
				"color":       0xE07F32, // Quill Amber
			},
		},
	}
	return postJSON(ctx, d.cfg.DiscordWebhookURL, nil, payload)
}
