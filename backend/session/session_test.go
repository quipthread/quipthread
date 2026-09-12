package session

import (
	"net/http"
	"net/http/httptest"
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

func TestParseForAudienceRejectsCrossAudience(t *testing.T) {
	tok, err := IssueWithAudience(testSecret, EmbedAudience, "u1", "Alice", "sso", "commenter", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseForAudience(testSecret, tok, DashboardAudience); err == nil {
		t.Fatal("dashboard parser accepted embed token")
	}
	if _, err := ParseForAudience(testSecret, tok, EmbedAudience); err != nil {
		t.Fatalf("embed parser rejected embed token: %v", err)
	}
}

func TestParseRejectsLegacyTokenWithoutAudience(t *testing.T) {
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		Sub: "legacy-user",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(testSecret, tok); err == nil {
		t.Fatal("legacy token without audience was accepted")
	}
}

func TestParseForAudienceRejectsNonHS256(t *testing.T) {
	claims := Claims{RegisteredClaims: jwt.RegisteredClaims{
		Audience:  jwt.ClaimStrings{DashboardAudience},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS512, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseForAudience(testSecret, tok, DashboardAudience); err == nil {
		t.Fatal("parser accepted HS512 token")
	}
}

func TestAudienceCookies(t *testing.T) {
	for _, test := range []struct {
		audience string
		secure   bool
		name     string
		sameSite http.SameSite
	}{
		{DashboardAudience, false, DashboardCookieName, http.SameSiteLaxMode},
		{EmbedAudience, true, EmbedCookieName, http.SameSiteNoneMode},
		{EmbedAudience, false, EmbedCookieName, http.SameSiteLaxMode},
	} {
		rr := httptest.NewRecorder()
		SetCookieForAudience(rr, "token", test.audience, test.secure)
		cookies := rr.Result().Cookies()
		var got *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == test.name {
				got = cookie
				break
			}
		}
		if got == nil || got.Value != "token" || got.SameSite != test.sameSite || got.Secure != test.secure {
			t.Fatalf("cookie = %#v, want %s secure=%v samesite=%v", got, test.name, test.secure, test.sameSite)
		}
		legacyCleared := false
		for _, cookie := range rr.Result().Cookies() {
			if cookie.Name == LegacyCookieName && cookie.MaxAge < 0 {
				legacyCleared = true
			}
		}
		if !legacyCleared {
			t.Fatal("legacy cookie was not cleared")
		}
	}
}

func TestClearAudienceCookiesPreservesScope(t *testing.T) {
	for _, test := range []struct {
		audience string
		secure   bool
		sameSite http.SameSite
	}{
		{DashboardAudience, false, http.SameSiteLaxMode},
		{DashboardAudience, true, http.SameSiteLaxMode},
		{EmbedAudience, false, http.SameSiteLaxMode},
		{EmbedAudience, true, http.SameSiteNoneMode},
	} {
		rr := httptest.NewRecorder()
		ClearCookieForAudience(rr, test.audience, test.secure)
		got := rr.Result().Cookies()[0]
		if got.Name != cookieName(test.audience) || got.Path != "/" || got.Value != "" || got.MaxAge >= 0 || !got.HttpOnly || got.Secure != test.secure || got.SameSite != test.sameSite {
			t.Fatalf("deletion cookie = %#v, want audience=%s secure=%v SameSite=%v", got, test.audience, test.secure, test.sameSite)
		}
	}
}
