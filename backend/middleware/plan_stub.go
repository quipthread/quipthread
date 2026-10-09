//go:build !cloud && !selfhosted

package middleware

import (
	"net/http"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/models"
	"github.com/quipthread/quipthread/session"
)

// GetCachedSubscription returns a synthetic "business" subscription in
// dev builds — all features unlocked, no Stripe interaction.
func GetCachedSubscription(_ string, _ db.Store) (*models.Subscription, error) {
	return &models.Subscription{Plan: "business", Status: "active"}, nil
}

// AccountIDFromRequest extracts the AccountID from the JWT claims in context.
// Returns "" for unauthenticated requests.
func AccountIDFromRequest(r *http.Request) string {
	claims, _ := r.Context().Value(session.UserKey).(*session.Claims)
	if claims != nil {
		return claims.AccountID
	}
	return ""
}

// RequirePlan is a no-op in dev builds — all plans are considered met.
func RequirePlan(_ db.Store, _ *config.Config, _ string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler { return next }
}

// EnforceCommentQuota is a no-op in dev builds.
func EnforceCommentQuota(_ db.Store, _ *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler { return next }
}

// EnforceSiteLimit is a no-op in dev builds.
func EnforceSiteLimit(_ db.Store, _ *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler { return next }
}
