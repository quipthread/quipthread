package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/mailer"
)

// EmailAPINotifier sends HTML email digests through an HTTP email provider.
// Selected by cfg.EmailProvider: cloudflare, postmark, sendgrid, or ses.
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
	if e == nil || e.cfg == nil || !emailAPIProviderReady(e.cfg) {
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
	send, ok := emailProviderSender(e.cfg.EmailProvider)
	if !ok {
		return channelError(ChannelEmail, ChannelErrorNotConfigured)
	}
	if e.providerSend != nil {
		send = e.providerSend
	}
	return channelErrorFromProvider(ChannelEmail, send(ctx, e.cfg, to, subject, body))
}

// emailProviderSender resolves EMAIL_PROVIDER to the shared delivery helper.
// The bool is the routing decision that the digest notifier and cloud channel
// eligibility both consume: false means the value selects no HTTP provider.
func emailProviderSender(provider string) (func(context.Context, *config.Config, string, string, string) error, bool) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "cloudflare":
		return mailer.SendCloudflare, true
	case "postmark":
		return sendPostmark, true
	case "sendgrid":
		return sendSendgrid, true
	case "ses":
		return sendSESContext, true
	default:
		return nil, false
	}
}

// sendSESContext adapts the SMTP-backed SES sender to the HTTP provider shape
// so a single dispatch path serves every provider.
func sendSESContext(ctx context.Context, cfg *config.Config, to, subject, html string) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return sendSES(cfg, to, subject, html)
}

// emailAPIProviderReady reports whether EMAIL_PROVIDER selects an HTTP mail
// provider that has every credential it needs. An empty provider means SMTP,
// and an unknown one is not an API provider at all.
func emailAPIProviderReady(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(cfg.EmailProvider)) {
	case "cloudflare":
		return cfg.CloudflareAPIToken != "" && cfg.CloudflareAccountID != "" && cfg.SMTPFrom != ""
	case "postmark", "sendgrid":
		return cfg.EmailAPIKey != "" && cfg.SMTPFrom != ""
	case "ses":
		return cfg.SMTPHost != "" && cfg.SMTPPort != "" && cfg.SMTPFrom != ""
	default:
		return false
	}
}

func channelErrorFromProvider(channel string, err error) error {
	if err == nil {
		return nil
	}
	return safeChannelError(channel, err)
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
