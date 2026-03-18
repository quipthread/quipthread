package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/quipthread/quipthread/config"
)

// TelegramNotifier posts a Markdown digest to a Telegram chat via Bot API.
type TelegramNotifier struct {
	cfg *config.Config
}

func NewTelegramNotifier(cfg *config.Config) *TelegramNotifier {
	return &TelegramNotifier{cfg: cfg}
}

func (t *TelegramNotifier) NotifyBatch(ctx context.Context, b Batch) error {
	var sb strings.Builder
	fmt.Fprintf(&sb, "*Quipthread: %d comment(s) awaiting approval on %s*\n\n",
		len(b.Comments), escapeMarkdown(b.Site.Domain))

	for i, c := range b.Comments {
		author := c.AuthorName
		if author == "" {
			author = "Anonymous"
		}
		excerpt := commentExcerpt(c.Content, 120)
		approveURL := b.ApproveURLs[c.ID]
		rejectURL := b.RejectURLs[c.ID]

		fmt.Fprintf(&sb, "%d\\. *%s*: %s\n[Approve](%s) | [Reject](%s)\n\n",
			i+1,
			escapeMarkdown(author),
			escapeMarkdown(excerpt),
			approveURL,
			rejectURL,
		)
	}

	payload := map[string]interface{}{
		"chat_id":    t.cfg.TelegramChatID,
		"text":       sb.String(),
		"parse_mode": "MarkdownV2",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.cfg.TelegramBotToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("telegram API: HTTP %d", resp.StatusCode)
	}
	return nil
}

// escapeMarkdown escapes special characters for Telegram MarkdownV2.
func escapeMarkdown(s string) string {
	replacer := strings.NewReplacer(
		"_", "\\_", "*", "\\*", "[", "\\[", "]", "\\]",
		"(", "\\(", ")", "\\)", "~", "\\~", "`", "\\`",
		">", "\\>", "#", "\\#", "+", "\\+", "-", "\\-",
		"=", "\\=", "|", "\\|", "{", "\\{", "}", "\\}",
		".", "\\.", "!", "\\!",
	)
	return replacer.Replace(s)
}
