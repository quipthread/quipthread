//go:build !cloud

package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	cloudpkg "github.com/quipthread/quipthread/cloud"
	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/middleware"
)

// RegisterBillingRoutes registers a minimal billing/status endpoint for
// self-hosted builds. Stripe checkout/portal/webhook are not available.
func RegisterBillingRoutes(r chi.Router, store db.Store, cfg *config.Config, _ cloudpkg.Store, _ *middleware.StoreCache) {
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAdmin(cfg.JWTSecret))
		r.Get("/api/billing/status", selfHostedBillingStatus(store))
	})
}

func selfHostedBillingStatus(store db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		commentsThisMonth, err := store.CountCommentsThisMonth()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to count comments"})
			return
		}
		sites, err := store.ListSites()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list sites"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"plan":                "business",
			"status":              "active",
			"trial_ends_at":       nil,
			"current_period_end":  nil,
			"interval":            "",
			"comments_this_month": commentsThisMonth,
			"comments_limit":      -1,
			"sites_count":         len(sites),
			"sites_limit":         nil,
		})
	}
}
