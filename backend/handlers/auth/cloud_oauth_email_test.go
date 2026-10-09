//go:build cloud

package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// An attacker who registers someone else's address with a password must not
// keep access once the owner signs in with a provider that verified it.
func TestCloudOAuthClaimsUnverifiedPasswordAccount(t *testing.T) {
	h, cs := newCloudProvisioningHandler(t, &recordingDBCreator{})

	req, rec := postJSON("/auth/email/register", `{"email":"victim@example.com","password":"attackerpass"}`)
	if !h.cloudEmailRegister(rec, req) || rec.Code != http.StatusOK {
		t.Fatalf("register: status %d, body %s", rec.Code, rec.Body.String())
	}
	var reg struct {
		PollToken string `json:"poll_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &reg); err != nil || reg.PollToken == "" {
		t.Fatalf("register response %s: %v", rec.Body.String(), err)
	}
	acc, err := cs.GetAccountByEmail("victim@example.com")
	if err != nil || acc == nil || acc.EmailVerified {
		t.Fatalf("registered account = %+v, %v", acc, err)
	}

	info := &UserInfo{Provider: "google", ProviderID: "g-victim", Email: "victim@example.com", EmailVerified: true, DisplayName: "Victim"}
	rec = httptest.NewRecorder()
	if !h.cloudUpsertAndIssueToken(rec, httptest.NewRequest(http.MethodGet, "/auth/google/callback", nil), info) {
		t.Fatal("OAuth sign-in not handled")
	}
	if rec.Code != http.StatusFound || !strings.HasSuffix(rec.Header().Get("Location"), "/dashboard/") {
		t.Fatalf("OAuth sign-in: status %d, location %q", rec.Code, rec.Header().Get("Location"))
	}
	claimed, err := cs.GetAccountByID(acc.ID)
	if err != nil || claimed == nil || !claimed.EmailVerified || claimed.PasswordHash != "" {
		t.Fatalf("claimed account = %+v, %v; want verified with no password", claimed, err)
	}
	if link, err := cs.GetOAuthLink("google", "g-victim"); err != nil || link == nil || link.AccountID != acc.ID {
		t.Fatalf("oauth link = %+v, %v", link, err)
	}

	// The attacker's signup tab must not be signed in by the verification.
	rec = httptest.NewRecorder()
	if !h.cloudEmailPoll(rec, httptest.NewRequest(http.MethodGet, "/auth/email/poll?token="+reg.PollToken, nil)) {
		t.Fatal("poll not handled")
	}
	if strings.Contains(rec.Body.String(), `"verified":true`) || rec.Header().Get("Set-Cookie") != "" {
		t.Fatalf("attacker poll succeeded: %s", rec.Body.String())
	}
	// Nor can the attacker sign in with the password they chose.
	req, rec = postJSON("/auth/email/login", `{"email":"victim@example.com","password":"attackerpass"}`)
	if !h.cloudEmailLogin(rec, req) || rec.Code != http.StatusUnauthorized {
		t.Fatalf("attacker password login: status %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestCloudOAuthUnverifiedEmailCannotMatchOrCreateAccount(t *testing.T) {
	h, cs := newCloudProvisioningHandler(t, &recordingDBCreator{})

	req, rec := postJSON("/auth/email/register", `{"email":"owner@example.com","password":"ownerpass1"}`)
	if !h.cloudEmailRegister(rec, req) || rec.Code != http.StatusOK {
		t.Fatalf("register: status %d", rec.Code)
	}
	owner, err := cs.GetAccountByEmail("owner@example.com")
	if err != nil || owner == nil {
		t.Fatalf("owner = %+v, %v", owner, err)
	}
	if err := cs.UpdateAccountEmailVerified(owner.ID); err != nil {
		t.Fatal(err)
	}

	for _, info := range []*UserInfo{
		{Provider: "google", ProviderID: "g-unverified", Email: "owner@example.com"},
		{Provider: "google", ProviderID: "g-new-unverified", Email: "new@example.com"},
		{Provider: "github", ProviderID: "gh-no-email"},
	} {
		rec := httptest.NewRecorder()
		if !h.cloudUpsertAndIssueToken(rec, httptest.NewRequest(http.MethodGet, "/cb", nil), info) {
			t.Fatalf("%s: not handled", info.ProviderID)
		}
		if loc := rec.Header().Get("Location"); !strings.HasSuffix(loc, "/login?error=oauth_email_unverified") {
			t.Fatalf("%s: location %q, want unverified-email error", info.ProviderID, loc)
		}
		if link, _ := cs.GetOAuthLink(info.Provider, info.ProviderID); link != nil {
			t.Fatalf("%s: linked to account %s", info.ProviderID, link.AccountID)
		}
	}
	if got, _ := cs.GetAccountByID(owner.ID); got == nil || got.PasswordHash == "" {
		t.Fatal("verified owner's password was removed")
	}
	if accs := mustListAccounts(t, cs); len(accs) != 1 {
		t.Fatalf("accounts = %d, want only the owner", len(accs))
	}
}

func TestPickGithubEmail(t *testing.T) {
	emails := []githubEmail{
		{Email: "primary@example.com", Primary: true, Verified: true},
		{Email: "public@example.com", Verified: true},
		{Email: "unverified@example.com"},
	}
	for _, tc := range []struct {
		profile, want string
		verified      bool
	}{
		{"Public@Example.com", "Public@Example.com", true},
		{"unverified@example.com", "unverified@example.com", false},
		{"elsewhere@example.com", "elsewhere@example.com", false},
		{"", "primary@example.com", true},
	} {
		got, verified := pickGithubEmail(tc.profile, emails)
		if got != tc.want || verified != tc.verified {
			t.Errorf("profile %q: got %q verified=%v, want %q verified=%v", tc.profile, got, verified, tc.want, tc.verified)
		}
	}
	if got, verified := pickGithubEmail("", []githubEmail{{Email: "x@example.com", Primary: true}}); got != "" || verified {
		t.Errorf("unverified primary: got %q verified=%v", got, verified)
	}
}
