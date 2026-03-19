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
// non-cloud builds. Stripe checkout/portal/webhook are not available.
func RegisterBillingRoutes(r chi.Router, store db.Store, cfg *config.Config, _ cloudpkg.Store, _ *middleware.StoreCache) {
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAdmin(cfg.JWTSecret))
		r.Get("/api/billing/status", nonCloudBillingStatus(store))
	})
}

func nonCloudBillingStatus(store db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sub, _ := middleware.GetCachedSubscription("", store)
		limits := middleware.PlanLimits[sub.Plan]

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

		var sitesLimit interface{} = limits[1]
		if limits[1] == -1 {
			sitesLimit = nil
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"plan":                sub.Plan,
			"status":              "active",
			"trial_ends_at":       nil,
			"current_period_end":  nil,
			"interval":            "",
			"comments_this_month": commentsThisMonth,
			"comments_limit":      limits[0],
			"sites_count":         len(sites),
			"sites_limit":         sitesLimit,
		})
	}
}
