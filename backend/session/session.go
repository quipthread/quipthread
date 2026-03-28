package session

import (
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const CookieName = "quipthread_session"

type contextKey string

const UserKey contextKey = "session_user"

type Claims struct {
	Sub         string `json:"sub"`
	DisplayName string `json:"display_name"`
	Provider    string `json:"provider"`
	Role        string `json:"role"`
	AccountID   string `json:"account_id,omitempty"`
	jwt.RegisteredClaims
}

func Issue(secret, userID, displayName, provider, role, accountID string) (string, error) {
	claims := Claims{
		Sub:         userID,
		DisplayName: displayName,
		Provider:    provider,
		Role:        role,
		AccountID:   accountID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(30 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func Parse(secret, tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return claims, nil
}

func SetCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   30 * 24 * 60 * 60,
	})
}

func ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

// SetIndicatorCookie sets a non-HttpOnly cookie that marketing pages can read
// to detect login state cross-origin. Contains no sensitive data.
func SetIndicatorCookie(w http.ResponseWriter, domain string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     "qt_logged_in",
		Value:    "1",
		Path:     "/",
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Domain:   domain,
		MaxAge:   30 * 24 * 60 * 60,
	})
}

// ClearIndicatorCookie removes the login indicator cookie.
func ClearIndicatorCookie(w http.ResponseWriter, domain string) {
	http.SetCookie(w, &http.Cookie{
		Name:   "qt_logged_in",
		Value:  "",
		Path:   "/",
		Domain: domain,
		MaxAge: -1,
	})
}
