package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/models"
	"github.com/quipthread/quipthread/session"
)

// Safeguard: the raw SSO secret must never appear in any site list/get/update
// API response. The one-time generation response in GenerateSSOSecret (cloud
// builds only) is the only raw-secret return path.

var testSSOSecret = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

var testSSOSecretPtr = &testSSOSecret

func newSanitizeTestHandler(t *testing.T) (*AdminHandler, db.Store) {
	t.Helper()
	store, err := db.NewSQLiteStoreForTest(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	h := NewAdminHandler(store, &config.Config{JWTSecret: "test-secret"})
	return h, store
}

func serveSitesRoute(t *testing.T, h *AdminHandler, method, pattern string, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Method(method, pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch pattern {
		case "/api/admin/sites":
			h.ListSites(w, r)
		case "/api/admin/sites/{id}":
			h.UpdateSite(w, r)
		default:
			t.Errorf("unexpected pattern %q", pattern)
		}
	}))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestSiteModelJSONNeverSerializesSSOSecret(t *testing.T) {
	site := &models.Site{ID: "s1", Domain: "example.com", SSOSecret: testSSOSecretPtr}

	body, err := json.Marshal(site)
	if err != nil {
		t.Fatalf("marshal site: %v", err)
	}
	if strings.Contains(string(body), testSSOSecret) || strings.Contains(string(body), "sso_secret") {
		t.Fatalf("site JSON leaks raw secret: %s", body)
	}
}

func TestListSitesResponseOmitsSSOSecret(t *testing.T) {
	h, store := newSanitizeTestHandler(t)
	if err := store.CreateSite(&models.Site{ID: "s1", OwnerID: "u1", Domain: "example.com"}); err != nil {
		t.Fatalf("seed site: %v", err)
	}
	if err := store.UpdateSiteSSO("s1", testSSOSecretPtr); err != nil {
		t.Fatalf("seed sso secret: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/sites", nil)
	rec := serveSitesRoute(t, h, http.MethodGet, "/api/admin/sites", req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	assertNoRawSecret(t, rec.Body.String(), true)
}

func TestUpdateSiteResponseOmitsSSOSecret(t *testing.T) {
	h, store := newSanitizeTestHandler(t)
	if err := store.CreateSite(&models.Site{ID: "s1", OwnerID: "u1", Domain: "example.com"}); err != nil {
		t.Fatalf("seed site: %v", err)
	}
	if err := store.UpdateSiteSSO("s1", testSSOSecretPtr); err != nil {
		t.Fatalf("seed sso secret: %v", err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/admin/sites/s1", strings.NewReader(`{"theme":"dark"}`))
	rec := serveSitesRoute(t, h, http.MethodPatch, "/api/admin/sites/{id}", req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	assertNoRawSecret(t, rec.Body.String(), true)

	var view struct {
		models.Site
		SSOEnabled bool `json:"sso_enabled"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !view.SSOEnabled {
		t.Error("sso_enabled = false, want true for a site with a configured secret")
	}
	if view.Theme != "dark" {
		t.Errorf("theme = %q, want dark", view.Theme)
	}
}

func TestUpdateSiteRejectsUnsafeNotificationIntervals(t *testing.T) {
	h, store := newSanitizeTestHandler(t)
	if err := store.CreateSite(&models.Site{ID: "s1", OwnerID: "u1", Domain: "example.com"}); err != nil {
		t.Fatalf("seed site: %v", err)
	}

	for _, interval := range []int{db.MinSiteDigestIntervalSeconds - 1, db.MaxSiteDigestIntervalSeconds + 1} {
		req := httptest.NewRequest(http.MethodPatch, "/api/admin/sites/s1", strings.NewReader(fmt.Sprintf(`{"notify_interval":%d}`, interval)))
		rec := serveSitesRoute(t, h, http.MethodPatch, "/api/admin/sites/{id}", req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("interval=%d status=%d body=%s, want 400", interval, rec.Code, rec.Body.String())
		}
	}

	for _, interval := range []int{db.MinSiteDigestIntervalSeconds, db.MaxSiteDigestIntervalSeconds} {
		req := httptest.NewRequest(http.MethodPatch, "/api/admin/sites/s1", strings.NewReader(fmt.Sprintf(`{"notify_interval":%d}`, interval)))
		rec := serveSitesRoute(t, h, http.MethodPatch, "/api/admin/sites/{id}", req)
		if rec.Code != http.StatusOK {
			t.Fatalf("interval=%d status=%d body=%s, want 200", interval, rec.Code, rec.Body.String())
		}
		site, err := store.GetSite("s1")
		if err != nil {
			t.Fatal(err)
		}
		if site.NotifyInterval == nil || *site.NotifyInterval != interval {
			t.Fatalf("stored interval = %v, want %d", site.NotifyInterval, interval)
		}
	}
}

func TestCreateSiteResponseOmitsSSOSecretAndReportsDisabled(t *testing.T) {
	h, _ := newSanitizeTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/sites", strings.NewReader(`{"domain":"new.example.com"}`))
	ctx := context.WithValue(req.Context(), session.UserKey, &session.Claims{Sub: "u1", Role: "admin"})
	req = req.WithContext(ctx)

	r := chi.NewRouter()
	r.Post("/api/admin/sites", h.CreateSite)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	assertNoRawSecret(t, rec.Body.String(), false)

	var view struct {
		SSOEnabled bool `json:"sso_enabled"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if view.SSOEnabled {
		t.Error("sso_enabled = true, want false for a fresh site")
	}
}

// assertNoRawSecret fails if the response body contains the raw test secret.
// When expectIndicator is true it also requires the sso_enabled indicator to
// be present so callers cannot silently lose SSO state visibility.
func assertNoRawSecret(t *testing.T, body string, expectIndicator bool) {
	t.Helper()
	if strings.Contains(body, testSSOSecret) {
		t.Fatalf("response leaks raw SSO secret: %s", body)
	}
	if strings.Contains(body, `"sso_secret"`) {
		t.Fatalf("response contains sso_secret field: %s", body)
	}
	if expectIndicator && !strings.Contains(body, `"sso_enabled"`) {
		t.Fatalf("response missing sso_enabled indicator: %s", body)
	}
}
