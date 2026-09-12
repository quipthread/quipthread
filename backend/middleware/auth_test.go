package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/models"
	"github.com/quipthread/quipthread/session"
)

const authTestSecret = "session-revocation-test-secret"

func TestRequireAuthBanUnbanAndLogoutAllRevokeSessions(t *testing.T) {
	store, err := db.NewSQLiteStoreForTest(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close() //nolint:errcheck

	user := &models.User{ID: "revocation-user", DisplayName: "User", Role: "commenter"}
	if err := store.UpsertUser(user); err != nil {
		t.Fatal(err)
	}
	issue := func() string {
		tok, err := session.IssueWithAudience(authTestSecret, session.EmbedAudience, user.ID, user.DisplayName, "sso", user.Role, "", user.EmbedSessionGeneration)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	protected := RequireAuthForAudience(authTestSecret, session.EmbedAudience, store)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	request := func(token string) int {
		r := httptest.NewRequest(http.MethodGet, "/api/comments", nil)
		r.AddCookie(&http.Cookie{Name: session.EmbedCookieName, Value: token})
		rr := httptest.NewRecorder()
		protected.ServeHTTP(rr, r)
		return rr.Code
	}

	token := issue()
	if got := request(token); got != http.StatusNoContent {
		t.Fatalf("initial session status = %d", got)
	}
	user.Banned = true
	if err := store.UpdateUser(user); err != nil {
		t.Fatal(err)
	}
	user, err = store.GetUser(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := request(token); got != http.StatusUnauthorized {
		t.Fatalf("banned session status = %d, want 401", got)
	}
	user.Banned = false
	if err := store.UpdateUser(user); err != nil {
		t.Fatal(err)
	}
	user, err = store.GetUser(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := request(token); got != http.StatusUnauthorized {
		t.Fatalf("pre-ban session revived after unban: status %d", got)
	}

	newToken := issue()
	if got := request(newToken); got != http.StatusNoContent {
		t.Fatalf("post-unban session status = %d", got)
	}
	if err := store.BumpSessionGeneration(user.ID); err != nil {
		t.Fatal(err)
	}
	if got := request(newToken); got != http.StatusUnauthorized {
		t.Fatalf("logout-all session status = %d, want 401", got)
	}
}

func TestSessionGenerationLogoutAllIsDashboardScoped(t *testing.T) {
	store, err := db.NewSQLiteStoreForTest(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close() //nolint:errcheck
	user := &models.User{ID: "scope-user", DisplayName: "User", Role: "admin"}
	if err := store.UpsertUser(user); err != nil {
		t.Fatal(err)
	}
	dashboardToken, err := session.IssueWithAudience(authTestSecret, session.DashboardAudience, user.ID, user.DisplayName, "email", user.Role, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	embedToken, err := session.IssueWithAudience(authTestSecret, session.EmbedAudience, user.ID, user.DisplayName, "sso", "commenter", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	check := func(audience, token string) int {
		h := RequireAuthForAudience(authTestSecret, audience, store)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.AddCookie(&http.Cookie{Name: session.CookieNameForAudience(audience), Value: token})
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, r)
		return rr.Code
	}
	if err := store.BumpSessionGeneration(user.ID, session.DashboardAudience); err != nil {
		t.Fatal(err)
	}
	if got := check(session.DashboardAudience, dashboardToken); got != http.StatusUnauthorized {
		t.Fatalf("dashboard token status = %d, want 401", got)
	}
	if got := check(session.EmbedAudience, embedToken); got != http.StatusNoContent {
		t.Fatalf("embed token status = %d, want 204", got)
	}
}
