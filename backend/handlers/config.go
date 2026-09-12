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
	// publicResolver is true when cloud mode + PUBLIC_SITE_RESOLVER_ENABLED
	// are active. In that mode PublicConfig requires an explicit public-tenant
	// context and never touches the handler/global store.
	publicResolver bool
}

func NewConfigHandler(cfg *config.Config, store db.Store) *ConfigHandler {
	return &ConfigHandler{
		turnstileSiteKey: cfg.TurnstileSiteKey,
		store:            store,
		publicResolver:   cfg.CloudMode && cfg.PublicSiteResolverEnabled,
	}
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
	if h.publicResolver {
		h.publicConfig(w, r)
		return
	}
	h.legacyConfig(w, r)
}

// publicConfig serves config only from the explicitly resolved public tenant.
// A missing public context or any site/key lookup failure is an error — never
// a silent fallback to default site/global Turnstile information, which would
// leak cross-tenant configuration.
func (h *ConfigHandler) publicConfig(w http.ResponseWriter, r *http.Request) {
	pt, ok := db.PublicTenantFromContext(r.Context())
	if !ok || pt.Store == nil || pt.SiteID == "" {
		writeError(w, r, http.StatusInternalServerError, "internal_error")
		return
	}
	if q := r.URL.Query().Get("siteId"); q != "" && q != pt.SiteID {
		writeError(w, r, http.StatusInternalServerError, "internal_error")
		return
	}

	site, err := pt.Store.GetSite(pt.SiteID)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	if site == nil {
		// The resolver verifies existence before injecting the context;
		// reaching this point means the verified-context invariant broke.
		writeError(w, r, http.StatusInternalServerError, "internal_error")
		return
	}

	accountKey, _, err := pt.Store.GetTurnstileKeys()
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	// Use account-level Turnstile site key if configured, fall back to global env var.
	turnstileKey := h.turnstileSiteKey
	if accountKey != "" {
		turnstileKey = accountKey
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"turnstileSiteKey": turnstileKey,
		"theme":            site.Theme,
	})
}

// legacyConfig preserves pre-resolver behavior (self-hosted and cloud without
// the resolver flag): context store if present, else the global store, with
// silent defaults on lookup failures.
func (h *ConfigHandler) legacyConfig(w http.ResponseWriter, r *http.Request) {
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
