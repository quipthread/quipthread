package middleware

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/quipthread/quipthread/session"
)

// RequireAuth validates the session JWT and attaches claims to the request context.
// Returns 401 if the cookie is missing or the token is invalid.
func RequireAuth(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(session.CookieName)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "authentication required")
				return
			}

			claims, err := session.Parse(jwtSecret, cookie.Value)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid or expired session")
				return
			}

			ctx := context.WithValue(r.Context(), session.UserKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdmin runs after RequireAuth. Returns 403 if the user is not an admin.
func RequireAdmin(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(session.CookieName)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "authentication required")
				return
			}

			claims, err := session.Parse(jwtSecret, cookie.Value)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid or expired session")
				return
			}

			if claims.Role != "admin" {
				writeError(w, http.StatusForbidden, "admin access required")
				return
			}

			ctx := context.WithValue(r.Context(), session.UserKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg}) //nolint:errcheck // error response; connection may already be broken
}
