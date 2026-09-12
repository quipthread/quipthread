package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/session"
)

type GoogleProvider struct {
	oauth2Config *oauth2.Config
}

func NewGoogleProvider(cfg *config.Config) *GoogleProvider {
	return &GoogleProvider{
		oauth2Config: &oauth2.Config{
			ClientID:     cfg.GoogleClientID,
			ClientSecret: cfg.GoogleSecret,
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     google.Endpoint,
			RedirectURL:  cfg.BaseURL + "/auth/google/callback",
		},
	}
}

func (p *GoogleProvider) Name() string { return "google" }

func (p *GoogleProvider) LoginURL(state string) string {
	return p.oauth2Config.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

func (p *GoogleProvider) ExchangeUser(ctx context.Context, r *http.Request) (*UserInfo, error) {
	code := r.URL.Query().Get("code")
	if code == "" {
		return nil, fmt.Errorf("missing code parameter")
	}

	token, err := p.oauth2Config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}

	client := p.oauth2Config.Client(ctx, token)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://openidconnect.googleapis.com/v1/userinfo", nil)
	if err != nil {
		return nil, fmt.Errorf("create google userinfo request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch google userinfo: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // deferred close; body already drained by Decode

	var userInfo struct {
		Sub     string `json:"sub"`
		Name    string `json:"name"`
		Email   string `json:"email"`
		Picture string `json:"picture"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("decode google userinfo: %w", err)
	}

	return &UserInfo{
		ProviderID:  userInfo.Sub,
		Provider:    "google",
		Email:       userInfo.Email,
		DisplayName: userInfo.Name,
		AvatarURL:   userInfo.Picture,
	}, nil
}

// --- HTTP handlers ----------------------------------------------------------

// GoogleLink initiates an OAuth flow that links Google to the currently
// authenticated account rather than performing a login.
func (h *Handler) GoogleLink(w http.ResponseWriter, r *http.Request) {
	session.ClearLegacyCookie(w, session.IsHTTPS(r, h.config.BaseURL))
	clearReturnToCookie(w, session.IsHTTPS(r, h.config.BaseURL))
	if h.google == nil {
		writeError(w, r, http.StatusNotFound, "Google auth not configured")
		return
	}

	if !h.validateDashboardUser(r, "") {
		http.Redirect(w, r, h.config.BaseURL+"/login", http.StatusFound)
		return
	}
	cookie, _ := r.Cookie(session.DashboardCookieName)
	claims, _ := session.ParseForAudience(h.config.JWTSecret, cookie.Value, session.DashboardAudience)
	if claims == nil {
		http.Redirect(w, r, h.config.BaseURL+"/login", http.StatusFound)
		return
	}

	state, err := generateState()
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to generate state")
		return
	}

	setLinkIntentCookie(w, claims.Sub, session.IsHTTPS(r, h.config.BaseURL))
	setStateCookie(w, oauthStateBinding{State: state, Audience: session.DashboardAudience, Redirect: h.config.BaseURL}, session.IsHTTPS(r, h.config.BaseURL))
	http.Redirect(w, r, h.google.LoginURL(state), http.StatusFound)
}

func (h *Handler) GoogleLogin(w http.ResponseWriter, r *http.Request) {
	session.ClearLegacyCookie(w, session.IsHTTPS(r, h.config.BaseURL))
	clearReturnToCookie(w, session.IsHTTPS(r, h.config.BaseURL))
	if h.google == nil {
		writeError(w, r, http.StatusNotFound, "Google auth not configured")
		return
	}
	binding, err := h.resolveOAuthFlow(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	state, err := generateState()
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to generate state")
		return
	}

	binding.State = state
	setStateCookie(w, binding, session.IsHTTPS(r, h.config.BaseURL))
	http.Redirect(w, r, h.google.LoginURL(state), http.StatusFound)
}

func (h *Handler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	session.ClearLegacyCookie(w, session.IsHTTPS(r, h.config.BaseURL))
	clearReturnToCookie(w, session.IsHTTPS(r, h.config.BaseURL))
	if h.google == nil {
		writeError(w, r, http.StatusNotFound, "Google auth not configured")
		return
	}

	state := r.URL.Query().Get("state")
	if !validateStateCookie(r, state) {
		clearStateCookie(w, session.IsHTTPS(r, h.config.BaseURL))
		http.Redirect(w, r, h.config.BaseURL+"/login?error=session_expired", http.StatusSeeOther)
		return
	}
	binding, bound := consumeStateBinding(w, r, state, session.IsHTTPS(r, h.config.BaseURL))
	clearStateCookie(w, session.IsHTTPS(r, h.config.BaseURL))
	if !bound {
		http.Redirect(w, r, h.config.BaseURL+"/login?error=session_expired", http.StatusSeeOther)
		return
	}
	audience := binding.Audience
	if !h.validRedirectForAudience(binding.Redirect, audience) {
		if audience == session.EmbedAudience {
			writeError(w, r, http.StatusBadRequest, "invalid OAuth redirect")
			return
		}
		binding.Redirect = h.config.BaseURL
	}

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		http.Redirect(w, r, h.config.BaseURL+"/login?error=oauth_denied", http.StatusSeeOther)
		return
	}

	info, err := h.google.ExchangeUser(r.Context(), r)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	if accountID := consumeLinkIntentCookie(w, r, session.IsHTTPS(r, h.config.BaseURL)); accountID != "" {
		if !h.validateDashboardUser(r, accountID) {
			http.Redirect(w, r, h.config.BaseURL+"/login?error=session_expired", http.StatusSeeOther)
			return
		}
		h.handleLinkCallback(w, r, info, accountID)
		return
	}

	if h.config.CloudMode && h.cloudUpsertAndIssueToken(w, r, info, audience, binding.Redirect) {
		return
	}

	tokenStr, err := h.upsertAndIssueToken(info, audience)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	session.SetCookieForAudience(w, tokenStr, audience, session.IsHTTPS(r, h.config.BaseURL))

	if !h.validRedirectForAudience(binding.Redirect, audience) {
		binding.Redirect = h.config.BaseURL
	}
	http.Redirect(w, r, binding.Redirect, http.StatusFound)
}
