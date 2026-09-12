package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
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

func TestResolveOAuthFlowAudienceAndRedirectPolicy(t *testing.T) {
	h := &Handler{config: &config.Config{
		BaseURL:        "https://app.example.test",
		AllowedOrigins: []string{"https://publisher.example.test"},
	}}
	legacy := httptest.NewRequest(http.MethodGet, "/auth/github/login?returnTo=https://publisher.example.test/article", nil)
	binding, err := h.resolveOAuthFlow(legacy)
	if err != nil || binding.Audience != session.EmbedAudience || binding.Redirect != "https://publisher.example.test/article" {
		t.Fatalf("legacy flow = %#v, err=%v", binding, err)
	}
	explicitDashboard := httptest.NewRequest(http.MethodGet, "/auth/github/login?audience=dashboard&returnTo=https://publisher.example.test/article", nil)
	binding, err = h.resolveOAuthFlow(explicitDashboard)
	if err != nil || binding.Audience != session.DashboardAudience || binding.Redirect != h.config.BaseURL {
		t.Fatalf("explicit dashboard flow = %#v, err=%v", binding, err)
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
