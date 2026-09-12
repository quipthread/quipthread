package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/mailer"
)

// EmailAPINotifier sends HTML email digests via Resend, Postmark, or Sendgrid.
// Selected by cfg.EmailProvider.
type EmailAPINotifier struct {
	cfg               *config.Config
	ownerEmail        func(ownerID string) string
	ownerEmailContext func(context.Context, string) (string, error)
	providerSend      func(context.Context, *config.Config, string, string, string) error
}

func NewEmailAPINotifier(cfg *config.Config, ownerEmail func(string) string) *EmailAPINotifier {
	return &EmailAPINotifier{cfg: cfg, ownerEmail: ownerEmail}
}

// NewContextEmailAPINotifier preserves the legacy recipient callback while
// allowing tenant delivery to cancel a context-aware lookup before a provider
// request is made.
func NewContextEmailAPINotifier(cfg *config.Config, ownerEmailContext func(context.Context, string) (string, error), ownerEmail func(string) string) *EmailAPINotifier {
	return &EmailAPINotifier{cfg: cfg, ownerEmail: ownerEmail, ownerEmailContext: ownerEmailContext}
}

func (e *EmailAPINotifier) NotifyBatch(ctx context.Context, b Batch) error {
	if e == nil || e.cfg == nil || e.cfg.EmailProvider == "" || e.cfg.EmailAPIKey == "" || e.cfg.SMTPFrom == "" {
		return channelError(ChannelEmail, ChannelErrorNotConfigured)
	}
	if e.cfg.EmailProvider == "ses" && (e.cfg.SMTPHost == "" || e.cfg.SMTPPort == "") {
		return channelError(ChannelEmail, ChannelErrorNotConfigured)
	}
	if ctx == nil {
		return channelError(ChannelEmail, ChannelErrorInvalidRequest)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if b.Site == nil {
		return channelError(ChannelEmail, ChannelErrorInvalidRequest)
	}

	var to string
	if e.ownerEmailContext != nil {
		var err error
		to, err = e.ownerEmailContext(ctx, b.Site.OwnerID)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if errors.Is(err, ErrRecipientUnavailable) {
				return err
			}
			return channelError(ChannelEmail, ChannelErrorRecipientUnavailable)
		}
	} else if e.ownerEmail != nil {
		to = e.ownerEmail(b.Site.OwnerID)
	}
	if to == "" {
		to = e.cfg.NotifyEmailTo
	}
	if to == "" {
		return channelError(ChannelEmail, ChannelErrorRecipientUnavailable)
	}

	subject := fmt.Sprintf("[Quipthread] %d comment(s) awaiting approval on %s",
		len(b.Comments), b.Site.Domain)
	body := buildEmailHTML(b)
	if e.providerSend != nil {
		return e.providerSend(ctx, e.cfg, to, subject, body)
	}

	switch e.cfg.EmailProvider {
	case "resend":
		return sendResend(ctx, e.cfg, to, subject, body)
	case "postmark":
		return sendPostmark(ctx, e.cfg, to, subject, body)
	case "sendgrid":
		return sendSendgrid(ctx, e.cfg, to, subject, body)
	case "ses":
		return channelErrorFromProvider(ChannelEmail, sendSES(e.cfg, to, subject, body))
	default:
		return channelError(ChannelEmail, ChannelErrorNotConfigured)
	}
}

func channelErrorFromProvider(channel string, err error) error {
	if err == nil {
		return nil
	}
	return safeChannelError(channel, err)
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

// sendSES sends via the AWS SES SMTP endpoint using the existing SMTP config.
// The SMTP_HOST should be set to email-smtp.{region}.amazonaws.com and
// SMTP_USER/SMTP_PASS should be the IAM SMTP credentials (not access keys).
// mailer.SendTransactional is SMTP-backed and has the same limitation as
// SMTPNotifier: a context cannot interrupt an in-flight send.
func sendSES(cfg *config.Config, to, subject, html string) error {
	return mailer.SendTransactional(cfg, to, subject, html)
}

func postJSON(ctx context.Context, url string, headers map[string]string, payload interface{}) error {
	if ctx == nil {
		return ErrChannelInvalidRequest
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ErrChannelProvider
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return ErrChannelProvider
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return ErrChannelProvider
	}
	defer resp.Body.Close() //nolint:errcheck // deferred close; response body not read
	if resp.StatusCode >= 400 {
		return ErrChannelProvider
	}
	return nil
}
