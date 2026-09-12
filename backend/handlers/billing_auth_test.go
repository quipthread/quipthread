//go:build !cloud

package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/models"
	"github.com/quipthread/quipthread/session"
)

func TestSelfHostedBillingRejectsRevokedDashboardToken(t *testing.T) {
	store, err := db.NewSQLiteStoreForTest(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close() //nolint:errcheck
	if err := store.UpsertUser(&models.User{ID: "billing-admin", DisplayName: "Admin", Role: "admin"}); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{JWTSecret: "billing-auth-secret"}
	r := chi.NewRouter()
	RegisterBillingRoutes(r, store, cfg, nil, nil)
	token, err := session.IssueWithAudience(cfg.JWTSecret, session.DashboardAudience, "billing-admin", "Admin", "email", "admin", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	request := func() int {
		req := httptest.NewRequest(http.MethodGet, "/api/billing/status", nil)
		req.AddCookie(&http.Cookie{Name: session.DashboardCookieName, Value: token})
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr.Code
	}
	if got := request(); got != http.StatusOK {
		t.Fatalf("valid token status = %d", got)
	}
	if err := store.BumpSessionGeneration("billing-admin", session.DashboardAudience); err != nil {
		t.Fatal(err)
	}
	if got := request(); got != http.StatusUnauthorized {
		t.Fatalf("revoked token status = %d, want 401", got)
	}
}
