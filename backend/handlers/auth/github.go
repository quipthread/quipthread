package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/session"
)

type GithubProvider struct {
	oauth2Config *oauth2.Config
}

func NewGithubProvider(cfg *config.Config) *GithubProvider {
	return &GithubProvider{
		oauth2Config: &oauth2.Config{
			ClientID:     cfg.GitHubClientID,
			ClientSecret: cfg.GitHubSecret,
			Scopes:       []string{"read:user", "user:email"},
			Endpoint:     github.Endpoint,
			RedirectURL:  cfg.BaseURL + "/auth/github/callback",
		},
	}
}

func (p *GithubProvider) Name() string { return "github" }

func (p *GithubProvider) LoginURL(state string) string {
	return p.oauth2Config.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

func (p *GithubProvider) ExchangeUser(ctx context.Context, r *http.Request) (*UserInfo, error) {
	code := r.URL.Query().Get("code")
	if code == "" {
		return nil, fmt.Errorf("missing code parameter")
	}

	token, err := p.oauth2Config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}

	client := p.oauth2Config.Client(ctx, token)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return nil, fmt.Errorf("create github user request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch github user: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // deferred close; body already drained by Decode

	var ghUser struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ghUser); err != nil {
		return nil, fmt.Errorf("decode github user: %w", err)
	}

	if ghUser.Email == "" {
		if email, err := fetchGithubPrimaryEmail(ctx, token.AccessToken); err == nil {
			ghUser.Email = email
		}
	}

	displayName := ghUser.Name
	if displayName == "" {
		displayName = ghUser.Login
	}

	return &UserInfo{
		ProviderID:  fmt.Sprintf("%d", ghUser.ID),
		Provider:    "github",
		Email:       ghUser.Email,
		DisplayName: displayName,
		AvatarURL:   ghUser.AvatarURL,
		Username:    ghUser.Login,
	}, nil
}

func fetchGithubPrimaryEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user/emails", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "token "+accessToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() //nolint:errcheck // deferred close; body already drained by Decode
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", err
	}
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}
	return "", fmt.Errorf("no primary verified email")
}

// --- HTTP handlers ----------------------------------------------------------

// GithubLink initiates an OAuth flow that links GitHub to the currently
// authenticated account rather than performing a login.
func (h *Handler) GithubLink(w http.ResponseWriter, r *http.Request) {
	if h.github == nil {
		writeError(w, http.StatusNotFound, "GitHub auth not configured")
		return
	}

	cookie, err := r.Cookie(session.CookieName)
	if err != nil {
		http.Redirect(w, r, h.config.BaseURL+"/login", http.StatusFound)
		return
	}
	claims, err := session.Parse(h.config.JWTSecret, cookie.Value)
	if err != nil || claims == nil {
		http.Redirect(w, r, h.config.BaseURL+"/login", http.StatusFound)
		return
	}

	state, err := generateState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate state")
		return
	}

	setLinkIntentCookie(w, claims.Sub)
	setStateCookie(w, state)
	http.Redirect(w, r, h.github.LoginURL(state), http.StatusFound)
}

func (h *Handler) GithubLogin(w http.ResponseWriter, r *http.Request) {
	if h.github == nil {
		writeError(w, http.StatusNotFound, "GitHub auth not configured")
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
	http.Redirect(w, r, h.github.LoginURL(state), http.StatusFound)
}

func (h *Handler) GithubCallback(w http.ResponseWriter, r *http.Request) {
	if h.github == nil {
		writeError(w, http.StatusNotFound, "GitHub auth not configured")
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

	info, err := h.github.ExchangeUser(r.Context(), r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if accountID := consumeLinkIntentCookie(w, r); accountID != "" {
		h.handleLinkCallback(w, r, info, accountID)
		return
	}

	if h.config.CloudMode {
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
