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
	resp, err := client.Get("https://openidconnect.googleapis.com/v1/userinfo")
	if err != nil {
		return nil, fmt.Errorf("fetch google userinfo: %w", err)
	}
	defer resp.Body.Close()

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

func (h *Handler) GoogleLogin(w http.ResponseWriter, r *http.Request) {
	if h.google == nil {
		writeError(w, http.StatusNotFound, "Google auth not configured")
		return
	}

	state, err := generateState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate state")
		return
	}

	if returnTo := r.URL.Query().Get("returnTo"); returnTo != "" && h.validateReturnTo(returnTo) {
		setReturnToCookie(w, returnTo)
	}

	setStateCookie(w, state)
	http.Redirect(w, r, h.google.LoginURL(state), http.StatusFound)
}

func (h *Handler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	if h.google == nil {
		writeError(w, http.StatusNotFound, "Google auth not configured")
		return
	}

	state := r.URL.Query().Get("state")
	if !validateStateCookie(r, state) {
		writeError(w, http.StatusBadRequest, "invalid state")
		return
	}
	clearStateCookie(w)

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		writeError(w, http.StatusBadRequest, errParam)
		return
	}

	info, err := h.google.ExchangeUser(r.Context(), r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if h.cloudStore != nil && h.config.CloudMode {
		h.cloudUpsertAndIssueToken(w, r, info)
		return
	}

	tokenStr, err := h.upsertAndIssueToken(info)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	session.SetCookie(w, tokenStr, r.TLS != nil)

	returnTo := consumeReturnToCookie(w, r)
	if returnTo == "" || !h.validateReturnTo(returnTo) {
		returnTo = h.config.BaseURL
	}
	http.Redirect(w, r, returnTo, http.StatusFound)
}
