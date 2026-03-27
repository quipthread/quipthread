//go:build !selfhosted

package handlers

import (
	"log"
	"net/http"
	"time"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/middleware"
)

type AnalyticsHandler struct {
	store db.Store
	cfg   *config.Config
}

func NewAnalyticsHandler(store db.Store, cfg *config.Config) *AnalyticsHandler {
	return &AnalyticsHandler{store: store, cfg: cfg}
}

func (h *AnalyticsHandler) db(r *http.Request) db.Store {
	if s, ok := db.StoreFromContext(r.Context()); ok {
		return s
	}
	return h.store
}

// GET /api/admin/analytics?siteId=&range=7d|30d|all
// siteId="all" is only permitted for Business tier.
func (h *AnalyticsHandler) Get(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	q := r.URL.Query()

	siteID := q.Get("siteId")
	if siteID == "" {
		writeError(w, r, http.StatusBadRequest, "siteId is required")
		return
	}

	rangeParam := q.Get("range")
	if rangeParam == "" {
		rangeParam = "30d"
	}

	var from time.Time
	switch rangeParam {
	case "7d":
		from = time.Now().UTC().AddDate(0, 0, -7)
	case "30d":
		from = time.Now().UTC().AddDate(0, 0, -30)
	case "all":
		// zero value = no lower bound
	default:
		writeError(w, r, http.StatusBadRequest, "range must be 7d, 30d, or all")
		return
	}

	// Resolve tier. Self-hosted (non-cloud) always gets business tier.
	tier := 2 // business
	if h.cfg.CloudMode {
		sub, err := middleware.GetCachedSubscription(middleware.AccountIDFromRequest(r), store)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "failed to check plan")
			return
		}
		switch sub.Plan {
		case "starter":
			tier = 0
		case "pro":
			tier = 1
		default: // business
			tier = 2
		}
		// hobby has no analytics access — frontend gates this, but enforce here too.
		if sub.Plan == "hobby" {
			writeError(w, r, http.StatusPaymentRequired, "plan_upgrade_required")
			return
		}
	}

	// "all sites" aggregate is Business-only.
	storeSiteID := siteID
	if siteID == "all" {
		if tier < 2 {
			writeError(w, r, http.StatusPaymentRequired, "plan_upgrade_required")
			return
		}
		storeSiteID = "" // empty = no site filter in the store
	}

	result, err := store.GetAnalytics(storeSiteID, from, 10, tier)
	if err != nil {
		log.Printf("analytics error: %v", err)
		writeError(w, r, http.StatusInternalServerError, "failed to fetch analytics")
		return
	}

	writeJSON(w, http.StatusOK, result)
}
