package notifications

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/models"
)

type recordingNotifier struct {
	mu    sync.Mutex
	calls int
	err   error
	wait  <-chan struct{}
}

type cancelingNotifier struct {
	cancel context.CancelFunc
}

func (n *cancelingNotifier) NotifyBatch(context.Context, Batch) error {
	n.cancel()
	return nil
}

func (n *recordingNotifier) NotifyBatch(ctx context.Context, _ Batch) error {
	n.mu.Lock()
	n.calls++
	err := n.err
	wait := n.wait
	n.mu.Unlock()
	if wait != nil {
		select {
		case <-wait:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}

func (n *recordingNotifier) callCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.calls
}

func testBatch() Batch {
	return Batch{
		Site:     &models.Site{ID: "site-1", Domain: "example.test"},
		Comments: []*models.Comment{{ID: "comment-1", SiteID: "site-1", Status: "pending"}},
		ApproveURLs: map[string]string{
			"comment-1": "https://example.test/approve/comment-1",
		},
		RejectURLs: map[string]string{
			"comment-1": "https://example.test/reject/comment-1",
		},
	}
}

func TestNamedChannelSenderRoutesOneChannelAndSanitizesProviderError(t *testing.T) {
	const secret = "https://hooks.example.test/secret-token?tenant=private"
	slack := &recordingNotifier{err: fmt.Errorf("provider response included %s and comment content", secret)}
	discord := &recordingNotifier{}
	sender := NewNamedChannelSender(map[string]Notifier{
		ChannelSlack:   slack,
		ChannelDiscord: discord,
	})

	err := sender.Send(context.Background(), ChannelSlack, testBatch())
	if !errors.Is(err, ErrChannelProvider) {
		t.Fatalf("Send error = %v, want provider error", err)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "comment content") {
		t.Fatalf("Send error leaked provider data: %v", err)
	}
	if slack.callCount() != 1 || discord.callCount() != 0 {
		t.Fatalf("calls = slack:%d discord:%d, want 1 and 0", slack.callCount(), discord.callCount())
	}
}

func TestNamedChannelSenderRejectsUnknownAndMissingChannels(t *testing.T) {
	sender := NewNamedChannelSender(map[string]Notifier{ChannelEmail: &recordingNotifier{}})

	if err := sender.Send(context.Background(), "tenant-locator-with-secret", testBatch()); !errors.Is(err, ErrInvalidChannel) {
		t.Fatalf("unknown channel error = %v, want invalid channel", err)
	}
	if strings.Contains(senderError(sender, "tenant-locator-with-secret"), "tenant-locator-with-secret") {
		t.Fatal("invalid channel error leaked persisted value")
	}
	if err := sender.Send(context.Background(), ChannelWebhook, testBatch()); !errors.Is(err, ErrChannelNotConfigured) {
		t.Fatalf("missing channel error = %v, want not configured", err)
	}
}

func senderError(sender *NamedChannelSender, channel string) string {
	return sender.Send(context.Background(), channel, testBatch()).Error()
}

func TestNamedChannelSenderPreservesContextOutcome(t *testing.T) {
	release := make(chan struct{})
	sender := NewNamedChannelSender(map[string]Notifier{
		ChannelWebhook: &recordingNotifier{wait: release},
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	err := sender.Send(ctx, ChannelWebhook, testBatch())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Send error = %v, want deadline exceeded", err)
	}
}

func TestNamedChannelSenderTreatsNilProviderErrorAsSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sender := NewNamedChannelSender(map[string]Notifier{
		ChannelWebhook: &cancelingNotifier{cancel: cancel},
	})

	if err := sender.Send(ctx, ChannelWebhook, testBatch()); err != nil {
		t.Fatalf("Send error = %v, want success after provider returned nil", err)
	}
}

func TestNamedChannelSenderRejectsMalformedBatchBeforeProvider(t *testing.T) {
	valid := testBatch()
	tests := []struct {
		name  string
		batch Batch
	}{
		{name: "nil site", batch: Batch{Comments: valid.Comments, ApproveURLs: valid.ApproveURLs, RejectURLs: valid.RejectURLs}},
		{name: "empty site ID", batch: func() Batch { b := testBatch(); b.Site = &models.Site{}; return b }()},
		{name: "empty comments", batch: func() Batch { b := testBatch(); b.Comments = nil; return b }()},
		{name: "nil comment", batch: func() Batch { b := testBatch(); b.Comments = []*models.Comment{nil}; return b }()},
		{name: "empty comment ID", batch: func() Batch {
			b := testBatch()
			b.Comments = []*models.Comment{{SiteID: "site-1", Status: "pending"}}
			return b
		}()},
		{name: "non-pending comment", batch: func() Batch { b := testBatch(); b.Comments[0].Status = "approved"; return b }()},
		{name: "wrong site", batch: func() Batch { b := testBatch(); b.Comments[0].SiteID = "site-2"; return b }()},
		{name: "duplicate comment IDs", batch: func() Batch {
			b := testBatch()
			b.Comments = append(b.Comments, &models.Comment{ID: "comment-1", SiteID: "site-1", Status: "pending"})
			return b
		}()},
		{name: "missing approval URL", batch: func() Batch { b := testBatch(); b.ApproveURLs = nil; return b }()},
		{name: "empty approval URL", batch: func() Batch { b := testBatch(); b.ApproveURLs["comment-1"] = ""; return b }()},
		{name: "missing rejection URL", batch: func() Batch { b := testBatch(); b.RejectURLs = nil; return b }()},
		{name: "empty rejection URL", batch: func() Batch { b := testBatch(); b.RejectURLs["comment-1"] = ""; return b }()},
		{name: "invalid approval URL", batch: func() Batch { b := testBatch(); b.ApproveURLs["comment-1"] = "not-a-url"; return b }()},
		{name: "invalid rejection URL", batch: func() Batch { b := testBatch(); b.RejectURLs["comment-1"] = "ftp://example.test/reject"; return b }()},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &recordingNotifier{}
			sender := NewNamedChannelSender(map[string]Notifier{ChannelWebhook: provider})
			err := sender.Send(context.Background(), ChannelWebhook, test.batch)
			if !errors.Is(err, ErrChannelInvalidRequest) {
				t.Fatalf("error = %v, want invalid request", err)
			}
			for _, value := range []string{"site-2", "comment-1", "example.test", "not-a-url"} {
				if strings.Contains(err.Error(), value) {
					t.Fatalf("invalid request error leaked %q: %v", value, err)
				}
			}
			if provider.callCount() != 0 {
				t.Fatalf("provider calls = %d, want 0", provider.callCount())
			}
		})
	}
}

func TestEmailNotifiersTreatMissingRecipientAsFailure(t *testing.T) {
	cfg := &config.Config{EmailProvider: "resend", EmailAPIKey: "api-key", SMTPFrom: "from@example.test"}
	email := NewEmailAPINotifier(cfg, func(string) string { return "" })
	err := email.NotifyBatch(context.Background(), testBatch())
	if !errors.Is(err, ErrRecipientUnavailable) {
		t.Fatalf("email error = %v, want recipient unavailable", err)
	}

	smtp := NewSMTPNotifier(&config.Config{SMTPHost: "smtp.example.test", SMTPPort: "587", SMTPFrom: "from@example.test"}, func(string) string { return "" })
	err = smtp.NotifyBatch(context.Background(), testBatch())
	if !errors.Is(err, ErrRecipientUnavailable) {
		t.Fatalf("SMTP error = %v, want recipient unavailable", err)
	}
}

func TestProviderNotifiersTreatMissingConfigurationAsFailure(t *testing.T) {
	cfg := &config.Config{}
	tests := []struct {
		name string
		n    Notifier
	}{
		{name: ChannelEmail, n: NewEmailAPINotifier(cfg, nil)},
		{name: ChannelSlack, n: NewSlackNotifier(cfg)},
		{name: ChannelDiscord, n: NewDiscordNotifier(cfg)},
		{name: ChannelTelegram, n: NewTelegramNotifier(cfg)},
		{name: ChannelWebhook, n: NewWebhookNotifier(cfg)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.n.NotifyBatch(context.Background(), testBatch())
			if !errors.Is(err, ErrChannelNotConfigured) {
				t.Fatalf("error = %v, want not configured", err)
			}
		})
	}
}

func TestPostJSONDoesNotReturnRequestURLOnProviderFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// The path deliberately contains values that must not reach persistence.
		// A status-only failure is enough to exercise the sanitization boundary.
		http.Error(w, "provider details: private", http.StatusBadGateway)
	}))
	defer server.Close()

	secretURL := server.URL + "/credential-token?recipient=private@example.test"
	err := postJSON(context.Background(), secretURL, nil, map[string]string{"content": "private"})
	if !errors.Is(err, ErrChannelProvider) {
		t.Fatalf("postJSON error = %v, want provider error", err)
	}
	for _, value := range []string{secretURL, "private", "private@example.test"} {
		if strings.Contains(err.Error(), value) {
			t.Fatalf("postJSON error leaked %q: %v", value, err)
		}
	}
}
