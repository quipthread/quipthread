package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/quipthread/quipthread/cloud/approvaltoken"
	"github.com/quipthread/quipthread/config"
)

// Startup guard: CLOUD_MODE=true must fail closed when APPROVAL_TOKEN_HMAC_KEY
// is absent or too short. Self-hosted deployments (CLOUD_MODE=false) are
// unaffected and never require the key.
func TestValidateCloudConfigApprovalTokenKey(t *testing.T) {
	validKey := strings.Repeat("k", approvaltoken.MinKeyBytes)

	t.Run("accepts valid key in cloud mode", func(t *testing.T) {
		cfg := &config.Config{CloudMode: true, ApprovalTokenHMACKey: validKey}
		if err := validateCloudConfig(cfg); err != nil {
			t.Fatalf("valid key rejected: %v", err)
		}
	})

	t.Run("rejects absent key in cloud mode", func(t *testing.T) {
		cfg := &config.Config{CloudMode: true, ApprovalTokenHMACKey: ""}
		err := validateCloudConfig(cfg)
		if !errors.Is(err, approvaltoken.ErrInvalidKey) {
			t.Fatalf("empty key accepted: got %v, want ErrInvalidKey", err)
		}
	})

	t.Run("rejects short key in cloud mode", func(t *testing.T) {
		cfg := &config.Config{CloudMode: true, ApprovalTokenHMACKey: strings.Repeat("k", approvaltoken.MinKeyBytes-1)}
		err := validateCloudConfig(cfg)
		if !errors.Is(err, approvaltoken.ErrInvalidKey) {
			t.Fatalf("short key accepted: got %v, want ErrInvalidKey", err)
		}
	})

	t.Run("errors never contain key material", func(t *testing.T) {
		key := "super-secret-key-material-do-not-leak-123456"[:approvaltoken.MinKeyBytes-1]
		cfg := &config.Config{CloudMode: true, ApprovalTokenHMACKey: key}
		msg := validateCloudConfig(cfg).Error()
		if strings.Contains(msg, key) || strings.Contains(msg, "secret") {
			t.Errorf("error message leaks key material: %q", msg)
		}
	})

	t.Run("self-hosted unaffected by missing key", func(t *testing.T) {
		cfg := &config.Config{CloudMode: false, ApprovalTokenHMACKey: ""}
		if err := validateCloudConfig(cfg); err != nil {
			t.Fatalf("self-hosted startup rejected with empty key: %v", err)
		}
	})
}
