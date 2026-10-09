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
		Sub           string `json:"sub"`
		Name          string `json:"name"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Picture       string `json:"picture"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("decode google userinfo: %w", err)
	}

	return &UserInfo{
		ProviderID:    userInfo.Sub,
		Provider:      "google",
		Email:         userInfo.Email,
		EmailVerified: userInfo.EmailVerified,
		DisplayName:   userInfo.Name,
		AvatarURL:     userInfo.Picture,
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
	h.oauthCallback(w, r, h.google.ExchangeUser)
}
