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
