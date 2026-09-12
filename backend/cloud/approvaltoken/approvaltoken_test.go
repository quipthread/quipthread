package approvaltoken

import (
	"strings"
	"testing"
)

func TestHashDeterministic(t *testing.T) {
	const key = "0123456789abcdef0123456789abcdef"

	h1 := Hash(key, "bearer-token-1")
	h2 := Hash(key, "bearer-token-1")
	if h1 != h2 {
		t.Fatalf("hash not deterministic: %q vs %q", h1, h2)
	}
	if len(h1) != 64 { // hex-encoded SHA-256
		t.Fatalf("hash length = %d, want 64", len(h1))
	}

	// Different tokens under the same key hash differently.
	if Hash(key, "bearer-token-2") == h1 {
		t.Fatal("distinct tokens produced identical hashes")
	}
	// The same token under a different key hashes differently.
	if Hash("fedcba9876543210fedcba9876543210", "bearer-token-1") == h1 {
		t.Fatal("distinct keys produced identical hashes")
	}
}

func TestHashesEqual(t *testing.T) {
	const key = "0123456789abcdef0123456789abcdef"
	h := Hash(key, "bearer-token-1")

	if !HashesEqual(h, h) {
		t.Error("identical hashes compared unequal")
	}
	if !HashesEqual(h, Hash(key, "bearer-token-1")) {
		t.Error("same token+key hashes compared unequal")
	}
	if HashesEqual(h, Hash(key, "bearer-token-2")) {
		t.Error("different tokens compared equal")
	}
	if HashesEqual(h, strings.ToUpper(h)) {
		t.Error("case-mutated hash compared equal")
	}
	if HashesEqual("", "") {
		t.Error("two empty hashes compared equal")
	}
}

func TestValidateKey(t *testing.T) {
	t.Run("accepts key at minimum length", func(t *testing.T) {
		if err := ValidateKey(strings.Repeat("k", MinKeyBytes)); err != nil {
			t.Fatalf("valid key rejected: %v", err)
		}
	})
	t.Run("rejects empty key", func(t *testing.T) {
		err := ValidateKey("")
		if err == nil {
			t.Fatal("empty key accepted")
		}
	})
	t.Run("rejects short key", func(t *testing.T) {
		err := ValidateKey(strings.Repeat("k", MinKeyBytes-1))
		if err == nil {
			t.Fatal("short key accepted")
		}
	})
	t.Run("errors never contain key material", func(t *testing.T) {
		key := "super-secret-key-material-do-not-leak-123456"[:MinKeyBytes-1]
		err := ValidateKey(key)
		if err == nil {
			t.Fatal("short key accepted")
		}
		msg := err.Error()
		if strings.Contains(msg, key) || strings.Contains(msg, "secret") {
			t.Errorf("error message leaks key material: %q", msg)
		}
	})
}
