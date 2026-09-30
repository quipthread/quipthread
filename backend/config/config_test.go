package config

import (
	"errors"
	"testing"
)

func validProductionConfig() *Config {
	return &Config{
		Port: "8080", JWTSecret: "secret", BaseURL: "https://app.example",
		AllowedOrigins: []string{"https://app.example"},
		GitHubClientID: "github-id", GitHubSecret: "github-secret",
		DatabaseURL: "./data/comments.db", RateLimitComments: "5/10m", RateLimitAuth: "10/5m",
	}
}

func TestValidateProductionConfigRejectsMissingRequiredValues(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Config)
	}{
		{"jwt", func(c *Config) { c.JWTSecret = "" }},
		{"base url", func(c *Config) { c.BaseURL = "not-an-origin" }},
		{"origins", func(c *Config) { c.AllowedOrigins = nil }},
		{"provider", func(c *Config) { c.GitHubClientID, c.GitHubSecret = "", "" }},
		{"rate", func(c *Config) { c.RateLimitAuth = "bad" }},
		{"port", func(c *Config) { c.Port = "70000" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validProductionConfig()
			tc.mutate(cfg)
			if err := ValidateProductionConfig(cfg); err == nil {
				t.Fatal("validation unexpectedly passed")
			}
		})
	}
}

func TestValidateProductionConfigAcceptsCompleteSelfHostedConfig(t *testing.T) {
	if err := ValidateProductionConfig(validProductionConfig()); err != nil {
		t.Fatalf("validation failed: %v", err)
	}
}

func TestValidateProductionConfigAcceptsCloudflareEmailWithoutSMTPHost(t *testing.T) {
	// Given an email-auth deployment that sends through Cloudflare over HTTPS
	cfg := validProductionConfig()
	cfg.EmailAuthEnabled = true
	cfg.EmailProvider = "cloudflare"
	cfg.CloudflareAPIToken = "cf-token"
	cfg.CloudflareAccountID = "cf-account"
	cfg.SMTPFrom = "hello@quipthread.com"

	// When
	err := ValidateProductionConfig(cfg)

	// Then SMTP_HOST is not required for the REST provider
	if err != nil {
		t.Fatalf("cloudflare email auth rejected: %v", err)
	}
}

func TestValidateProductionConfigRejectsCloudflareEmailWithMissingCredentials(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Config)
	}{
		{"missing from", func(c *Config) { c.SMTPFrom = "" }},
		{"missing token", func(c *Config) { c.CloudflareAPIToken = "   " }},
		{"missing account", func(c *Config) { c.CloudflareAccountID = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validProductionConfig()
			cfg.EmailAuthEnabled = true
			cfg.EmailProvider = "cloudflare"
			cfg.CloudflareAPIToken = "cf-token"
			cfg.CloudflareAccountID = "cf-account"
			cfg.SMTPFrom = "hello@quipthread.com"
			tc.mutate(cfg)

			if err := ValidateProductionConfig(cfg); err == nil {
				t.Fatal("validation unexpectedly accepted an incomplete Cloudflare sender")
			}
		})
	}
}

func TestValidateProductionConfigRejectsUnknownEmailProvider(t *testing.T) {
	cfg := validProductionConfig()
	cfg.EmailProvider = "resend"

	if err := ValidateProductionConfig(cfg); err == nil {
		t.Fatal("validation accepted an unsupported EMAIL_PROVIDER")
	}
}

func TestValidateProductionConfigNormalizesEmailProviderValue(t *testing.T) {
	// Given a provider value with surrounding whitespace and mixed case
	cfg := validProductionConfig()
	cfg.EmailAuthEnabled = true
	cfg.EmailProvider = "  CloudFlare  "
	cfg.CloudflareAPIToken = "cf-token"
	cfg.CloudflareAccountID = "cf-account"
	cfg.SMTPFrom = "hello@quipthread.com"

	// When / Then the value is accepted and resolved as Cloudflare
	if err := ValidateProductionConfig(cfg); err != nil {
		t.Fatalf("normalized provider rejected: %v", err)
	}
}

func TestValidateProductionConfigTreatsBlankProviderAsSMTP(t *testing.T) {
	// Given an email-auth deployment whose provider is whitespace only
	cfg := validProductionConfig()
	cfg.EmailAuthEnabled = true
	cfg.EmailProvider = "   "
	cfg.SMTPFrom = "hello@quipthread.com"

	// When / Then it keeps the legacy SMTP path, so SMTP_HOST is still required
	if err := ValidateProductionConfig(cfg); err == nil {
		t.Fatal("blank provider without SMTP_HOST unexpectedly passed")
	}

	cfg.SMTPHost = "smtp.example.test"
	if err := ValidateProductionConfig(cfg); err != nil {
		t.Fatalf("blank provider with SMTP_HOST rejected: %v", err)
	}
}

func TestValidateRejectsNilConfig(t *testing.T) {
	if err := ValidateProductionConfig(nil); err == nil {
		t.Fatal("ValidateProductionConfig(nil) unexpectedly passed")
	}
	if err := Validate(nil); err == nil {
		t.Fatal("Validate(nil) unexpectedly passed")
	}
}

func TestValidateSelfHostedDatabaseURLRejectsRemoteBeforeOpen(t *testing.T) {
	for _, databaseURL := range []string{
		"libsql://tenant.example.test",
		"https://tenant.example.test",
		"http://tenant.example.test",
	} {
		if err := ValidateSelfHostedDatabaseURL(databaseURL); !errors.Is(err, ErrSelfHostedRemoteDatabase) {
			t.Fatalf("ValidateSelfHostedDatabaseURL(%q) = %v", databaseURL, err)
		}
	}
	if err := ValidateSelfHostedDatabaseURL("./data/comments.db"); err != nil {
		t.Fatalf("local SQLite URL rejected: %v", err)
	}
}

func TestLoadCloudNotificationDeliveryDisabledByDefault(t *testing.T) {
	t.Setenv("CLOUD_NOTIFICATION_DELIVERY_ENABLED", "")
	if Load().CloudNotificationDeliveryEnabled {
		t.Fatal("cloud notification delivery enabled without explicit opt-in")
	}
}

func TestLoadCloudNotificationDeliveryRequiresExplicitTrue(t *testing.T) {
	for _, value := range []string{"1", "TRUE", "yes"} {
		t.Setenv("CLOUD_NOTIFICATION_DELIVERY_ENABLED", value)
		if Load().CloudNotificationDeliveryEnabled {
			t.Fatalf("cloud notification delivery enabled for %q", value)
		}
	}
	t.Setenv("CLOUD_NOTIFICATION_DELIVERY_ENABLED", "true")
	if !Load().CloudNotificationDeliveryEnabled {
		t.Fatal("cloud notification delivery did not honor explicit opt-in")
	}
}

func TestLoadReadsCloudflareEmailSettings(t *testing.T) {
	// Isolate from the ignored root .env: every asserted value is set here.
	t.Setenv("EMAIL_PROVIDER", "cloudflare")
	t.Setenv("CLOUDFLARE_API_TOKEN", "cf-token")
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "cf-account")
	t.Setenv("EMAIL_REPLY_TO", "reply@quipthread.com")

	cfg := Load()

	if cfg.EmailProvider != "cloudflare" {
		t.Fatalf("EmailProvider = %q, want cloudflare", cfg.EmailProvider)
	}
	if cfg.CloudflareAPIToken != "cf-token" {
		t.Fatalf("CloudflareAPIToken = %q, want cf-token", cfg.CloudflareAPIToken)
	}
	if cfg.CloudflareAccountID != "cf-account" {
		t.Fatalf("CloudflareAccountID = %q, want cf-account", cfg.CloudflareAccountID)
	}
	if cfg.EmailReplyTo != "reply@quipthread.com" {
		t.Fatalf("EmailReplyTo = %q, want reply@quipthread.com", cfg.EmailReplyTo)
	}
}
