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
	EmbedCookieName     = "quipthread_embed_session"
	LegacyCookieName    = "quipthread_session"

	// CookieName is retained as the dashboard name for source compatibility.
	// LegacyCookieName is never accepted for authorization.
	CookieName = DashboardCookieName
)

type contextKey string

const UserKey contextKey = "session_user"

type Claims struct {
	Sub               string `json:"sub"`
	DisplayName       string `json:"display_name"`
	Provider          string `json:"provider"`
	Role              string `json:"role"`
	AccountID         string `json:"account_id,omitempty"`
	IsTeamMember      bool   `json:"is_team_member,omitempty"`
	SessionGeneration int64  `json:"session_generation"`
	jwt.RegisteredClaims
}

// Issue issues a dashboard session. It remains as a compatibility wrapper for
// callers that predate audience-specific sessions.
func Issue(secret, userID, displayName, provider, role, accountID string) (string, error) {
	return IssueWithAudience(secret, DashboardAudience, userID, displayName, provider, role, accountID, 0)
}

// IssueWithAudience issues a session in one of the two explicit trust domains.
func IssueWithAudience(secret, audience, userID, displayName, provider, role, accountID string, generation int64) (string, error) {
	return issue(secret, audience, userID, displayName, provider, role, accountID, generation, false)
}

// IssueTeamMember issues a dashboard team-member session for compatibility
// with the existing cloud invitation flow.
func IssueTeamMember(secret, userID, displayName, provider, ownerAccountID string) (string, error) {
	return IssueTeamMemberWithGeneration(secret, userID, displayName, provider, ownerAccountID, 0)
}

func IssueTeamMemberWithGeneration(secret, userID, displayName, provider, ownerAccountID string, generation int64) (string, error) {
	return issue(secret, DashboardAudience, userID, displayName, provider, "admin", ownerAccountID, generation, true)
}

func issue(secret, audience, userID, displayName, provider, role, accountID string, generation int64, teamMember bool) (string, error) {
	if audience != DashboardAudience && audience != EmbedAudience {
		return "", fmt.Errorf("unsupported session audience: %s", audience)
	}
	claims := Claims{
		Sub: userID, DisplayName: displayName, Provider: provider, Role: role,
		AccountID: accountID, IsTeamMember: teamMember, SessionGeneration: generation,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(30 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Audience:  jwt.ClaimStrings{audience},
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// Parse validates a dashboard session. Legacy tokens without an audience are
// intentionally rejected by the same parser used for the new cookies.
func Parse(secret, tokenStr string) (*Claims, error) {
	return ParseForAudience(secret, tokenStr, DashboardAudience)
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

// SetCookie issues a dashboard cookie for source compatibility.
func SetCookie(w http.ResponseWriter, token string, secure bool) {
	SetCookieForAudience(w, token, DashboardAudience, secure)
}

func SetCookieForAudience(w http.ResponseWriter, token, audience string, secure bool) {
	setAudienceCookie(w, token, audience, secure, 30*24*60*60)
	ClearLegacyCookie(w, secure)
}

func setAudienceCookie(w http.ResponseWriter, token, audience string, secure bool, maxAge int) {
	sameSite := http.SameSiteLaxMode
	if audience == EmbedAudience && secure {
		// SameSite=None; Secure is required for cross-origin widget requests.
		sameSite = http.SameSiteNoneMode
	}
	// #nosec G124 -- Secure follows the deployment's HTTPS setting; secure embed cookies require SameSite=None.
	http.SetCookie(w, &http.Cookie{
		Name: cookieName(audience), Value: token, Path: "/", HttpOnly: true,
		Secure: secure, SameSite: sameSite, MaxAge: maxAge,
	})
}

func ClearCookie(w http.ResponseWriter) {
	ClearCookieForAudience(w, DashboardAudience, false)
}

func ClearCookieForAudience(w http.ResponseWriter, audience string, secure bool) {
	setAudienceCookie(w, "", audience, secure, -1)
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

func CookieNameForAudience(audience string) string { return cookieName(audience) }

func cookieName(audience string) string {
	if audience == EmbedAudience {
		return EmbedCookieName
	}
	return DashboardCookieName
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
