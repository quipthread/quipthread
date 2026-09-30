package notifications

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/mailer"
)

func TestEmailProviderSenderRoutesCloudflareThroughSharedHelper(t *testing.T) {
	// Given the cloudflare EMAIL_PROVIDER value
	// When
	send, ok := emailProviderSender("cloudflare")

	// Then
	if !ok {
		t.Fatal("cloudflare did not resolve a provider sender")
	}
	if reflect.ValueOf(send).Pointer() != reflect.ValueOf(mailer.SendCloudflare).Pointer() {
		t.Fatal("cloudflare digest did not route through mailer.SendCloudflare")
	}
}

func TestEmailProviderSenderAcceptsOnlySupportedProviders(t *testing.T) {
	tests := []struct {
		provider string
		want     bool
	}{
		{provider: "cloudflare", want: true},
		{provider: "CloudFlare", want: true},
		{provider: " cloudflare ", want: true},
		{provider: "postmark", want: true},
		{provider: "sendgrid", want: true},
		{provider: "ses", want: true},
		{provider: "", want: false},
		{provider: "resend", want: false},
		{provider: "RESEND", want: false},
		{provider: "smtp", want: false},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("provider=%q", test.provider), func(t *testing.T) {
			if _, ok := emailProviderSender(test.provider); ok != test.want {
				t.Fatalf("emailProviderSender(%q) ok = %v, want %v", test.provider, ok, test.want)
			}
		})
	}
}

func TestEmailAPIProviderReadyRequiresProviderCredentials(t *testing.T) {
	completeCloudflare := func() *config.Config {
		return &config.Config{
			EmailProvider: "cloudflare", CloudflareAPIToken: "token",
			CloudflareAccountID: "account", SMTPFrom: "hello@quipthread.com",
		}
	}
	tests := []struct {
		name string
		cfg  *config.Config
		want bool
	}{
		{name: "nil config", cfg: nil, want: false},
		{name: "empty provider", cfg: &config.Config{SMTPHost: "smtp", SMTPFrom: "from"}, want: false},
		{name: "cloudflare complete", cfg: completeCloudflare(), want: true},
		{name: "cloudflare without token", cfg: func() *config.Config { c := completeCloudflare(); c.CloudflareAPIToken = ""; return c }(), want: false},
		{name: "cloudflare without account", cfg: func() *config.Config { c := completeCloudflare(); c.CloudflareAccountID = ""; return c }(), want: false},
		{name: "cloudflare without sender", cfg: func() *config.Config { c := completeCloudflare(); c.SMTPFrom = ""; return c }(), want: false},
		{name: "postmark complete", cfg: &config.Config{EmailProvider: "postmark", EmailAPIKey: "key", SMTPFrom: "from"}, want: true},
		{name: "postmark without key", cfg: &config.Config{EmailProvider: "postmark", SMTPFrom: "from"}, want: false},
		{name: "ses complete", cfg: &config.Config{EmailProvider: "ses", SMTPHost: "smtp", SMTPPort: "587", SMTPFrom: "from"}, want: true},
		{name: "removed resend provider", cfg: &config.Config{EmailProvider: "resend", EmailAPIKey: "key", SMTPFrom: "from"}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := emailAPIProviderReady(test.cfg); got != test.want {
				t.Fatalf("emailAPIProviderReady = %v, want %v", got, test.want)
			}
		})
	}
}

func TestEmailAPIProviderReadyRequiresCloudflareTokenNotGenericAPIKey(t *testing.T) {
	// Given the Cloudflare provider whose only credential is the generic
	// EMAIL_API_KEY used by postmark/sendgrid
	cfg := &config.Config{
		EmailProvider: "cloudflare", EmailAPIKey: "generic-key",
		CloudflareAccountID: "account", SMTPFrom: "hello@quipthread.com",
	}

	// When / Then Cloudflare readiness requires CLOUDFLARE_API_TOKEN, so the
	// generic key alone is not enough
	if emailAPIProviderReady(cfg) {
		t.Fatal("cloudflare readiness accepted a generic EMAIL_API_KEY without CLOUDFLARE_API_TOKEN")
	}

	// And supplying the dedicated token completes the provider
	cfg.CloudflareAPIToken = "cf-token"
	if !emailAPIProviderReady(cfg) {
		t.Fatal("cloudflare readiness rejected a complete Cloudflare configuration")
	}
}

func TestCloudEligibleChannelNamesCoversHTTPProvidersOnly(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want bool
	}{
		{
			name: "postmark digest",
			cfg:  &config.Config{EmailProvider: "postmark", EmailAPIKey: "key", SMTPFrom: "hello@quipthread.com"},
			want: true,
		},
		{
			name: "cloudflare digest case insensitive",
			cfg: &config.Config{
				EmailProvider: " CloudFlare ", CloudflareAPIToken: "token",
				CloudflareAccountID: "account", SMTPFrom: "hello@quipthread.com",
			},
			want: true,
		},
		{
			name: "postmark without key is not eligible",
			cfg:  &config.Config{EmailProvider: "postmark", SMTPFrom: "hello@quipthread.com", SMTPHost: "smtp", SMTPPort: "587"},
			want: false,
		},
		{
			name: "plain SMTP is never a cloud digest channel",
			cfg:  &config.Config{SMTPHost: "smtp", SMTPPort: "587", SMTPFrom: "hello@quipthread.com"},
			want: false,
		},
		{
			name: "SES stays excluded because it is SMTP backed",
			cfg:  &config.Config{EmailProvider: "ses", SMTPHost: "smtp", SMTPPort: "587", SMTPFrom: "hello@quipthread.com"},
			want: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			eligible := CloudEligibleChannelNames(test.cfg)
			got := len(eligible) == 1 && eligible[0] == ChannelEmail
			if got != test.want {
				t.Fatalf("CloudEligibleChannelNames = %v, want email=%v", eligible, test.want)
			}
		})
	}
}

func TestEmailAPINotifierGatesOnProviderReadiness(t *testing.T) {
	// Given a notifier with no resolvable recipient so the provider gate is the
	// only thing that can stop the send
	batch := testBatch()
	noRecipient := func(string) string { return "" }

	// When / Then the selected provider passes the gate and reaches resolution
	cloudflare := NewEmailAPINotifier(&config.Config{
		EmailProvider: "cloudflare", CloudflareAPIToken: "token",
		CloudflareAccountID: "account", SMTPFrom: "from@example.test",
	}, noRecipient)
	if err := cloudflare.NotifyBatch(context.Background(), batch); !errors.Is(err, ErrRecipientUnavailable) {
		t.Fatalf("cloudflare error = %v, want recipient unavailable", err)
	}

	// And the removed provider is refused before any recipient lookup
	removed := NewEmailAPINotifier(&config.Config{
		EmailProvider: "resend", EmailAPIKey: "key", SMTPFrom: "from@example.test",
	}, noRecipient)
	if err := removed.NotifyBatch(context.Background(), batch); !errors.Is(err, ErrChannelNotConfigured) {
		t.Fatalf("removed provider error = %v, want not configured", err)
	}
}

func TestCloudEligibleChannelNamesIncludesCloudflareDigest(t *testing.T) {
	// Given a complete Cloudflare provider configuration
	// When
	eligible := CloudEligibleChannelNames(&config.Config{
		EmailProvider: "cloudflare", CloudflareAPIToken: "token",
		CloudflareAccountID: "account", SMTPFrom: "hello@quipthread.com",
	})

	// Then
	if len(eligible) != 1 || eligible[0] != ChannelEmail {
		t.Fatalf("cloudflare channels = %v, want [email]", eligible)
	}

	// And an incomplete provider never becomes a digest channel, even with SMTP
	// host variables present.
	incomplete := CloudEligibleChannelNames(&config.Config{
		EmailProvider: "cloudflare", SMTPFrom: "hello@quipthread.com",
		SMTPHost: "smtp.example.test", SMTPPort: "587",
	})
	if len(incomplete) != 0 {
		t.Fatalf("incomplete cloudflare channels = %v, want none", incomplete)
	}
}
