package session

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-key"

func TestIssue_Parse_RoundTrip(t *testing.T) {
	tok, err := Issue(testSecret, "u1", "Alice", "github", "admin", "acc1")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if tok == "" {
		t.Fatal("Issue returned empty token")
	}

	claims, err := Parse(testSecret, tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if claims.Sub != "u1" {
		t.Errorf("Sub: got %q, want u1", claims.Sub)
	}
	if claims.DisplayName != "Alice" {
		t.Errorf("DisplayName: got %q, want Alice", claims.DisplayName)
	}
	if claims.Provider != "github" {
		t.Errorf("Provider: got %q, want github", claims.Provider)
	}
	if claims.Role != "admin" {
		t.Errorf("Role: got %q, want admin", claims.Role)
	}
	if claims.AccountID != "acc1" {
		t.Errorf("AccountID: got %q, want acc1", claims.AccountID)
	}
}

func TestParse_WrongSecret(t *testing.T) {
	tok, err := Issue(testSecret, "u1", "Alice", "github", "admin", "")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := Parse("wrong-secret", tok); err == nil {
		t.Error("Parse with wrong secret: expected error, got nil")
	}
}

func TestParse_ExpiredToken(t *testing.T) {
	claims := Claims{
		Sub:  "u1",
		Role: "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tok, err := token.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign expired token: %v", err)
	}
	if _, err := Parse(testSecret, tok); err == nil {
		t.Error("Parse of expired token: expected error, got nil")
	}
}

func TestParse_MalformedToken(t *testing.T) {
	cases := []string{"", "not-a-jwt", "a.b", "a.b.c.d"}
	for _, s := range cases {
		if _, err := Parse(testSecret, s); err == nil {
			t.Errorf("Parse(%q): expected error, got nil", s)
		}
	}
}
