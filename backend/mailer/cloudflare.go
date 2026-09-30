package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/quipthread/quipthread/config"
)

const (
	cloudflareSendTimeout     = 15 * time.Second
	maxCloudflareResponseBody = 64 * 1024
	cloudflareSendURLTemplate = "https://api.cloudflare.com/client/v4/accounts/%s/email/sending/send"
)

var (
	// ErrNotConfigured reports that no provider has usable credentials.
	ErrNotConfigured = errors.New("mailer: email is not configured")
	// ErrUnknownProvider reports an EMAIL_PROVIDER value with no sender.
	ErrUnknownProvider = errors.New("mailer: unknown email provider")
	// ErrCloudflareRejected reports a send Cloudflare refused: a transport
	// failure, a non-2xx status, success=false, or a success response that
	// named no delivery outcome.
	ErrCloudflareRejected = errors.New("mailer: cloudflare send rejected")
	// ErrCloudflareBounce reports at least one permanent bounce.
	ErrCloudflareBounce = errors.New("mailer: cloudflare reported a permanent bounce")
)

var cloudflareHTTPClient = &http.Client{Timeout: cloudflareSendTimeout}

// cloudflareSendRequest is the REST payload. The REST API uses a plain string
// for from/to and snake_case reply_to (never a Reply-To header).
type cloudflareSendRequest struct {
	To      string `json:"to"`
	From    string `json:"from"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
	Text    string `json:"text"`
	ReplyTo string `json:"reply_to,omitempty"`
}

// cloudflareSendResponse mirrors the documented success envelope. Only the
// fields needed to decide the delivery outcome are decoded; provider error
// message text is never logged or returned because it can echo request values.
type cloudflareSendResponse struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code int `json:"code"`
	} `json:"errors"`
	Result *struct {
		Delivered        []string `json:"delivered"`
		PermanentBounces []string `json:"permanent_bounces"`
		Queued           []string `json:"queued"`
	} `json:"result"`
}

// cloudflareDelivery binds one send to an explicit endpoint and HTTP client.
// Tests point these fields at an httptest server, so the package keeps no
// mutable send seam that production code could accidentally depend on.
type cloudflareDelivery struct {
	client   *http.Client
	endpoint string
	cfg      *config.Config
	to       string
	subject  string
	html     string
}

// SendCloudflare delivers one transactional email through the Cloudflare Email
// Sending REST API. The HTML body is sent verbatim and text is a plain-text
// alternative derived from it. The context bounds the request and the response
// body read is capped.
func SendCloudflare(ctx context.Context, cfg *config.Config, to, subject, htmlBody string) error {
	return cloudflareDelivery{
		client:   cloudflareHTTPClient,
		endpoint: cloudflareSendEndpoint(cfg),
		cfg:      cfg,
		to:       to,
		subject:  subject,
		html:     htmlBody,
	}.deliver(ctx)
}

// cloudflareSendEndpoint builds the production Email Sending REST URL. Credential
// validation runs before any request is built, so a nil cfg only has to produce
// a harmless placeholder here.
func cloudflareSendEndpoint(cfg *config.Config) string {
	accountID := ""
	if cfg != nil {
		accountID = strings.TrimSpace(cfg.CloudflareAccountID)
	}
	return fmt.Sprintf(cloudflareSendURLTemplate, accountID)
}

func (d cloudflareDelivery) deliver(ctx context.Context) error {
	if d.cfg == nil || strings.TrimSpace(d.cfg.CloudflareAPIToken) == "" ||
		strings.TrimSpace(d.cfg.CloudflareAccountID) == "" || strings.TrimSpace(d.cfg.SMTPFrom) == "" {
		return fmt.Errorf("%w: cloudflare requires CLOUDFLARE_API_TOKEN, CLOUDFLARE_ACCOUNT_ID, and SMTP_FROM", ErrNotConfigured)
	}
	if ctx == nil {
		return fmt.Errorf("%w: cloudflare send requires a context", ErrCloudflareRejected)
	}
	if strings.TrimSpace(d.to) == "" {
		return fmt.Errorf("%w: recipient is empty", ErrCloudflareRejected)
	}

	payload, err := json.Marshal(cloudflareSendRequest{
		To:      d.to,
		From:    d.cfg.SMTPFrom,
		Subject: d.subject,
		HTML:    d.html,
		Text:    plainTextFromHTML(d.html),
		ReplyTo: strings.TrimSpace(d.cfg.EmailReplyTo),
	})
	if err != nil {
		return fmt.Errorf("%w: encode request", ErrCloudflareRejected)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint, bytes.NewReader(payload))
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("%w: build request", ErrCloudflareRejected)
	}
	req.Header.Set("Authorization", "Bearer "+d.cfg.CloudflareAPIToken)
	req.Header.Set("Content-Type", "application/json")

	client := d.client
	if client == nil {
		client = cloudflareHTTPClient
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		// The transport error is reduced to a constant. net/http embeds the
		// request URL in its error and a malformed CLOUDFLARE_API_TOKEN can
		// surface as an invalid Authorization value, so the raw error must
		// never cross this boundary into logs or persisted delivery state.
		return fmt.Errorf("%w: transport failure", ErrCloudflareRejected)
	}
	defer resp.Body.Close() //nolint:errcheck // response body is read below

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: status %d", ErrCloudflareRejected, resp.StatusCode)
	}

	var decoded cloudflareSendResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxCloudflareResponseBody)).Decode(&decoded); err != nil {
		return fmt.Errorf("%w: status %d unreadable response", ErrCloudflareRejected, resp.StatusCode)
	}
	if !decoded.Success {
		return fmt.Errorf("%w: status %d codes %v", ErrCloudflareRejected, resp.StatusCode, decoded.errorCodes())
	}
	if decoded.Result == nil {
		return fmt.Errorf("%w: status %d without a delivery outcome", ErrCloudflareRejected, resp.StatusCode)
	}
	// A single-recipient send that reports a bounce did not deliver. Report it
	// even when the API also returns queued or delivered entries.
	if len(decoded.Result.PermanentBounces) > 0 {
		return fmt.Errorf("%w: %d recipient(s)", ErrCloudflareBounce, len(decoded.Result.PermanentBounces))
	}
	if len(decoded.Result.Delivered) > 0 || len(decoded.Result.Queued) > 0 {
		return nil
	}
	return fmt.Errorf("%w: status %d without a delivery outcome", ErrCloudflareRejected, resp.StatusCode)
}

func (r cloudflareSendResponse) errorCodes() []int {
	codes := make([]int, 0, len(r.Errors))
	for _, item := range r.Errors {
		codes = append(codes, item.Code)
	}
	return codes
}
