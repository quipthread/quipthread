package session

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// IsHTTPS returns true when the request is over TLS directly, or when the
// configured base URL uses https (indicating a TLS-terminating proxy upstream).
func IsHTTPS(r *http.Request, baseURL string) bool {
	return r.TLS != nil || strings.HasPrefix(baseURL, "https://")
}

const (
	DashboardAudience = "dashboard"
	EmbedAudience     = "embed"

	DashboardCookieName = "quipthread_dashboard_session"
	LegacyCookieName    = "quipthread_session"

	// CookieName is retained as the dashboard name for source compatibility.
	// LegacyCookieName is never accepted for authorization.
	CookieName = DashboardCookieName
)

type contextKey string

const UserKey contextKey = "session_user"

type Claims struct {
	Sub          string `json:"sub"`
	DisplayName  string `json:"display_name"`
	Provider     string `json:"provider"`
	Role         string `json:"role"`
	AccountID    string `json:"account_id,omitempty"`
	IsTeamMember bool   `json:"is_team_member,omitempty"`
	// SiteID binds an embed token to the one site it was issued for. Embed
	// tokens are bearer credentials held by the publisher's page, so a token
	// issued on one site must never authenticate requests for another.
	SiteID            string `json:"site_id,omitempty"`
	SessionGeneration int64  `json:"session_generation"`
	jwt.RegisteredClaims
}

// IssueWithAudience issues a dashboard session. Embed tokens are site-bound
// and must be issued with IssueEmbed; asking for the embed audience here fails.
func IssueWithAudience(secret, audience, userID, displayName, provider, role, accountID string, generation int64) (string, error) {
	return issue(secret, audience, userID, displayName, provider, role, accountID, generation, false)
}

func IssueTeamMemberWithGeneration(secret, userID, displayName, provider, ownerAccountID string, generation int64) (string, error) {
	return issue(secret, DashboardAudience, userID, displayName, provider, "admin", ownerAccountID, generation, true)
}

// IssueEmbed issues a site-bound embed bearer token. accountID is the site
// owner's account in cloud mode and empty for self-hosted deployments.
func IssueEmbed(secret, userID, displayName, provider, role, accountID, siteID string, generation int64) (string, error) {
	if siteID == "" {
		return "", fmt.Errorf("embed token requires a site id")
	}
	return issueClaims(secret, EmbedAudience, Claims{
		Sub: userID, DisplayName: displayName, Provider: provider, Role: role,
		AccountID: accountID, SiteID: siteID, SessionGeneration: generation,
	})
}

func issue(secret, audience, userID, displayName, provider, role, accountID string, generation int64, teamMember bool) (string, error) {
	if audience == EmbedAudience {
		return "", fmt.Errorf("embed tokens must be issued with IssueEmbed")
	}
	return issueClaims(secret, audience, Claims{
		Sub: userID, DisplayName: displayName, Provider: provider, Role: role,
		AccountID: accountID, IsTeamMember: teamMember, SessionGeneration: generation,
	})
}

func issueClaims(secret, audience string, claims Claims) (string, error) {
	if audience != DashboardAudience && audience != EmbedAudience {
		return "", fmt.Errorf("unsupported session audience: %s", audience)
	}
	claims.RegisteredClaims = jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(30 * 24 * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		Audience:  jwt.ClaimStrings{audience},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// BearerToken returns the token from an "Authorization: Bearer" header, or "".
func BearerToken(r *http.Request) string {
	const prefix = "bearer "
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}

// ParseForAudience accepts only an HS256 token with exactly the requested
// audience. Other HMAC algorithms and multi-audience tokens are rejected.
func ParseForAudience(secret, tokenStr, audience string) (*Claims, error) {
	if audience != DashboardAudience && audience != EmbedAudience {
		return nil, fmt.Errorf("unsupported session audience: %s", audience)
	}
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithAudience(audience))
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	if len(claims.Audience) != 1 || claims.Audience[0] != audience {
		return nil, fmt.Errorf("invalid session audience")
	}
	return claims, nil
}

// SetCookie issues the dashboard session cookie. Embed sessions never use
// cookies: they are site-bound bearer tokens held by the publisher's page.
func SetCookie(w http.ResponseWriter, token string, secure bool) {
	setDashboardCookie(w, token, secure, 30*24*60*60)
	ClearLegacyCookie(w, secure)
}

func setDashboardCookie(w http.ResponseWriter, token string, secure bool, maxAge int) {
	// #nosec G124 -- Secure follows the deployment's HTTPS setting so local HTTP development works; the cookie is HttpOnly and Lax.
	http.SetCookie(w, &http.Cookie{
		Name: DashboardCookieName, Value: token, Path: "/", HttpOnly: true,
		Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge,
	})
}

func ClearDashboardCookie(w http.ResponseWriter, secure bool) {
	setDashboardCookie(w, "", secure, -1)
}

// ClearLegacyCookie implements the hard cutover: the old cookie is cleared,
// but it is never parsed or used for authorization.
func ClearLegacyCookie(w http.ResponseWriter, secure bool) {
	sameSite := http.SameSiteLaxMode
	if secure {
		sameSite = http.SameSiteNoneMode
	}
	// #nosec G124 -- Match the legacy cross-origin cookie scope on HTTPS, with Lax for local HTTP.
	http.SetCookie(w, &http.Cookie{Name: LegacyCookieName, Value: "", Path: "/", HttpOnly: true, Secure: secure, SameSite: sameSite, MaxAge: -1})
}

// SetIndicatorCookie sets a non-HttpOnly cookie that marketing pages can read
// to detect login state cross-origin. Contains no sensitive data.
func SetIndicatorCookie(w http.ResponseWriter, domain string, secure bool) {
	setIndicatorCookie(w, domain, "1", secure, 30*24*60*60)
}

func setIndicatorCookie(w http.ResponseWriter, domain, value string, secure bool, maxAge int) {
	sameSite := http.SameSiteLaxMode
	if secure {
		sameSite = http.SameSiteNoneMode
	}
	// #nosec G124 -- This non-sensitive login indicator must be readable by marketing-page JavaScript; HTTPS allows cross-origin access.
	http.SetCookie(w, &http.Cookie{Name: "qt_logged_in", Value: value, Path: "/", HttpOnly: false, Secure: secure, SameSite: sameSite, Domain: domain, MaxAge: maxAge})
}

func ClearIndicatorCookie(w http.ResponseWriter, domain string, secure bool) {
	setIndicatorCookie(w, domain, "", secure, -1)
}
