package middleware

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/models"
	"github.com/quipthread/quipthread/session"
)

// RequireAuth is the dashboard compatibility wrapper. New routes should name
// their trust domain explicitly with RequireAuthForAudience.
func RequireAuth(jwtSecret string) func(http.Handler) http.Handler {
	return RequireAuthForAudience(jwtSecret, session.DashboardAudience, nil)
}

func RequireAuthForAudience(jwtSecret, audience string, fallback db.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := authenticate(w, r, jwtSecret, audience, fallback)
			if !ok {
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), session.UserKey, claims)))
		})
	}
}

// RequireAdmin runs after RequireAuth and validates a dashboard session.
func RequireAdmin(jwtSecret string) func(http.Handler) http.Handler {
	return RequireAdminForAudience(jwtSecret, session.DashboardAudience, nil)
}

func RequireAdminForAudience(jwtSecret, audience string, fallback db.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := authenticate(w, r, jwtSecret, audience, fallback)
			if !ok {
				return
			}
			if claims.Role != "admin" {
				writeError(w, http.StatusForbidden, "admin access required")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), session.UserKey, claims)))
		})
	}
}

func RequireOwner(jwtSecret string) func(http.Handler) http.Handler {
	return RequireOwnerForAudience(jwtSecret, session.DashboardAudience, nil)
}

func RequireOwnerForAudience(jwtSecret, audience string, fallback db.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := authenticate(w, r, jwtSecret, audience, fallback)
			if !ok {
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
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), session.UserKey, claims)))
		})
	}
}

// InjectAuth is the dashboard compatibility wrapper.
func InjectAuth(jwtSecret string) func(http.Handler) http.Handler {
	return InjectAuthForAudience(jwtSecret, session.DashboardAudience, nil)
}

func InjectAuthForAudience(jwtSecret, audience string, fallback db.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if claims, ok := authenticate(nil, r, jwtSecret, audience, fallback); ok {
				r = r.WithContext(context.WithValue(r.Context(), session.UserKey, claims))
			}
			next.ServeHTTP(w, r)
		})
	}
}

func authenticate(w http.ResponseWriter, r *http.Request, jwtSecret, audience string, fallback db.Store) (*session.Claims, bool) {
	cookie, err := r.Cookie(session.CookieNameForAudience(audience))
	if err != nil {
		if w != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
		}
		return nil, false
	}
	claims, err := session.ParseForAudience(jwtSecret, cookie.Value, audience)
	if err != nil {
		if w != nil {
			writeError(w, http.StatusUnauthorized, "invalid or expired session")
		}
		return nil, false
	}

	store := fallback
	if contextual, ok := db.StoreFromContext(r.Context()); ok {
		store = contextual
	}
	if store != nil {
		user, err := store.GetUser(claims.Sub)
		if err != nil || user == nil || user.Banned || generationForAudience(user, audience) != claims.SessionGeneration {
			if w != nil {
				writeError(w, http.StatusUnauthorized, "invalid or revoked session")
			}
			return nil, false
		}
	}
	return claims, true
}

func generationForAudience(user *models.User, audience string) int64 {
	if audience == session.EmbedAudience {
		return user.EmbedSessionGeneration
	}
	return user.DashboardSessionGeneration
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg}) //nolint:errcheck,gosec // error response; connection may already be broken
}
