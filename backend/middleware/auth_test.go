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
		tok, err := session.IssueEmbed(authTestSecret, user.ID, user.DisplayName, "sso", user.Role, "", "site-1", user.EmbedSessionGeneration)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	protected := RequireAuthForAudience(authTestSecret, session.EmbedAudience, store)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	request := func(token string) int {
		r := httptest.NewRequest(http.MethodGet, "/api/comments?siteId=site-1", nil)
		r.Header.Set("Authorization", "Bearer "+token)
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
	embedToken, err := session.IssueEmbed(authTestSecret, user.ID, user.DisplayName, "sso", "commenter", "", "site-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	check := func(audience, token string) int {
		h := RequireAuthForAudience(authTestSecret, audience, store)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
		r := httptest.NewRequest(http.MethodGet, "/?siteId=site-1", nil)
		if audience == session.EmbedAudience {
			r.Header.Set("Authorization", "Bearer "+token)
		} else {
			r.AddCookie(&http.Cookie{Name: session.DashboardCookieName, Value: token})
		}
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

func TestEmbedAuthIsBearerOnlyAndSiteBound(t *testing.T) {
	store, err := db.NewSQLiteStoreForTest(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close() //nolint:errcheck
	user := &models.User{ID: "bound-user", DisplayName: "User", Role: "commenter"}
	if err := store.UpsertUser(user); err != nil {
		t.Fatal(err)
	}
	token, err := session.IssueEmbed(authTestSecret, user.ID, user.DisplayName, "github", user.Role, "", "site-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	h := RequireAuthForAudience(authTestSecret, session.EmbedAudience, store)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, tc := range []struct {
		name   string
		target string
		bearer bool
		cookie bool
		want   int
	}{
		{"bearer for its own site", "/api/comments?siteId=site-1", true, false, http.StatusNoContent},
		{"bearer for another site", "/api/comments?siteId=site-2", true, false, http.StatusUnauthorized},
		{"bearer without a site", "/api/comments", true, false, http.StatusUnauthorized},
		{"token sent as a cookie", "/api/comments?siteId=site-1", false, true, http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tc.target, nil)
			if tc.bearer {
				r.Header.Set("Authorization", "Bearer "+token)
			}
			if tc.cookie {
				r.AddCookie(&http.Cookie{Name: "quipthread_embed_session", Value: token})
				r.AddCookie(&http.Cookie{Name: session.DashboardCookieName, Value: token})
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, r)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d", rr.Code, tc.want)
			}
		})
	}
}

func TestEmbedAuthRequiresResolvedTenantMatch(t *testing.T) {
	store, err := db.NewSQLiteStoreForTest(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close() //nolint:errcheck
	user := &models.User{ID: "tenant-user", DisplayName: "User", Role: "commenter"}
	if err := store.UpsertUser(user); err != nil {
		t.Fatal(err)
	}
	token, err := session.IssueEmbed(authTestSecret, user.ID, user.DisplayName, "github", user.Role, "acct-1", "site-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	h := RequireAuthForAudience(authTestSecret, session.EmbedAudience, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, tc := range []struct {
		name string
		pt   db.PublicTenant
		want int
	}{
		{"same site and account", db.PublicTenant{Store: store, SiteID: "site-1", AccountID: "acct-1"}, http.StatusNoContent},
		{"same site id, other account", db.PublicTenant{Store: store, SiteID: "site-1", AccountID: "acct-2"}, http.StatusUnauthorized},
		{"other site", db.PublicTenant{Store: store, SiteID: "site-2", AccountID: "acct-1"}, http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The query names site-1 in every case: the resolved tenant, not
			// the query string, is authoritative once it exists.
			r := httptest.NewRequest(http.MethodGet, "/api/comments?siteId=site-1", nil)
			r.Header.Set("Authorization", "Bearer "+token)
			ctx := db.WithPublicTenant(r.Context(), tc.pt)
			ctx = db.WithStore(ctx, tc.pt.Store)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, r.WithContext(ctx))
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d", rr.Code, tc.want)
			}
		})
	}
}
