package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/url"

	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/middleware"
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
	ProviderID string
	Provider   string
	Email      string
	// EmailVerified reports that the provider vouches the user owns Email.
	// Only verified emails may match or create cloud accounts.
	EmailVerified bool
	DisplayName   string
	AvatarURL     string
	Username      string // human-readable handle; populated by providers that expose one (e.g. GitHub login)
}

// Handler holds shared dependencies for all auth sub-handlers.
type Handler struct {
	store       db.Store
	config      *config.Config
	cloudExtras //nolint:unused // populated by provider_cloud.go in cloud builds
	embedSites  *middleware.EmbedSites
	github      *GithubProvider
	google      *GoogleProvider
	email       *EmailProvider
}

// SetEmbedSites wires the resolver used by embed sign-in flows (OAuth and
// SSO) to find a site's owning store.
func (h *Handler) SetEmbedSites(sites *middleware.EmbedSites) {
	h.embedSites = sites
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
		// Embed tokens are only accepted through the site-binding auth
		// middleware, which places verified claims in the context.
		if audience == session.EmbedAudience {
			writeError(w, r, http.StatusUnauthorized, "not authenticated")
			return
		}
		cookie, err := r.Cookie(session.DashboardCookieName)
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
		"id":             claims.Sub,
		"display_name":   claims.DisplayName,
		"provider":       claims.Provider,
		"role":           claims.Role,
		"is_team_member": claims.IsTeamMember,
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

func (h *Handler) DashboardLogout(w http.ResponseWriter, r *http.Request) {
	session.ClearDashboardCookie(w, session.IsHTTPS(r, h.config.BaseURL))
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
	SiteID   string `json:"site_id,omitempty"`
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

func (h *Handler) resolveOAuthFlow(r *http.Request) (oauthStateBinding, error) {
	q := r.URL.Query()
	switch q.Get("audience") {
	case "", session.DashboardAudience:
		// Dashboard redirects are always constrained to the configured base URL.
		return oauthStateBinding{Audience: session.DashboardAudience, Redirect: h.config.BaseURL}, nil
	case session.EmbedAudience:
		site, returnTo, err := h.resolveEmbedReturn(q.Get("siteId"), q.Get("returnTo"))
		if err != nil {
			return oauthStateBinding{}, err
		}
		site.Release()
		return oauthStateBinding{Audience: session.EmbedAudience, Redirect: returnTo, SiteID: site.Site.ID}, nil
	default:
		return oauthStateBinding{}, fmt.Errorf("invalid OAuth audience")
	}
}

// resolveEmbedReturn resolves the embed site and validates that returnTo is
// a page on that site's own origin. It returns returnTo without any fragment,
// since the fragment carries the OAuth result back to the embed. The caller
// must Release the returned site.
func (h *Handler) resolveEmbedReturn(siteID, returnTo string) (*middleware.EmbedSite, string, error) {
	if h.embedSites == nil {
		return nil, "", fmt.Errorf("embed sign-in is not available")
	}
	if siteID == "" || returnTo == "" {
		return nil, "", fmt.Errorf("embed sign-in requires siteId and returnTo")
	}
	u, err := url.Parse(returnTo)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil, "", fmt.Errorf("invalid returnTo")
	}
	u.Fragment = ""
	u.RawFragment = ""
	site, err := h.embedSites.Resolve(siteID)
	if err != nil {
		return nil, "", fmt.Errorf("unknown site")
	}
	if !h.embedSites.OriginAllowed(site.Site, u.Scheme+"://"+u.Host) {
		site.Release()
		return nil, "", fmt.Errorf("returnTo is not on this site's domain")
	}
	return site, u.String(), nil
}

func clearReturnToCookie(w http.ResponseWriter, secure bool) {
	setOAuthCookie(w, returnToCookieName, "", secure, -1)
}

// --- OAuth callback -----------------------------------------------------------

// oauthCallback completes a GitHub or Google login. Embed logins return a
// site-bound token to the publisher page; dashboard logins set the session
// cookie (or link an identity) and redirect into the dashboard.
func (h *Handler) oauthCallback(w http.ResponseWriter, r *http.Request, exchange func(context.Context, *http.Request) (*UserInfo, error)) {
	secure := session.IsHTTPS(r, h.config.BaseURL)
	state := r.URL.Query().Get("state")
	if !validateStateCookie(r, state) {
		clearStateCookie(w, secure)
		http.Redirect(w, r, h.config.BaseURL+"/login?error=session_expired", http.StatusSeeOther)
		return
	}
	binding, bound := consumeStateBinding(w, r, state, secure)
	clearStateCookie(w, secure)
	if !bound {
		http.Redirect(w, r, h.config.BaseURL+"/login?error=session_expired", http.StatusSeeOther)
		return
	}

	if binding.Audience == session.EmbedAudience {
		h.embedOAuthCallback(w, r, binding, exchange)
		return
	}

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		http.Redirect(w, r, h.config.BaseURL+"/login?error=oauth_denied", http.StatusSeeOther)
		return
	}

	info, err := exchange(r.Context(), r)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	if accountID := consumeLinkIntentCookie(w, r, secure); accountID != "" {
		if !h.validateDashboardUser(r, accountID) {
			http.Redirect(w, r, h.config.BaseURL+"/login?error=session_expired", http.StatusSeeOther)
			return
		}
		h.handleLinkCallback(w, r, info, accountID)
		return
	}

	if h.config.CloudMode && h.cloudUpsertAndIssueToken(w, r, info) {
		return
	}

	tokenStr, err := h.upsertAndIssueToken(info)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	session.SetCookie(w, tokenStr, secure)
	http.Redirect(w, r, h.config.BaseURL, http.StatusFound)
}

// embedOAuthCallback signs a reader in to one publisher site. The reader is
// stored in the site's own tenant store (never as a Quipthread account), and
// the site-bound token is returned in the URL fragment of the validated
// publisher page, where the embed stores it and removes it from the URL.
// Fragments are never sent to servers.
func (h *Handler) embedOAuthCallback(w http.ResponseWriter, r *http.Request, binding oauthStateBinding, exchange func(context.Context, *http.Request) (*UserInfo, error)) {
	site, redirect, err := h.resolveEmbedReturn(binding.SiteID, binding.Redirect)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid OAuth redirect")
		return
	}
	defer site.Release()

	w.Header().Set("Cache-Control", "no-store")
	fail := func(code string) {
		http.Redirect(w, r, redirect+"#qt_auth_error="+code, http.StatusFound)
	}

	if r.URL.Query().Get("error") != "" {
		fail("denied")
		return
	}
	info, err := exchange(r.Context(), r)
	if err != nil {
		log.Printf("auth: embed oauth exchange for site %s: %v", binding.SiteID, err) //nolint:gosec // G706: site ID is validated against the registry
		fail("provider_error")
		return
	}
	// Readers never become admins through the embed; existing users keep
	// their role, so site owners can still moderate from their own pages.
	user, err := upsertIdentityUser(site.Store, info, "commenter")
	if err != nil {
		if errors.Is(err, errAccountBanned) {
			fail("banned")
			return
		}
		log.Printf("auth: embed oauth upsert for site %s: %v", binding.SiteID, err) //nolint:gosec // G706: site ID is validated against the registry
		fail("server_error")
		return
	}
	tok, err := session.IssueEmbed(h.config.JWTSecret, user.ID, user.DisplayName, info.Provider, user.Role, site.AccountID, binding.SiteID, user.EmbedSessionGeneration)
	if err != nil {
		fail("server_error")
		return
	}
	http.Redirect(w, r, redirect+"#qt_auth="+url.QueryEscape(tok), http.StatusFound)
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

// upsertAndIssueToken finds or creates the user for the given provider identity
// in the deployment store, then issues a dashboard session JWT.
func (h *Handler) upsertAndIssueToken(info *UserInfo) (string, error) {
	// First user to register becomes admin automatically.
	role := "commenter"
	if _, total, err := h.store.ListUsers(1, 1); err == nil && total == 0 {
		role = "admin"
	}
	user, err := upsertIdentityUser(h.store, info, role)
	if err != nil {
		return "", err
	}
	return session.IssueWithAudience(h.config.JWTSecret, session.DashboardAudience, user.ID, user.DisplayName, info.Provider, user.Role, "", user.DashboardSessionGeneration)
}

var errAccountBanned = errors.New("account is banned")

// upsertIdentityUser finds the user linked to the provider identity in store,
// refreshing their profile, or creates one with newUserRole. Banned users are
// rejected.
func upsertIdentityUser(store db.Store, info *UserInfo, newUserRole string) (*models.User, error) {
	identity, err := store.GetIdentity(info.Provider, info.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("get identity: %w", err)
	}

	var userID string
	if identity == nil {
		u := &models.User{
			DisplayName: info.DisplayName,
			Email:       info.Email,
			AvatarURL:   info.AvatarURL,
			Role:        newUserRole,
		}
		if err := store.UpsertUser(u); err != nil {
			return nil, fmt.Errorf("upsert user: %w", err)
		}
		ident := &models.UserIdentity{
			UserID:     u.ID,
			Provider:   info.Provider,
			ProviderID: info.ProviderID,
			Username:   info.Username,
		}
		if err := store.CreateIdentity(ident); err != nil {
			return nil, fmt.Errorf("create identity: %w", err)
		}
		userID = u.ID
	} else {
		userID = identity.UserID
		u, err := store.GetUser(userID)
		if err != nil {
			return nil, fmt.Errorf("get user: %w", err)
		}
		if u != nil {
			u.DisplayName = info.DisplayName
			u.AvatarURL = info.AvatarURL
			if info.Email != "" {
				u.Email = info.Email
			}
			if err := store.UpsertUser(u); err != nil {
				log.Printf("auth: upsert user profile on login %s: %v", userID, err) //nolint:gosec // G706: userID is an internal UUID, not user-controlled format string
			}
		}
		if info.Username != "" {
			if err := store.UpdateIdentityUsername(userID, info.Provider, info.Username); err != nil {
				log.Printf("auth: update identity username %s: %v", userID, err) //nolint:gosec // G706: userID is an internal UUID, not user-controlled format string
			}
		}
	}

	user, err := store.GetUser(userID)
	if err != nil || user == nil {
		return nil, fmt.Errorf("could not load user after upsert")
	}
	if user.Banned {
		return nil, errAccountBanned
	}
	return user, nil
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
