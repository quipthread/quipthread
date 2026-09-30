//go:build !selfhosted

package notifications

import (
	"testing"

	"github.com/quipthread/quipthread/config"
)

func providerAndSMTPConfig() *config.Config {
	return &config.Config{
		EmailProvider: "cloudflare", CloudflareAPIToken: "token", CloudflareAccountID: "account",
		SMTPFrom: "hello@quipthread.com", SMTPHost: "smtp.example.test", SMTPPort: "587",
	}
}

func TestBuildSelectsOneEmailRoute_whenProviderAndSMTPAreConfigured(t *testing.T) {
	// Given a cloud build with both a selected API provider and an SMTP relay
	// When
	multi := Build(providerAndSMTPConfig(), nil)

	// Then
	apiRoutes, smtpRoutes := 0, 0
	for _, notifier := range multi.notifiers {
		switch notifier.(type) {
		case *EmailAPINotifier:
			apiRoutes++
		case *SMTPNotifier:
			smtpRoutes++
		}
	}
	if apiRoutes != 1 || smtpRoutes != 0 {
		t.Fatalf("email notifiers = api:%d smtp:%d, want 1 and 0", apiRoutes, smtpRoutes)
	}
}

func TestBuildKeepsSMTPRoute_whenNoProviderIsSelected(t *testing.T) {
	// Given a legacy cloud build without EMAIL_PROVIDER
	cfg := &config.Config{SMTPFrom: "hello@quipthread.com", SMTPHost: "smtp.example.test", SMTPPort: "587"}

	// When
	multi := Build(cfg, nil)

	// Then
	smtpRoutes := 0
	for _, notifier := range multi.notifiers {
		if _, ok := notifier.(*SMTPNotifier); ok {
			smtpRoutes++
		}
	}
	if smtpRoutes != 1 {
		t.Fatalf("smtp notifiers = %d, want 1", smtpRoutes)
	}
}

func TestBuildKeepsSMTPRoute_whenProviderIsWhitespaceOnly(t *testing.T) {
	// Given a cloud build whose EMAIL_PROVIDER is whitespace only, which config
	// validation and the mailer both treat as an empty provider (SMTP)
	cfg := &config.Config{
		EmailProvider: "   ", SMTPFrom: "hello@quipthread.com",
		SMTPHost: "smtp.example.test", SMTPPort: "587",
	}

	// When
	multi := Build(cfg, nil)

	// Then the whitespace-only provider still resolves to the SMTP route
	smtpRoutes, apiRoutes := 0, 0
	for _, notifier := range multi.notifiers {
		switch notifier.(type) {
		case *SMTPNotifier:
			smtpRoutes++
		case *EmailAPINotifier:
			apiRoutes++
		}
	}
	if smtpRoutes != 1 || apiRoutes != 0 {
		t.Fatalf("email notifiers = smtp:%d api:%d, want 1 and 0", smtpRoutes, apiRoutes)
	}
}

func TestBuildChannelSenderRoutesWhitespaceOnlyProviderToSMTP(t *testing.T) {
	// Given an explicit whitespace-only provider and an SMTP relay
	cfg := &config.Config{
		EmailProvider: " \t ", SMTPFrom: "hello@quipthread.com",
		SMTPHost: "smtp.example.test", SMTPPort: "587",
	}

	// When
	named := BuildNamedChannelSender(cfg, nil)
	direct := BuildChannelSender(cfg, nil)

	// Then both builders route the email channel to SMTP, not an API provider
	for _, sender := range []*NamedChannelSender{named, direct} {
		if _, ok := sender.notifiers[ChannelEmail].(*SMTPNotifier); !ok {
			t.Fatalf("email channel = %T, want *SMTPNotifier", sender.notifiers[ChannelEmail])
		}
	}
}

func TestBuildChannelSenderPrefersSelectedProviderOverSMTP(t *testing.T) {
	// Given an explicit provider and an SMTP relay
	cfg := providerAndSMTPConfig()

	// When
	named := BuildNamedChannelSender(cfg, nil)
	direct := BuildChannelSender(cfg, nil)

	// Then
	for _, sender := range []*NamedChannelSender{named, direct} {
		if _, ok := sender.notifiers[ChannelEmail].(*EmailAPINotifier); !ok {
			t.Fatalf("email channel = %T, want *EmailAPINotifier", sender.notifiers[ChannelEmail])
		}
	}
}

func TestBuildDoesNotFallBackToSMTP_whenProviderIsUnsupported(t *testing.T) {
	// Given a misconfigured build with an unsupported provider and an SMTP relay
	cfg := &config.Config{
		EmailProvider: "resend", EmailAPIKey: "key",
		SMTPFrom: "hello@quipthread.com", SMTPHost: "smtp.example.test", SMTPPort: "587",
	}

	// When
	multi := Build(cfg, nil)

	// Then the email channel stays unrouted instead of silently using SMTP
	for _, notifier := range multi.notifiers {
		switch notifier.(type) {
		case *EmailAPINotifier, *SMTPNotifier:
			t.Fatalf("unexpected email route %T", notifier)
		}
	}

	// And the named builder keeps the same decision
	if _, ok := BuildNamedChannelSender(cfg, nil).notifiers[ChannelEmail]; ok {
		t.Fatal("named builder routed email for an unsupported provider")
	}
}
