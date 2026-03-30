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

// RequireOwner runs after RequireAuth. Returns 403 if the user is not an admin
// or if the user is a team member. Use this on routes that only account owners
// should access (billing, invitation management, account deletion, etc.).
func RequireOwner(jwtSecret string) func(http.Handler) http.Handler {
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

			if claims.IsTeamMember {
				writeError(w, http.StatusForbidden, "account owner access required")
				return
			}

			ctx := context.WithValue(r.Context(), session.UserKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// InjectAuth tries to parse the session JWT and, if valid, attaches claims to
// the request context. Unlike RequireAuth it never rejects the request — callers
// check for nil claims to distinguish authenticated from anonymous users.
func InjectAuth(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cookie, err := r.Cookie(session.CookieName); err == nil {
				if claims, err := session.Parse(jwtSecret, cookie.Value); err == nil {
					r = r.WithContext(context.WithValue(r.Context(), session.UserKey, claims))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg}) //nolint:errcheck,gosec // error response; connection may already be broken
}
