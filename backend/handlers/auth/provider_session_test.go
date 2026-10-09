package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/middleware"
	"github.com/quipthread/quipthread/models"
	"github.com/quipthread/quipthread/session"
)

func TestOAuthAudienceStateIsBoundAndConsumed(t *testing.T) {
	rr := httptest.NewRecorder()
	setStateCookie(rr, oauthStateBinding{State: "state-1", Audience: session.EmbedAudience, Redirect: "https://publisher.example.test/article"}, false)
	r := httptest.NewRequest(http.MethodGet, "/auth/github/callback?state=state-1", nil)
	for _, cookie := range rr.Result().Cookies() {
		r.AddCookie(cookie)
	}
	if !validateStateCookie(r, "state-1") {
		t.Fatal("state did not validate")
	}
	out := httptest.NewRecorder()
	binding, ok := consumeStateBinding(out, r, "state-1", false)
	if !ok || binding.Audience != session.EmbedAudience || binding.Redirect != "https://publisher.example.test/article" {
		t.Fatalf("binding = %#v, valid=%v", binding, ok)
	}
	cleared := false
	for _, cookie := range out.Result().Cookies() {
		if cookie.Name == stateAudienceCookieName && cookie.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("state audience cookie was not cleared")
	}
}

func TestInterruptedEmbedFlowCannotRedirectLaterDashboardFlow(t *testing.T) {
	h := &Handler{config: &config.Config{
		BaseURL:        "https://app.example.test",
		AllowedOrigins: []string{"https://publisher.example.test"},
	}}
	first := httptest.NewRecorder()
	setStateCookie(first, oauthStateBinding{State: "embed-state", Audience: session.EmbedAudience, Redirect: "https://publisher.example.test/article"}, false)
	second := httptest.NewRecorder()
	setStateCookie(second, oauthStateBinding{State: "dashboard-state", Audience: session.DashboardAudience, Redirect: h.config.BaseURL}, false)

	dashboardRequest := httptest.NewRequest(http.MethodGet, "/auth/github/callback?state=dashboard-state", nil)
	for _, cookie := range second.Result().Cookies() {
		dashboardRequest.AddCookie(cookie)
	}
	binding, ok := consumeStateBinding(httptest.NewRecorder(), dashboardRequest, "dashboard-state", false)
	if !ok || binding.Audience != session.DashboardAudience || binding.Redirect != h.config.BaseURL {
		t.Fatalf("dashboard binding = %#v, valid=%v", binding, ok)
	}

	interruptedRequest := httptest.NewRequest(http.MethodGet, "/auth/github/callback?state=embed-state", nil)
	for _, cookie := range second.Result().Cookies() {
		interruptedRequest.AddCookie(cookie)
	}
	if _, ok := consumeStateBinding(httptest.NewRecorder(), interruptedRequest, "embed-state", false); ok {
		t.Fatal("interrupted embed state was accepted after dashboard flow replaced it")
	}
}

// newEmbedOAuthHandler returns a self-hosted handler whose store holds one
// site on publisher.example.test, wired for embed sign-in.
func newEmbedOAuthHandler(t *testing.T, allowed ...string) (*Handler, db.Store) {
	t.Helper()
	store, err := db.NewSQLiteStoreForTest(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() }) //nolint:errcheck
	if err := store.CreateSite(&models.Site{ID: "site-1", OwnerID: "owner", Domain: "publisher.example.test", Theme: "auto"}); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{BaseURL: "https://app.example.test", JWTSecret: "embed-oauth-secret", AllowedOrigins: allowed}
	h := &Handler{store: store, config: cfg}
	h.SetEmbedSites(middleware.NewEmbedSites(store, nil, nil, cfg))
	return h, store
}

func TestResolveOAuthFlowAudienceAndRedirectPolicy(t *testing.T) {
	h, _ := newEmbedOAuthHandler(t, "https://allowed.example.test")
	for _, tc := range []struct {
		name         string
		query        string
		wantErr      bool
		wantAudience string
		wantRedirect string
	}{
		{"no audience is dashboard, even with an external returnTo", "returnTo=https://publisher.example.test/article", false, session.DashboardAudience, "https://app.example.test"},
		{"explicit dashboard", "audience=dashboard&returnTo=https://publisher.example.test/article", false, session.DashboardAudience, "https://app.example.test"},
		{"embed on the site's domain", "audience=embed&siteId=site-1&returnTo=https://publisher.example.test/article?p=1%23old", false, session.EmbedAudience, "https://publisher.example.test/article?p=1"},
		{"embed on a self-hosted allowed origin", "audience=embed&siteId=site-1&returnTo=https://allowed.example.test/a", false, session.EmbedAudience, "https://allowed.example.test/a"},
		{"embed on another domain", "audience=embed&siteId=site-1&returnTo=https://evil.example.test/a", true, "", ""},
		{"embed on a subdomain", "audience=embed&siteId=site-1&returnTo=https://www.publisher.example.test/a", true, "", ""},
		{"embed with a non-http scheme", "audience=embed&siteId=site-1&returnTo=javascript:alert(1)", true, "", ""},
		{"embed without a site", "audience=embed&returnTo=https://publisher.example.test/a", true, "", ""},
		{"embed for an unknown site", "audience=embed&siteId=nope&returnTo=https://publisher.example.test/a", true, "", ""},
		{"unknown audience", "audience=admin", true, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binding, err := h.resolveOAuthFlow(httptest.NewRequest(http.MethodGet, "/auth/github/login?"+tc.query, nil))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("binding = %#v, want error", binding)
				}
				return
			}
			if err != nil || binding.Audience != tc.wantAudience || binding.Redirect != tc.wantRedirect {
				t.Fatalf("binding = %#v, err = %v", binding, err)
			}
			if tc.wantAudience == session.EmbedAudience && binding.SiteID != "site-1" {
				t.Fatalf("embed binding site = %q", binding.SiteID)
			}
		})
	}
}

// embedCallback runs the shared OAuth callback for an embed binding with a
// fake provider exchange and returns the redirect Location.
func embedCallback(t *testing.T, h *Handler, query string, exchange func(context.Context, *http.Request) (*UserInfo, error)) string {
	t.Helper()
	rr := httptest.NewRecorder()
	setStateCookie(rr, oauthStateBinding{State: "s1", Audience: session.EmbedAudience, Redirect: "https://publisher.example.test/article", SiteID: "site-1"}, false)
	r := httptest.NewRequest(http.MethodGet, "/auth/github/callback?state=s1"+query, nil)
	for _, cookie := range rr.Result().Cookies() {
		r.AddCookie(cookie)
	}
	out := httptest.NewRecorder()
	h.oauthCallback(out, r, exchange)
	if out.Code != http.StatusFound {
		t.Fatalf("status = %d, body = %s", out.Code, out.Body.String())
	}
	for _, cookie := range out.Result().Cookies() {
		if cookie.Name == session.DashboardCookieName && cookie.MaxAge >= 0 {
			t.Fatal("embed sign-in set a dashboard session cookie")
		}
	}
	return out.Header().Get("Location")
}

func TestEmbedOAuthCallbackReturnsSiteBoundTokenInFragment(t *testing.T) {
	h, store := newEmbedOAuthHandler(t)
	reader := &UserInfo{Provider: "github", ProviderID: "gh-1", DisplayName: "Reader", Email: "reader@example.test"}
	loc := embedCallback(t, h, "", func(context.Context, *http.Request) (*UserInfo, error) { return reader, nil })

	const prefix = "https://publisher.example.test/article#qt_auth="
	if !strings.HasPrefix(loc, prefix) {
		t.Fatalf("Location = %q", loc)
	}
	raw, err := url.QueryUnescape(strings.TrimPrefix(loc, prefix))
	if err != nil {
		t.Fatal(err)
	}
	claims, err := session.ParseForAudience(h.config.JWTSecret, raw, session.EmbedAudience)
	if err != nil {
		t.Fatal(err)
	}
	if claims.SiteID != "site-1" || claims.Role != "commenter" {
		t.Fatalf("claims = %#v", claims)
	}
	// The first user in an empty store must not become an admin via the embed.
	user, err := store.GetUser(claims.Sub)
	if err != nil || user == nil || user.Role != "commenter" {
		t.Fatalf("user = %#v, err = %v", user, err)
	}
}

func TestEmbedOAuthCallbackFailuresReturnErrorFragment(t *testing.T) {
	h, store := newEmbedOAuthHandler(t)
	banned := &models.User{ID: "banned-user", DisplayName: "Banned", Role: "commenter", Banned: true}
	if err := store.UpsertUser(banned); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateIdentity(&models.UserIdentity{UserID: banned.ID, Provider: "github", ProviderID: "gh-banned"}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		query    string
		exchange func(context.Context, *http.Request) (*UserInfo, error)
		code     string
	}{
		{"provider denied", "&error=access_denied", nil, "denied"},
		{"exchange failed", "", func(context.Context, *http.Request) (*UserInfo, error) { return nil, errors.New("boom") }, "provider_error"},
		{"banned reader", "", func(context.Context, *http.Request) (*UserInfo, error) {
			return &UserInfo{Provider: "github", ProviderID: "gh-banned", DisplayName: "Banned"}, nil
		}, "banned"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loc := embedCallback(t, h, tc.query, tc.exchange)
			if want := "https://publisher.example.test/article#qt_auth_error=" + tc.code; loc != want {
				t.Fatalf("Location = %q, want %q", loc, want)
			}
		})
	}
}

func TestOAuthAudienceStateOverridesExternalReturnToFallback(t *testing.T) {
	h := &Handler{config: &config.Config{BaseURL: "https://app.example.test", AllowedOrigins: []string{"https://publisher.example.test"}}}
	rr := httptest.NewRecorder()
	setStateCookie(rr, oauthStateBinding{State: "state-2", Audience: session.DashboardAudience, Redirect: h.config.BaseURL}, false)
	http.SetCookie(rr, &http.Cookie{Name: returnToCookieName, Value: "https://publisher.example.test/article", Path: "/"})
	r := httptest.NewRequest(http.MethodGet, "/auth/github/callback?state=state-2", nil)
	for _, cookie := range rr.Result().Cookies() {
		r.AddCookie(cookie)
	}
	out := httptest.NewRecorder()
	binding, ok := consumeStateBinding(out, r, "state-2", false)
	if !ok || binding.Audience != session.DashboardAudience || binding.Redirect != h.config.BaseURL {
		t.Fatalf("binding = %#v, valid=%v", binding, ok)
	}
}

func TestOAuthLinkValidationRejectsBannedOrRevokedDashboardUser(t *testing.T) {
	store, err := db.NewSQLiteStoreForTest(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close() //nolint:errcheck
	user := &models.User{ID: "oauth-link-user", DisplayName: "User", Role: "admin"}
	if err := store.UpsertUser(user); err != nil {
		t.Fatal(err)
	}
	h := &Handler{store: store, config: &config.Config{JWTSecret: "oauth-link-secret"}}
	token, err := session.IssueWithAudience(h.config.JWTSecret, session.DashboardAudience, user.ID, user.DisplayName, "email", user.Role, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	request := func() *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/auth/github/link", nil)
		r.AddCookie(&http.Cookie{Name: session.DashboardCookieName, Value: token})
		return r
	}
	if !h.validateDashboardUser(request(), user.ID) {
		t.Fatal("current dashboard user was rejected")
	}
	user.Banned = true
	if err := store.UpdateUser(user); err != nil {
		t.Fatal(err)
	}
	if h.validateDashboardUser(request(), user.ID) {
		t.Fatal("banned dashboard user was accepted")
	}
	user, err = store.GetUser(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	user.Banned = false
	if err := store.UpdateUser(user); err != nil {
		t.Fatal(err)
	}
	user, err = store.GetUser(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	token, err = session.IssueWithAudience(h.config.JWTSecret, session.DashboardAudience, user.ID, user.DisplayName, "email", user.Role, "", user.DashboardSessionGeneration)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.BumpSessionGeneration(user.ID, session.DashboardAudience); err != nil {
		t.Fatal(err)
	}
	if h.validateDashboardUser(request(), user.ID) {
		t.Fatal("generation-revoked dashboard user was accepted")
	}
}
