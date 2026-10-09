package middleware

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/models"
	"github.com/quipthread/quipthread/session"
)

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
	fail := func(msg string) (*session.Claims, bool) {
		if w != nil {
			writeError(w, http.StatusUnauthorized, msg)
		}
		return nil, false
	}

	// Dashboard sessions live in an HttpOnly cookie on the application origin.
	// Embed sessions are site-bound bearer tokens held by the publisher's page
	// and are never read from cookies.
	var raw string
	if audience == session.EmbedAudience {
		raw = session.BearerToken(r)
	} else if cookie, err := r.Cookie(session.DashboardCookieName); err == nil {
		raw = cookie.Value
	}
	if raw == "" {
		return fail("authentication required")
	}
	claims, err := session.ParseForAudience(jwtSecret, raw, audience)
	if err != nil {
		return fail("invalid or expired session")
	}
	if audience == session.EmbedAudience && !embedClaimsMatchRequest(r, claims) {
		return fail("session not valid for this site")
	}

	store := fallback
	if contextual, ok := db.StoreFromContext(r.Context()); ok {
		store = contextual
	}
	if store != nil {
		user, err := store.GetUser(claims.Sub)
		if err != nil || user == nil || user.Banned || generationForAudience(user, audience) != claims.SessionGeneration {
			return fail("invalid or revoked session")
		}
	}
	return claims, true
}

// embedClaimsMatchRequest binds an embed token to the site the request is
// for. When a public tenant has been resolved, both its site and owning
// account must match the token; otherwise the required siteId query
// parameter must match.
func embedClaimsMatchRequest(r *http.Request, claims *session.Claims) bool {
	if claims.SiteID == "" {
		return false
	}
	if pt, ok := db.PublicTenantFromContext(r.Context()); ok {
		return pt.SiteID == claims.SiteID && pt.AccountID == claims.AccountID
	}
	return r.URL.Query().Get("siteId") == claims.SiteID
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
