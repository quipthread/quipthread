package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/models"
	"github.com/quipthread/quipthread/session"
)

// Provider abstracts OAuth and OAuth-like login flows.
type Provider interface {
	Name() string
	LoginURL(state string) string
	ExchangeUser(ctx context.Context, r *http.Request) (*UserInfo, error)
}

// UserInfo is the normalised identity returned by every provider.
type UserInfo struct {
	ProviderID  string
	Provider    string
	Email       string
	DisplayName string
	AvatarURL   string
	Username    string // human-readable handle; populated by providers that expose one (e.g. GitHub login)
}

// Handler holds shared dependencies for all auth sub-handlers.
type Handler struct {
	store       db.Store
	config      *config.Config
	cloudExtras //nolint:unused // populated by provider_cloud.go in cloud builds
	github      *GithubProvider
	google      *GoogleProvider
	email       *EmailProvider
}

// newHandler constructs the base Handler without any cloud extras set.
func newHandler(store db.Store, cfg *config.Config) *Handler {
	h := &Handler{
		store:  store,
		config: cfg,
	}

	if cfg.GitHubClientID != "" && cfg.GitHubSecret != "" {
		h.github = NewGithubProvider(cfg)
	}
	if cfg.GoogleClientID != "" && cfg.GoogleSecret != "" {
		h.google = NewGoogleProvider(cfg)
	}
	if cfg.EmailAuthEnabled {
		h.email = NewEmailProvider(store, cfg)
	}

	return h
}

// Me is the dashboard compatibility alias.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	h.MeForAudience(w, r, session.DashboardAudience)
}

func (h *Handler) DashboardMe(w http.ResponseWriter, r *http.Request) {
	h.MeForAudience(w, r, session.DashboardAudience)
}

func (h *Handler) EmbedMe(w http.ResponseWriter, r *http.Request) {
	h.MeForAudience(w, r, session.EmbedAudience)
}

func (h *Handler) MeForAudience(w http.ResponseWriter, r *http.Request, audience string) {
	session.ClearLegacyCookie(w, session.IsHTTPS(r, h.config.BaseURL))
	claims, _ := r.Context().Value(session.UserKey).(*session.Claims)
	if claims == nil {
		cookie, err := r.Cookie(session.CookieNameForAudience(audience))
		if err != nil {
			writeError(w, r, http.StatusUnauthorized, "not authenticated")
			return
		}
		var parseErr error
		claims, parseErr = session.ParseForAudience(h.config.JWTSecret, cookie.Value, audience)
		if parseErr != nil {
			writeError(w, r, http.StatusUnauthorized, "invalid session")
			return
		}
	}
	store := h.store
	if contextual, ok := db.StoreFromContext(r.Context()); ok {
		store = contextual
	}
	user, err := store.GetUser(claims.Sub)
	if err != nil || user == nil || user.Banned || audienceGeneration(user, audience) != claims.SessionGeneration {
		writeError(w, r, http.StatusUnauthorized, "invalid or revoked session")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":           claims.Sub,
		"display_name": claims.DisplayName,
		"provider":     claims.Provider,
		"role":         claims.Role,
	})
}

// validateDashboardUser validates the dashboard cookie against the current
// tenant user record. OAuth link flows must not mutate identities for a
// banned or generation-revoked dashboard session.
func (h *Handler) validateDashboardUser(r *http.Request, expectedUserID string) bool {
	cookie, err := r.Cookie(session.DashboardCookieName)
	if err != nil {
		return false
	}
	claims, err := session.ParseForAudience(h.config.JWTSecret, cookie.Value, session.DashboardAudience)
	if err != nil || claims == nil || expectedUserID != "" && claims.Sub != expectedUserID {
		return false
	}
	if h.config.CloudMode {
		return h.validateCloudDashboardUser(r.Context(), claims)
	}
	store := h.store
	if contextual, ok := db.StoreFromContext(r.Context()); ok {
		store = contextual
	}
	user, err := store.GetUser(claims.Sub)
	return err == nil && user != nil && !user.Banned && audienceGeneration(user, session.DashboardAudience) == claims.SessionGeneration
}

// Logout is the dashboard compatibility alias.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	h.DashboardLogout(w, r)
}

func (h *Handler) DashboardLogout(w http.ResponseWriter, r *http.Request) {
	session.ClearCookieForAudience(w, session.DashboardAudience, session.IsHTTPS(r, h.config.BaseURL))
	session.ClearLegacyCookie(w, session.IsHTTPS(r, h.config.BaseURL))
	session.ClearIndicatorCookie(w, h.config.CookieDomain, session.IsHTTPS(r, h.config.BaseURL))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "logged out"}) //nolint:errcheck,gosec // error response; connection may already be broken
}

func (h *Handler) EmbedLogout(w http.ResponseWriter, r *http.Request) {
	session.ClearCookieForAudience(w, session.EmbedAudience, session.IsHTTPS(r, h.config.BaseURL))
	session.ClearLegacyCookie(w, session.IsHTTPS(r, h.config.BaseURL))
	session.ClearIndicatorCookie(w, h.config.CookieDomain, session.IsHTTPS(r, h.config.BaseURL))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "logged out"}) //nolint:errcheck,gosec // error response; connection may already be broken
}

// --- state cookie helpers ---------------------------------------------------

const stateCookieName = "quipthread_oauth_state"
const stateAudienceCookieName = "quipthread_oauth_audience"

func generateState() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

type oauthStateBinding struct {
	State    string `json:"state"`
	Audience string `json:"audience"`
	Redirect string `json:"redirect"`
}

func setOAuthCookie(w http.ResponseWriter, name, value string, secure bool, maxAge int) {
	// #nosec G124 -- OAuth cookies use Lax for provider redirects; Secure follows the deployment HTTPS setting.
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}

func setStateCookie(w http.ResponseWriter, binding oauthStateBinding, secure bool) {
	setOAuthCookie(w, stateCookieName, binding.State, secure, 600)
	data, _ := json.Marshal(binding)
	setOAuthCookie(w, stateAudienceCookieName, base64.RawURLEncoding.EncodeToString(data), secure, 600)
}

func validateStateCookie(r *http.Request, state string) bool {
	cookie, err := r.Cookie(stateCookieName)
	if err != nil {
		return false
	}
	return state != "" && cookie.Value == state
}

func clearStateCookie(w http.ResponseWriter, secure bool) {
	setOAuthCookie(w, stateCookieName, "", secure, -1)
	setOAuthCookie(w, stateAudienceCookieName, "", secure, -1)
}

func consumeStateBinding(w http.ResponseWriter, r *http.Request, state string, secure bool) (oauthStateBinding, bool) {
	var binding oauthStateBinding
	cookie, err := r.Cookie(stateAudienceCookieName)
	setOAuthCookie(w, stateAudienceCookieName, "", secure, -1)
	if err != nil {
		return binding, false
	}
	data, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil || json.Unmarshal(data, &binding) != nil || binding.State != state {
		return oauthStateBinding{}, false
	}
	if binding.Audience != session.DashboardAudience && binding.Audience != session.EmbedAudience {
		return oauthStateBinding{}, false
	}
	return binding, true
}

// --- returnTo cookie helpers ------------------------------------------------

const returnToCookieName = "quipthread_return_to"

// validateReturnTo returns true if returnTo is a safe http/https URL whose
// origin matches either the configured BASE_URL or one of AllowedOrigins.
// When AllowedOrigins is empty, any http/https URL is accepted.
func (h *Handler) validateReturnTo(returnTo string) bool {
	u, err := url.Parse(returnTo)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	origin := u.Scheme + "://" + u.Host

	// Always allow returnTo on the same origin as BASE_URL (covers /admin/*).
	if base, err := url.Parse(h.config.BaseURL); err == nil {
		baseOrigin := base.Scheme + "://" + base.Host
		if origin == baseOrigin {
			return true
		}
	}

	if len(h.config.AllowedOrigins) == 0 {
		return true
	}
	for _, allowed := range h.config.AllowedOrigins {
		if strings.TrimRight(allowed, "/") == origin {
			return true
		}
	}
	return false
}

func requestedOAuthAudience(r *http.Request) string {
	audience := r.URL.Query().Get("audience")
	if audience == "" {
		return ""
	}
	if audience == session.DashboardAudience || audience == session.EmbedAudience {
		return audience
	}
	return session.DashboardAudience
}

func (h *Handler) resolveOAuthFlow(r *http.Request) (oauthStateBinding, error) {
	requested := r.URL.Query().Get("audience")
	audience := requestedOAuthAudience(r)
	explicit := requested != ""
	if explicit && audience != session.DashboardAudience && audience != session.EmbedAudience {
		return oauthStateBinding{}, fmt.Errorf("invalid OAuth audience")
	}

	returnTo := r.URL.Query().Get("returnTo")
	validReturnTo := returnTo != "" && h.validateReturnTo(returnTo)
	external := validReturnTo && !h.isBaseOrigin(returnTo)
	if !explicit {
		if external {
			audience = session.EmbedAudience
		} else {
			audience = session.DashboardAudience
		}
	}

	redirect := h.config.BaseURL
	if audience == session.EmbedAudience {
		if !external {
			return oauthStateBinding{}, fmt.Errorf("embed OAuth requires an allowed publisher returnTo")
		}
		redirect = returnTo
	}
	// Dashboard redirects are always constrained to the configured base URL,
	// even when a caller supplied an allowed external returnTo.
	return oauthStateBinding{Audience: audience, Redirect: redirect}, nil
}

func (h *Handler) isBaseOrigin(raw string) bool {
	requested, err := url.Parse(raw)
	base, baseErr := url.Parse(h.config.BaseURL)
	return err == nil && baseErr == nil && requested.Scheme == base.Scheme && requested.Host == base.Host
}

func clearReturnToCookie(w http.ResponseWriter, secure bool) {
	setOAuthCookie(w, returnToCookieName, "", secure, -1)
}

func (h *Handler) validRedirectForAudience(redirect, audience string) bool {
	if audience == session.DashboardAudience {
		return h.isBaseOrigin(redirect)
	}
	return audience == session.EmbedAudience && h.validateReturnTo(redirect) && !h.isBaseOrigin(redirect)
}

// --- link intent cookie helpers ---------------------------------------------

const linkIntentCookieName = "quipthread_link_intent"

func setLinkIntentCookie(w http.ResponseWriter, accountID string, secure bool) {
	setOAuthCookie(w, linkIntentCookieName, accountID, secure, 600)
}

func consumeLinkIntentCookie(w http.ResponseWriter, r *http.Request, secure bool) string {
	cookie, err := r.Cookie(linkIntentCookieName)
	if err != nil {
		return ""
	}
	setOAuthCookie(w, linkIntentCookieName, "", secure, -1)
	return cookie.Value
}

// handleLinkCallback links an OAuth identity to an existing authenticated account
// instead of performing a login. accountID is the Sub claim from the current session.
func (h *Handler) handleLinkCallback(w http.ResponseWriter, r *http.Request, info *UserInfo, accountID string) {
	if h.cloudHandleLinkCallback(w, r, info, accountID) {
		return
	}

	accountPage := h.config.BaseURL + "/dashboard/account"

	// Self-hosted: link the OAuth identity directly to the user in the tenant store.
	existing, err := h.store.GetIdentity(info.Provider, info.ProviderID)
	if err != nil {
		http.Redirect(w, r, accountPage+"?link_error=server_error", http.StatusFound)
		return
	}
	if existing != nil && existing.UserID != accountID {
		http.Redirect(w, r, accountPage+"?link_error=already_linked", http.StatusFound)
		return
	}
	if existing == nil {
		if err := h.store.CreateIdentity(&models.UserIdentity{
			UserID:     accountID,
			Provider:   info.Provider,
			ProviderID: info.ProviderID,
			Username:   info.Username,
		}); err != nil {
			log.Printf("handleLinkCallback: create identity for user %s: %v", accountID, err) //nolint:gosec // G706: accountID is internal UUID
			http.Redirect(w, r, accountPage+"?link_error=server_error", http.StatusFound)
			return
		}
	}
	http.Redirect(w, r, accountPage, http.StatusFound)
}

// --- shared upsert + JWT issue ----------------------------------------------

// upsertAndIssueToken finds or creates the user for the given provider identity,
// then issues a session JWT. Returns the signed token string.
func (h *Handler) upsertAndIssueToken(info *UserInfo, audiences ...string) (string, error) {
	audience := session.DashboardAudience
	if len(audiences) > 0 && audiences[0] != "" {
		audience = audiences[0]
	}
	identity, err := h.store.GetIdentity(info.Provider, info.ProviderID)
	if err != nil {
		return "", fmt.Errorf("get identity: %w", err)
	}

	var userID string

	if identity == nil {
		// First user to register becomes admin automatically.
		role := "commenter"
		if _, total, err := h.store.ListUsers(1, 1); err == nil && total == 0 {
			role = "admin"
		}

		u := &models.User{
			DisplayName: info.DisplayName,
			Email:       info.Email,
			AvatarURL:   info.AvatarURL,
			Role:        role,
		}
		if err := h.store.UpsertUser(u); err != nil {
			return "", fmt.Errorf("upsert user: %w", err)
		}

		ident := &models.UserIdentity{
			UserID:     u.ID,
			Provider:   info.Provider,
			ProviderID: info.ProviderID,
			Username:   info.Username,
		}
		if err := h.store.CreateIdentity(ident); err != nil {
			return "", fmt.Errorf("create identity: %w", err)
		}
		userID = u.ID
	} else {
		userID = identity.UserID
		u, err := h.store.GetUser(userID)
		if err != nil {
			return "", fmt.Errorf("get user: %w", err)
		}
		if u != nil {
			u.DisplayName = info.DisplayName
			u.AvatarURL = info.AvatarURL
			if info.Email != "" {
				u.Email = info.Email
			}
			if err := h.store.UpsertUser(u); err != nil {
				log.Printf("auth: upsert user profile on login %s: %v", userID, err) //nolint:gosec // G706: userID is an internal UUID, not user-controlled format string
			}
		}
		if info.Username != "" {
			if err := h.store.UpdateIdentityUsername(userID, info.Provider, info.Username); err != nil {
				log.Printf("auth: update identity username %s: %v", userID, err) //nolint:gosec // G706: userID is an internal UUID, not user-controlled format string
			}
		}
	}

	user, err := h.store.GetUser(userID)
	if err != nil || user == nil {
		return "", fmt.Errorf("could not load user after upsert")
	}
	if user.Banned {
		return "", fmt.Errorf("account is banned")
	}

	return session.IssueWithAudience(h.config.JWTSecret, audience, user.ID, user.DisplayName, info.Provider, user.Role, "", audienceGeneration(user, audience))
}

func audienceGeneration(user *models.User, audience string) int64 {
	if audience == session.EmbedAudience {
		return user.EmbedSessionGeneration
	}
	return user.DashboardSessionGeneration
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck,gosec // error response; connection may already be broken
}

func writeError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	reqID := chimiddleware.GetReqID(r.Context())
	if status >= 500 {
		slog.ErrorContext(r.Context(), "request error", "request_id", reqID, "status", status, "error", msg, "path", r.URL.Path)
	}
	writeJSON(w, status, map[string]string{"error": msg, "request_id": reqID})
}
