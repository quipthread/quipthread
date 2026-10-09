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
	tok, err := IssueWithAudience(testSecret, DashboardAudience, "u1", "Alice", "github", "admin", "acc1", 0)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if tok == "" {
		t.Fatal("Issue returned empty token")
	}

	claims, err := ParseForAudience(testSecret, tok, DashboardAudience)
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
	tok, err := IssueWithAudience(testSecret, DashboardAudience, "u1", "Alice", "github", "admin", "", 0)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := ParseForAudience("wrong-secret", tok, DashboardAudience); err == nil {
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
	if _, err := ParseForAudience(testSecret, tok, DashboardAudience); err == nil {
		t.Error("Parse of expired token: expected error, got nil")
	}
}

func TestParse_MalformedToken(t *testing.T) {
	cases := []string{"", "not-a-jwt", "a.b", "a.b.c.d"}
	for _, s := range cases {
		if _, err := ParseForAudience(testSecret, s, DashboardAudience); err == nil {
			t.Errorf("Parse(%q): expected error, got nil", s)
		}
	}
}

func TestParseForAudienceRejectsCrossAudience(t *testing.T) {
	tok, err := IssueEmbed(testSecret, "u1", "Alice", "sso", "commenter", "", "site-1", 0)
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
	if _, err := ParseForAudience(testSecret, tok, DashboardAudience); err == nil {
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

func TestDashboardCookie(t *testing.T) {
	for _, secure := range []bool{false, true} {
		rr := httptest.NewRecorder()
		SetCookie(rr, "token", secure)
		var got *http.Cookie
		legacyCleared := false
		for _, cookie := range rr.Result().Cookies() {
			switch {
			case cookie.Name == DashboardCookieName:
				got = cookie
			case cookie.Name == LegacyCookieName && cookie.MaxAge < 0:
				legacyCleared = true
			}
		}
		if got == nil || got.Value != "token" || got.SameSite != http.SameSiteLaxMode || got.Secure != secure || !got.HttpOnly {
			t.Fatalf("cookie = %#v, want dashboard Lax secure=%v", got, secure)
		}
		if !legacyCleared {
			t.Fatal("legacy cookie was not cleared")
		}
	}
}

func TestClearDashboardCookiePreservesScope(t *testing.T) {
	for _, secure := range []bool{false, true} {
		rr := httptest.NewRecorder()
		ClearDashboardCookie(rr, secure)
		got := rr.Result().Cookies()[0]
		if got.Name != DashboardCookieName || got.Path != "/" || got.Value != "" || got.MaxAge >= 0 || !got.HttpOnly || got.Secure != secure || got.SameSite != http.SameSiteLaxMode {
			t.Fatalf("deletion cookie = %#v, want dashboard secure=%v", got, secure)
		}
	}
}

func TestEmbedTokensAreSiteBound(t *testing.T) {
	if _, err := IssueWithAudience(testSecret, EmbedAudience, "u", "User", "github", "commenter", "", 0); err == nil {
		t.Fatal("IssueWithAudience issued an embed token without a site")
	}
	if _, err := IssueEmbed(testSecret, "u", "User", "github", "commenter", "", "", 0); err == nil {
		t.Fatal("IssueEmbed accepted an empty site id")
	}
	tok, err := IssueEmbed(testSecret, "u", "User", "github", "commenter", "acct-1", "site-1", 3)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ParseForAudience(testSecret, tok, EmbedAudience)
	if err != nil {
		t.Fatal(err)
	}
	if claims.SiteID != "site-1" || claims.AccountID != "acct-1" || claims.SessionGeneration != 3 {
		t.Fatalf("claims = %#v", claims)
	}
	if _, err := ParseForAudience(testSecret, tok, DashboardAudience); err == nil {
		t.Fatal("embed token accepted as a dashboard session")
	}
}

func TestBearerToken(t *testing.T) {
	for header, want := range map[string]string{
		"":                "",
		"Bearer":          "",
		"Bearer ":         "",
		"Basic abc":       "",
		"Bearer abc":      "abc",
		"bearer abc":      "abc",
		"  Bearer  abc  ": "abc",
	} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if header != "" {
			r.Header.Set("Authorization", header)
		}
		if got := BearerToken(r); got != want {
			t.Errorf("BearerToken(%q) = %q, want %q", header, got, want)
		}
	}
}
