// Package approvaltoken provides deterministic hashing for comment-approval
// bearer tokens in the cloud control plane. Only HMAC-SHA-256 hashes are ever
// produced or compared here; raw tokens and key material must never be logged,
// persisted, or embedded in errors.
package approvaltoken

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// MinKeyBytes is the minimum accepted HMAC key length (256-bit).
const MinKeyBytes = 32

// ErrInvalidKey is returned when the configured HMAC key is missing or too
// short. Wrapped errors carry only length metadata, never key contents.
var ErrInvalidKey = errors.New("approval token hmac key invalid")

// ValidateKey reports whether key is usable as the approval-token HMAC key.
// It fails closed on empty keys and keys shorter than MinKeyBytes.
func ValidateKey(key string) error {
	if key == "" {
		return fmt.Errorf("%w: APPROVAL_TOKEN_HMAC_KEY is empty", ErrInvalidKey)
	}
	if len(key) < MinKeyBytes {
		return fmt.Errorf("%w: must be at least %d bytes (got %d bytes)", ErrInvalidKey, MinKeyBytes, len(key))
	}
	return nil
}

// Hash returns the hex-encoded HMAC-SHA-256 of token under key. The mapping is
// deterministic for a given key, so hashes can be used as database lookup keys.
func Hash(key, token string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}

// HashesEqual compares two hex-encoded hashes in constant time. It is safe to
// use on attacker-influenced input; timing does not depend on where the
// strings differ. Empty hashes never compare equal so a missing/unparsed
// hash can never match a stored locator.
func HashesEqual(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return hmac.Equal([]byte(a), []byte(b))
}
