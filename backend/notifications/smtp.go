package notifications

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"net/smtp"
	"strings"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/models"
)

// SMTPNotifier sends HTML email digests via a standard SMTP server.
type SMTPNotifier struct {
	cfg        *config.Config
	ownerEmail func(ownerID string) string // resolves site owner email; may return ""
}

func NewSMTPNotifier(cfg *config.Config, ownerEmail func(string) string) *SMTPNotifier {
	return &SMTPNotifier{cfg: cfg, ownerEmail: ownerEmail}
}

func (s *SMTPNotifier) NotifyBatch(ctx context.Context, b Batch) error {
	to := s.ownerEmail(b.Site.OwnerID)
	if to == "" {
		to = s.cfg.NotifyEmailTo
	}
	if to == "" {
		return nil // nowhere to send
	}

	subject := fmt.Sprintf("[Quipthread] %d comment(s) awaiting approval on %s",
		len(b.Comments), b.Site.Domain)
	body := buildEmailHTML(b)

	msg := buildMIMEMessage(s.cfg.SMTPFrom, to, subject, body)

	addr := fmt.Sprintf("%s:%s", s.cfg.SMTPHost, s.cfg.SMTPPort)
	var auth smtp.Auth
	if s.cfg.SMTPUser != "" {
		auth = smtp.PlainAuth("", s.cfg.SMTPUser, s.cfg.SMTPPass, s.cfg.SMTPHost)
	}
	return smtp.SendMail(addr, auth, s.cfg.SMTPFrom, []string{to}, []byte(msg))
}

func buildMIMEMessage(from, to, subject, htmlBody string) string {
	var b strings.Builder
	b.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "From: %s\r\n", from)       //nolint:errcheck // strings.Builder.Write never fails
	fmt.Fprintf(&b, "To: %s\r\n", to)           //nolint:errcheck // strings.Builder.Write never fails
	fmt.Fprintf(&b, "Subject: %s\r\n", subject) //nolint:errcheck // strings.Builder.Write never fails
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(htmlBody)
	return b.String()
}

func buildEmailHTML(b Batch) string {
	var buf bytes.Buffer
	buf.WriteString(`<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<style>
  body { font-family: system-ui, sans-serif; background: #F7F4EF; color: #1A1714; margin: 0; padding: 24px; }
  .container { max-width: 640px; margin: 0 auto; background: #EEEBE4; border-radius: 8px; overflow: hidden; }
  .header { background: #1A1A1A; color: #E8E3DC; padding: 24px 32px; }
  .header h1 { margin: 0; font-size: 1.125rem; }
  .header p { margin: 4px 0 0; font-size: 0.875rem; color: #8A8480; }
  .comments { padding: 16px 32px; }
  .comment { background: #fff; border: 1px solid #D9D4CB; border-radius: 6px; margin: 12px 0; padding: 16px; }
  .comment-meta { font-size: 0.75rem; color: #7A7570; margin-bottom: 8px; }
  .comment-author { font-weight: 600; color: #1A1714; }
  .comment-content { font-size: 0.9375rem; line-height: 1.5; margin: 8px 0 12px; }
  .comment-actions { display: flex; gap: 8px; }
  .btn { display: inline-block; padding: 6px 16px; border-radius: 4px; font-size: 0.8125rem; font-weight: 600; text-decoration: none; }
  .btn-approve { background: #E07F32; color: #fff; }
  .btn-reject  { background: transparent; color: #7A7570; border: 1px solid #D9D4CB; }
  .footer { padding: 16px 32px; font-size: 0.75rem; color: #8A8480; border-top: 1px solid #D9D4CB; }
</style>
</head>
<body>
<div class="container">
  <div class="header">
    <h1>Quipthread Moderation</h1>
    <p>`)
	fmt.Fprintf(&buf, "%d comment(s) awaiting approval on %s", len(b.Comments), html.EscapeString(b.Site.Domain))
	buf.WriteString(`</p>
  </div>
  <div class="comments">`)

	for _, c := range b.Comments {
		excerpt := commentExcerpt(c.Content, 200)
		approveURL := b.ApproveURLs[c.ID]
		rejectURL := b.RejectURLs[c.ID]
		author := c.AuthorName
		if author == "" {
			author = "Anonymous"
		}

		fmt.Fprintf(&buf, `
    <div class="comment">
      <div class="comment-meta">
        <span class="comment-author">%s</span> on <a href="%s">%s</a>
      </div>
      <div class="comment-content">%s</div>
      <div class="comment-actions">
        <a class="btn btn-approve" href="%s">Approve</a>
        <a class="btn btn-reject"  href="%s">Reject</a>
      </div>
    </div>`,
			html.EscapeString(author),
			html.EscapeString(c.PageURL),
			html.EscapeString(pageLabel(c)),
			html.EscapeString(excerpt),
			html.EscapeString(approveURL),
			html.EscapeString(rejectURL),
		)
	}

	buf.WriteString(`
  </div>
  <div class="footer">Sent by Quipthread. Open the approval link to take action — no login required.</div>
</div>
</body>
</html>`)
	return buf.String()
}

func commentExcerpt(content string, max int) string {
	// Strip basic HTML tags for the plain excerpt.
	stripped := strings.NewReplacer(
		"<p>", "", "</p>", " ",
		"<br>", " ", "<br/>", " ", "<br />", " ",
		"<strong>", "", "</strong>", "",
		"<em>", "", "</em>", "",
		"<a ", "<a ", // keep links but this is just an excerpt
	).Replace(content)
	// Remove any remaining tags crudely.
	for strings.Contains(stripped, "<") {
		start := strings.Index(stripped, "<")
		end := strings.Index(stripped, ">")
		if end == -1 {
			break
		}
		stripped = stripped[:start] + stripped[end+1:]
	}
	stripped = strings.TrimSpace(stripped)
	if len(stripped) > max {
		return stripped[:max] + "…"
	}
	return stripped
}

func pageLabel(c *models.Comment) string {
	if c.PageTitle != "" {
		return c.PageTitle
	}
	if c.PageURL != "" {
		return c.PageURL
	}
	return c.PageID
}
