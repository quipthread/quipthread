//go:build !cloud

package middleware

import (
	"net/http"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/models"
	"github.com/quipthread/quipthread/session"
)

// PlanLimits maps plan → [commentsPerMonth, sitesLimit] (-1 = unlimited).
// Kept in the !cloud stub so shared code (e.g. billing status) can reference it.
var PlanLimits = map[string][2]int{
	"hobby":      {1_000, 1},
	"starter":    {10_000, 5},
	"pro":        {50_000, 20},
	"business":   {250_000, -1},
	"enterprise": {-1, -1},
}

// PlanRank maps plan name → ordinal for comparison.
var PlanRank = map[string]int{
	"hobby": 0, "starter": 1, "pro": 2, "business": 3, "enterprise": 4,
}

// InvalidateSubCache is a no-op in self-hosted builds.
func InvalidateSubCache() {}

// GetCachedSubscription returns a synthetic "business" subscription in
// self-hosted builds — all features unlocked, no Stripe interaction.
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

// RequirePlan is a no-op in self-hosted builds — all plans are considered met.
func RequirePlan(_ db.Store, _ *config.Config, _ string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler { return next }
}

// EnforceCommentQuota is a no-op in self-hosted builds.
func EnforceCommentQuota(_ db.Store, _ *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler { return next }
}

// EnforceSiteLimit is a no-op in self-hosted builds.
func EnforceSiteLimit(_ db.Store, _ *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler { return next }
}
