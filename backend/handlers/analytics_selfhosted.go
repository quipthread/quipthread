//go:build selfhosted

package handlers

import (
	"net/http"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
)

// AnalyticsHandler is a stub for the selfhosted build. Analytics is a cloud-only feature.
type AnalyticsHandler struct {
	store db.Store
	cfg   *config.Config
}

func NewAnalyticsHandler(store db.Store, cfg *config.Config) *AnalyticsHandler {
	return &AnalyticsHandler{store: store, cfg: cfg}
}

// Get returns 404 in selfhosted builds — analytics is not available.
func (h *AnalyticsHandler) Get(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, "not_available")
}
