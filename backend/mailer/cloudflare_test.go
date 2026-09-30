package mailer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/quipthread/quipthread/config"
)

// styledCloudflareHTML is an arbitrary rendered body that must reach the API
// byte for byte while the derived text part loses only the markup.
const styledCloudflareHTML = `<h1 style="color:#E07F32">Styled &amp; ready</h1><p>Hello <a href="https://example.test/x">link</a></p>`

const deliveredCloudflareResponse = `{"success":true,"errors":[],"messages":[],"result":{"delivered":["owner@example.test"],"permanent_bounces":[],"queued":[]}}`

const queuedCloudflareResponse = `{"success":true,"errors":[],"result":{"delivered":[],"permanent_bounces":[],"queued":["owner@example.test"]}}`

func cloudflareConfig(accountID string) *config.Config {
	return &config.Config{
		EmailProvider:       "cloudflare",
		CloudflareAPIToken:  "test-token",
		CloudflareAccountID: accountID,
		SMTPFrom:            "hello@quipthread.com",
		EmailReplyTo:        "support@quipthread.com",
	}
}

// cloudflareTestDelivery points the real sender at an httptest server through
// its endpoint and client fields. No package-level state is mutated, so there
// is no endpoint to restore after the test. The server closes through
// t.Cleanup so an early t.Fatal cannot leak its goroutines.
func cloudflareTestDelivery(t *testing.T, server *httptest.Server, cfg *config.Config, to, subject, htmlBody string) cloudflareDelivery {
	t.Helper()
	t.Cleanup(server.Close)
	return cloudflareDelivery{
		client:   server.Client(),
		endpoint: server.URL,
		cfg:      cfg,
		to:       to,
		subject:  subject,
		html:     htmlBody,
	}
}

// receiveWithin fails the test instead of hanging when a handler never records
// the request it was supposed to see.
func receiveWithin[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(5 * time.Second):
		var zero T
		t.Fatal("handler never received the request")
		return zero
	}
}

func TestSendCloudflareSendsStyledHTMLVerbatim_whenRecipientAccepted(t *testing.T) {
	// Given a styled body that must reach the API unchanged
	requests := make(chan struct {
		payload       cloudflareSendRequest
		authorization string
	}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload cloudflareSendRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		requests <- struct {
			payload       cloudflareSendRequest
			authorization string
		}{payload: payload, authorization: r.Header.Get("Authorization")}
		_, _ = io.WriteString(w, deliveredCloudflareResponse)
	}))

	// When
	err := cloudflareTestDelivery(t, server, cloudflareConfig("account-1"), "owner@example.test", "2 comments awaiting approval", styledCloudflareHTML).
		deliver(context.Background())

	// Then
	if err != nil {
		t.Fatalf("SendCloudflare error = %v, want nil", err)
	}
	got := receiveWithin(t, requests)
	if got.payload.HTML != styledCloudflareHTML {
		t.Fatalf("html on the wire = %q, want the rendered body unchanged", got.payload.HTML)
	}
	if got.payload.To != "owner@example.test" || got.payload.From != "hello@quipthread.com" || got.payload.Subject != "2 comments awaiting approval" {
		t.Fatalf("wire envelope = %+v", got.payload)
	}
	if got.payload.ReplyTo != "support@quipthread.com" {
		t.Fatalf("reply_to = %q, want the configured reply address", got.payload.ReplyTo)
	}
	if got.authorization != "Bearer test-token" {
		t.Fatalf("authorization = %q", got.authorization)
	}
	if !strings.Contains(got.payload.Text, "Styled & ready") || strings.Contains(got.payload.Text, "<h1") {
		t.Fatalf("plain text alternative = %q", got.payload.Text)
	}
	if !strings.Contains(got.payload.Text, "https://example.test/x") {
		t.Fatalf("plain text alternative lost the link target: %q", got.payload.Text)
	}
}

func TestSendCloudflareOmitsReplyTo_whenUnset(t *testing.T) {
	// Given a config without EMAIL_REPLY_TO
	requests := make(chan cloudflareSendRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload cloudflareSendRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		requests <- payload
		_, _ = io.WriteString(w, `{"success":true,"result":{"delivered":["owner@example.test"]}}`)
	}))
	cfg := cloudflareConfig("account-1")
	cfg.EmailReplyTo = ""

	// When
	err := cloudflareTestDelivery(t, server, cfg, "owner@example.test", "subject", "<p>body</p>").deliver(context.Background())

	// Then
	if err != nil {
		t.Fatalf("SendCloudflare error = %v, want nil", err)
	}
	if got := receiveWithin(t, requests); got.ReplyTo != "" {
		t.Fatalf("reply_to = %q, want it omitted", got.ReplyTo)
	}
}

func TestSendCloudflareAcceptsQueuedEmail_whenNotDeliveredYet(t *testing.T) {
	// Given a queued outcome, accepted with either the documented 200 or the
	// 202 the API returns while the message is still in flight
	tests := []struct {
		name     string
		status   int
		response string
	}{
		{"200 with queued recipients", http.StatusOK, queuedCloudflareResponse},
		{"202 accepted with queued recipients", http.StatusAccepted, queuedCloudflareResponse},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.response)
			}))

			// When
			err := cloudflareTestDelivery(t, server, cloudflareConfig("account-1"), "owner@example.test", "subject", "<p>body</p>").
				deliver(context.Background())

			// Then
			if err != nil {
				t.Fatalf("queued send error = %v, want nil for status %d", err, test.status)
			}
		})
	}
}

func TestSendCloudflareAcceptsDelivered_whenProviderConfirms(t *testing.T) {
	// Given a success envelope that names the delivered recipient
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, deliveredCloudflareResponse)
	}))

	// When
	err := cloudflareTestDelivery(t, server, cloudflareConfig("account-1"), "owner@example.test", "subject", "<p>body</p>").
		deliver(context.Background())

	// Then
	if err != nil {
		t.Fatalf("delivered send error = %v, want nil", err)
	}
}

func TestSendCloudflareFails_whenRecipientPermanentBounces(t *testing.T) {
	// Given a permanent bounce for the only recipient
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"success":true,"errors":[],"result":{"delivered":[],"permanent_bounces":["owner@example.test"],"queued":[]}}`)
	}))

	// When
	err := cloudflareTestDelivery(t, server, cloudflareConfig("account-1"), "owner@example.test", "subject", "<p>body</p>").
		deliver(context.Background())

	// Then
	if !errors.Is(err, ErrCloudflareBounce) {
		t.Fatalf("bounce error = %v, want ErrCloudflareBounce", err)
	}
}

func TestSendCloudflareFails_whenAPIReturnsUnauthorized(t *testing.T) {
	// Given a token the API rejects
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))

	// When
	err := cloudflareTestDelivery(t, server, cloudflareConfig("account-1"), "owner@example.test", "subject", "<p>body</p>").
		deliver(context.Background())

	// Then
	if !errors.Is(err, ErrCloudflareRejected) {
		t.Fatalf("unauthorized error = %v, want ErrCloudflareRejected", err)
	}
	for _, secret := range []string{"test-token", "owner@example.test", "<p>body</p>"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaked %q: %v", secret, err)
		}
	}
}

func TestSendCloudflareFails_whenAPIReturnsSuccessFalse(t *testing.T) {
	// Given a documented validation failure envelope
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"success":false,"errors":[{"code":1000,"message":"Sender domain not verified"}],"result":null}`)
	}))

	// When
	err := cloudflareTestDelivery(t, server, cloudflareConfig("account-1"), "owner@example.test", "subject", "<p>body</p>").
		deliver(context.Background())

	// Then
	if !errors.Is(err, ErrCloudflareRejected) {
		t.Fatalf("rejected error = %v, want ErrCloudflareRejected", err)
	}
	if !strings.Contains(err.Error(), "1000") {
		t.Fatalf("rejection error = %v, want the numeric provider code", err)
	}
	if strings.Contains(err.Error(), "Sender domain not verified") {
		t.Fatalf("rejection error echoed the provider message: %v", err)
	}
}

func TestSendCloudflareFails_whenResponseIsMalformed(t *testing.T) {
	// Given a truncated success envelope
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"success":true,`)
	}))

	// When
	err := cloudflareTestDelivery(t, server, cloudflareConfig("account-1"), "owner@example.test", "subject", "<p>body</p>").
		deliver(context.Background())

	// Then
	if !errors.Is(err, ErrCloudflareRejected) {
		t.Fatalf("malformed response error = %v, want ErrCloudflareRejected", err)
	}
}

func TestSendCloudflareFails_whenResponseIsOversized(t *testing.T) {
	// Given a response body that never closes inside the read cap
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"success":true,"result":{"delivered":["owner@example.test"],"padding":"`)
		_, _ = io.WriteString(w, strings.Repeat("x", maxCloudflareResponseBody+4096))
		_, _ = io.WriteString(w, `"}}`)
	}))

	// When
	err := cloudflareTestDelivery(t, server, cloudflareConfig("account-1"), "owner@example.test", "subject", "<p>body</p>").
		deliver(context.Background())

	// Then
	if !errors.Is(err, ErrCloudflareRejected) {
		t.Fatalf("oversized response error = %v, want ErrCloudflareRejected", err)
	}
}

func TestSendCloudflareFails_whenSuccessNamesNoDeliveryOutcome(t *testing.T) {
	// Given a success envelope with an empty result
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"success":true,"errors":[],"result":{"delivered":[],"permanent_bounces":[],"queued":[]}}`)
	}))

	// When
	err := cloudflareTestDelivery(t, server, cloudflareConfig("account-1"), "owner@example.test", "subject", "<p>body</p>").
		deliver(context.Background())

	// Then
	if !errors.Is(err, ErrCloudflareRejected) {
		t.Fatalf("empty outcome error = %v, want ErrCloudflareRejected", err)
	}
}

func TestSendCloudflareFails_whenRequestDeadlineExpires(t *testing.T) {
	// Given a provider that never answers and a caller deadline. The handler
	// also watches a release channel so the server closes deterministically
	// even if the client cancellation is never observed.
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer close(release) // runs before server.Close
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// When
	err := cloudflareTestDelivery(t, server, cloudflareConfig("account-1"), "owner@example.test", "subject", "<p>body</p>").
		deliver(ctx)

	// Then
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v, want context.DeadlineExceeded", err)
	}
}

func TestSendCloudflarePreservesCancellation_whenContextAlreadyCancelled(t *testing.T) {
	// Given a cancelled caller context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// When
	err := cloudflareDelivery{
		client:   cloudflareHTTPClient,
		endpoint: cloudflareSendEndpoint(cloudflareConfig("account-1")),
		cfg:      cloudflareConfig("account-1"),
		to:       "owner@example.test",
		subject:  "subject",
		html:     "<p>body</p>",
	}.deliver(ctx)

	// Then
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled delivery error = %v, want context.Canceled", err)
	}
}

func TestSendCloudflareTransportFailureDoesNotEchoRequestData(t *testing.T) {
	// Given a token that must never be echoed and an endpoint whose transport
	// error would otherwise carry the URL and the request values
	cfg := cloudflareConfig("account-1")
	cfg.CloudflareAPIToken = "bad-token\nX-Injected: leaked-secret"

	// When
	err := cloudflareDelivery{
		client:   cloudflareHTTPClient,
		endpoint: "http://127.0.0.1:1/leak-token=secret",
		cfg:      cfg,
		to:       "owner@example.test",
		subject:  "subject",
		html:     "<p>body</p>",
	}.deliver(context.Background())

	// Then
	if !errors.Is(err, ErrCloudflareRejected) {
		t.Fatalf("transport failure error = %v, want ErrCloudflareRejected", err)
	}
	for _, leaked := range []string{"bad-token", "leaked-secret", "leak-token=secret", "127.0.0.1", "owner@example.test", "<p>body</p>"} {
		if strings.Contains(err.Error(), leaked) {
			t.Fatalf("transport failure leaked %q: %v", leaked, err)
		}
	}
}

func TestSendCloudflareRejectsEmptyRecipient(t *testing.T) {
	// Given a send without a recipient
	// When
	err := SendCloudflare(context.Background(), cloudflareConfig("account-1"), "  ", "subject", "<p>body</p>")

	// Then
	if !errors.Is(err, ErrCloudflareRejected) {
		t.Fatalf("empty recipient error = %v, want ErrCloudflareRejected", err)
	}
}

func TestSendCloudflareFails_whenCredentialsMissing(t *testing.T) {
	// Given a config without a token
	cfg := cloudflareConfig("account-1")
	cfg.CloudflareAPIToken = ""

	// When
	err := SendCloudflare(context.Background(), cfg, "owner@example.test", "subject", "<p>body</p>")

	// Then
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("error = %v, want ErrNotConfigured", err)
	}
}

func TestSendTransactionalRejectsNilConfig(t *testing.T) {
	// Given a caller with no configuration at all

	// When
	err := SendTransactional(nil, "owner@example.test", "subject", "<p>body</p>")

	// Then
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("error = %v, want ErrNotConfigured", err)
	}
}

func TestCloudflareSendEndpointTargetsAccount(t *testing.T) {
	// Given a Cloudflare account id
	// When
	endpoint := cloudflareSendEndpoint(cloudflareConfig("account-1"))

	// Then
	const want = "https://api.cloudflare.com/client/v4/accounts/account-1/email/sending/send"
	if endpoint != want {
		t.Fatalf("endpoint = %q, want %q", endpoint, want)
	}
}

func TestSendTransactionalFails_whenProviderIsUnknown(t *testing.T) {
	// Given a removed provider that also has SMTP available
	cfg := &config.Config{
		EmailProvider: "resend", EmailAPIKey: "api-key",
		SMTPHost: "smtp.example.test", SMTPPort: "587", SMTPFrom: "from@example.test",
	}

	// When
	err := SendTransactional(cfg, "to@example.test", "subject", "<p>body</p>")

	// Then
	if !errors.Is(err, ErrUnknownProvider) {
		t.Fatalf("error = %v, want ErrUnknownProvider", err)
	}
	if strings.Contains(err.Error(), "api-key") {
		t.Fatalf("error leaked the API key: %v", err)
	}
}

func TestSendTransactionalSkipsEmail_whenSMTPUnconfigured(t *testing.T) {
	// Given the legacy SMTP path with no SMTP host
	cfg := &config.Config{SMTPFrom: "from@example.test"}

	// When
	err := SendTransactional(cfg, "to@example.test", "subject", "<p>body</p>")

	// Then
	if err != nil {
		t.Fatalf("error = %v, want nil for an unconfigured SMTP install", err)
	}
}

func TestSendTransactionalUsesSMTP_whenProviderEmpty(t *testing.T) {
	// Given a self-hosted SMTP config aimed at a closed port
	cfg := &config.Config{SMTPHost: "127.0.0.1", SMTPPort: "1", SMTPFrom: "hello@quipthread.com"}

	// When
	err := SendTransactional(cfg, "owner@example.test", "subject", "<p>body</p>")

	// Then
	if err == nil {
		t.Fatal("SMTP transport unexpectedly succeeded")
	}
}
