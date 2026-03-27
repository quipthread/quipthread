package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/middleware"
	"github.com/quipthread/quipthread/session"
)

type AccountHandler struct {
	store db.Store
	cfg   *config.Config
}

func NewAccountHandler(store db.Store, cfg *config.Config) *AccountHandler {
	return &AccountHandler{store: store, cfg: cfg}
}

func claimsFromReq(r *http.Request) *session.Claims {
	c, _ := r.Context().Value(session.UserKey).(*session.Claims)
	return c
}

// db returns the tenant store from context when in cloud mode, falling back to
// the global store for self-hosted deployments.
func (h *AccountHandler) db(r *http.Request) db.Store {
	if s, ok := db.StoreFromContext(r.Context()); ok {
		return s
	}
	return h.store
}

// GET /api/admin/account
func (h *AccountHandler) Get(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	claims := claimsFromReq(r)
	user, err := store.GetUser(claims.Sub)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to get user")
		return
	}
	if user == nil {
		writeError(w, r, http.StatusNotFound, "user not found")
		return
	}
	identities, err := store.ListUserIdentities(claims.Sub)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to get identities")
		return
	}
	providers := make([]string, 0, len(identities))
	providerUsernames := make(map[string]string, len(identities))
	for _, id := range identities {
		providers = append(providers, id.Provider)
		if id.Username != "" {
			providerUsernames[id.Provider] = id.Username
		}
	}

	configured := make([]string, 0, 2)
	if h.cfg.GitHubClientID != "" {
		configured = append(configured, "github")
	}
	if h.cfg.GoogleClientID != "" {
		configured = append(configured, "google")
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":                   user.ID,
		"display_name":         user.DisplayName,
		"email":                user.Email,
		"avatar_url":           user.AvatarURL,
		"providers":            providers,
		"provider_usernames":   providerUsernames,
		"configured_providers": configured,
	})
}

// PATCH /api/admin/account/profile
func (h *AccountHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	claims := claimsFromReq(r)
	var req struct {
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DisplayName == "" {
		writeError(w, r, http.StatusBadRequest, "display_name is required")
		return
	}
	if err := store.UpdateUserDisplayName(claims.Sub, req.DisplayName); err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to update profile")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// PATCH /api/admin/account/password
func (h *AccountHandler) UpdatePassword(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	claims := claimsFromReq(r)
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request")
		return
	}
	if len(req.NewPassword) < 8 {
		writeError(w, r, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}
	identity, err := store.GetIdentityByUser(claims.Sub, "email")
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, r, http.StatusBadRequest, "no email/password login found for this account")
		} else {
			writeError(w, r, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(identity.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		writeError(w, r, http.StatusUnauthorized, "current password is incorrect")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to hash password")
		return
	}
	if err := store.UpdatePasswordHashByUser(claims.Sub, "email", string(hash)); err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to update password")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// DELETE /api/admin/account/identity/{provider}
func (h *AccountHandler) DisconnectIdentity(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	claims := claimsFromReq(r)
	provider := chi.URLParam(r, "provider")
	identities, err := store.ListUserIdentities(claims.Sub)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to get identities")
		return
	}
	if len(identities) <= 1 {
		writeError(w, r, http.StatusBadRequest, "cannot disconnect your only login method")
		return
	}
	if err := store.DeleteUserIdentity(claims.Sub, provider); err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to disconnect")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/admin/account/security
func (h *AccountHandler) GetSecurity(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	siteKey, secretKey, err := store.GetTurnstileKeys()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"turnstile_site_key":   "",
			"has_turnstile_secret": false,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"turnstile_site_key":   siteKey,
		"has_turnstile_secret": secretKey != "",
	})
}

// PATCH /api/admin/account/security
func (h *AccountHandler) UpdateSecurity(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	if h.cfg.CloudMode {
		sub, err := middleware.GetCachedSubscription(middleware.AccountIDFromRequest(r), store)
		if err != nil || middleware.PlanRank[sub.Plan] < middleware.PlanRank["starter"] {
			writeError(w, r, http.StatusPaymentRequired, "plan_upgrade_required")
			return
		}
	}
	var req struct {
		TurnstileSiteKey   string  `json:"turnstile_site_key"`
		TurnstileSecretKey *string `json:"turnstile_secret_key"` // nil = preserve existing
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request")
		return
	}
	secretKey := ""
	if req.TurnstileSecretKey == nil {
		_, existing, _ := store.GetTurnstileKeys()
		secretKey = existing
	} else {
		secretKey = *req.TurnstileSecretKey
	}
	if err := store.SetTurnstileKeys(req.TurnstileSiteKey, secretKey); err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to update security settings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
