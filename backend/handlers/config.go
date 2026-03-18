package handlers

import (
	"net/http"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
)

// ConfigHandler serves public configuration to the embed widget.
type ConfigHandler struct {
	turnstileSiteKey string
	store            db.Store
}

func NewConfigHandler(cfg *config.Config, store db.Store) *ConfigHandler {
	return &ConfigHandler{turnstileSiteKey: cfg.TurnstileSiteKey, store: store}
}

func (h *ConfigHandler) db(r *http.Request) db.Store {
	if s, ok := db.StoreFromContext(r.Context()); ok {
		return s
	}
	return h.store
}

// GET /api/config?siteId= — returns public widget configuration.
// The Turnstile site key is public by design (it's embedded in the page).
func (h *ConfigHandler) PublicConfig(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	theme := "auto"
	if siteID := r.URL.Query().Get("siteId"); siteID != "" {
		if site, err := store.GetSite(siteID); err == nil && site != nil {
			theme = site.Theme
		}
	}
	// Use account-level Turnstile site key if configured, fall back to global env var.
	turnstileKey := h.turnstileSiteKey
	if accountKey, _, err := store.GetTurnstileKeys(); err == nil && accountKey != "" {
		turnstileKey = accountKey
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"turnstileSiteKey": turnstileKey,
		"theme":            theme,
	})
}
