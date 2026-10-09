package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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

	// The profile email is whatever the user made public; the email list says
	// which addresses GitHub has verified. Without the list, the email is
	// kept but treated as unverified.
	emailVerified := false
	if emails, err := fetchGithubEmails(ctx, token.AccessToken); err == nil {
		ghUser.Email, emailVerified = pickGithubEmail(ghUser.Email, emails)
	}

	displayName := ghUser.Name
	if displayName == "" {
		displayName = ghUser.Login
	}

	return &UserInfo{
		ProviderID:    fmt.Sprintf("%d", ghUser.ID),
		Provider:      "github",
		Email:         ghUser.Email,
		EmailVerified: emailVerified,
		DisplayName:   displayName,
		AvatarURL:     ghUser.AvatarURL,
		Username:      ghUser.Login,
	}, nil
}

type githubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

func fetchGithubEmails(ctx context.Context, accessToken string) ([]githubEmail, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user/emails", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+accessToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck // deferred close; body already drained by Decode
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github emails: status %d", resp.StatusCode)
	}
	var emails []githubEmail
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return nil, err
	}
	return emails, nil
}

// pickGithubEmail returns the profile email and whether GitHub verified it,
// or the primary verified email when the profile has none.
func pickGithubEmail(profileEmail string, emails []githubEmail) (string, bool) {
	if profileEmail != "" {
		for _, e := range emails {
			if e.Verified && strings.EqualFold(e.Email, profileEmail) {
				return profileEmail, true
			}
		}
		return profileEmail, false
	}
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, true
		}
	}
	return "", false
}

// --- HTTP handlers ----------------------------------------------------------

// GithubLink initiates an OAuth flow that links GitHub to the currently
// authenticated account rather than performing a login.
func (h *Handler) GithubLink(w http.ResponseWriter, r *http.Request) {
	session.ClearLegacyCookie(w, session.IsHTTPS(r, h.config.BaseURL))
	clearReturnToCookie(w, session.IsHTTPS(r, h.config.BaseURL))
	if h.github == nil {
		writeError(w, r, http.StatusNotFound, "GitHub auth not configured")
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
	http.Redirect(w, r, h.github.LoginURL(state), http.StatusFound)
}

func (h *Handler) GithubLogin(w http.ResponseWriter, r *http.Request) {
	session.ClearLegacyCookie(w, session.IsHTTPS(r, h.config.BaseURL))
	clearReturnToCookie(w, session.IsHTTPS(r, h.config.BaseURL))
	if h.github == nil {
		writeError(w, r, http.StatusNotFound, "GitHub auth not configured")
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
	http.Redirect(w, r, h.github.LoginURL(state), http.StatusFound)
}

func (h *Handler) GithubCallback(w http.ResponseWriter, r *http.Request) {
	session.ClearLegacyCookie(w, session.IsHTTPS(r, h.config.BaseURL))
	clearReturnToCookie(w, session.IsHTTPS(r, h.config.BaseURL))
	if h.github == nil {
		writeError(w, r, http.StatusNotFound, "GitHub auth not configured")
		return
	}
	h.oauthCallback(w, r, h.github.ExchangeUser)
}
