package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/quipthread/quipthread/config"
)

// EmailAPINotifier sends HTML email digests via Resend, Postmark, or Sendgrid.
// Selected by cfg.EmailProvider.
type EmailAPINotifier struct {
	cfg        *config.Config
	ownerEmail func(ownerID string) string
}

func NewEmailAPINotifier(cfg *config.Config, ownerEmail func(string) string) *EmailAPINotifier {
	return &EmailAPINotifier{cfg: cfg, ownerEmail: ownerEmail}
}

func (e *EmailAPINotifier) NotifyBatch(ctx context.Context, b Batch) error {
	to := e.ownerEmail(b.Site.OwnerID)
	if to == "" {
		to = e.cfg.NotifyEmailTo
	}
	if to == "" {
		return nil
	}

	subject := fmt.Sprintf("[Quipthread] %d comment(s) awaiting approval on %s",
		len(b.Comments), b.Site.Domain)
	body := buildEmailHTML(b)

	switch e.cfg.EmailProvider {
	case "resend":
		return sendResend(ctx, e.cfg, to, subject, body)
	case "postmark":
		return sendPostmark(ctx, e.cfg, to, subject, body)
	case "sendgrid":
		return sendSendgrid(ctx, e.cfg, to, subject, body)
	default:
		return fmt.Errorf("unknown email provider: %s", e.cfg.EmailProvider)
	}
}

func sendResend(ctx context.Context, cfg *config.Config, to, subject, html string) error {
	payload := map[string]interface{}{
		"from":    cfg.SMTPFrom,
		"to":      []string{to},
		"subject": subject,
		"html":    html,
	}
	return postJSON(ctx, "https://api.resend.com/emails",
		map[string]string{"Authorization": "Bearer " + cfg.EmailAPIKey},
		payload)
}

func sendPostmark(ctx context.Context, cfg *config.Config, to, subject, html string) error {
	payload := map[string]interface{}{
		"From":     cfg.SMTPFrom,
		"To":       to,
		"Subject":  subject,
		"HtmlBody": html,
	}
	return postJSON(ctx, "https://api.postmarkapp.com/email",
		map[string]string{
			"X-Postmark-Server-Token": cfg.EmailAPIKey,
			"Accept":                  "application/json",
		},
		payload)
}

func sendSendgrid(ctx context.Context, cfg *config.Config, to, subject, html string) error {
	payload := map[string]interface{}{
		"personalizations": []map[string]interface{}{
			{"to": []map[string]string{{"email": to}}},
		},
		"from":    map[string]string{"email": cfg.SMTPFrom},
		"subject": subject,
		"content": []map[string]string{
			{"type": "text/html", "value": html},
		},
	}
	return postJSON(ctx, "https://api.sendgrid.com/v3/mail/send",
		map[string]string{"Authorization": "Bearer " + cfg.EmailAPIKey},
		payload)
}

func postJSON(ctx context.Context, url string, headers map[string]string, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("email API %s: HTTP %d", url, resp.StatusCode)
	}
	return nil
}
