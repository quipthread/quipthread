//go:build selfhosted

package middleware

import (
	"net/http"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/models"
	"github.com/quipthread/quipthread/session"
)

// PlanLimits maps plan → [commentsPerMonth, sitesLimit] (-1 = unlimited).
var PlanLimits = map[string][2]int{
	"selfhosted": {-1, 1},
	"hobby":      {1_000, 1},
	"starter":    {10_000, 5},
	"pro":        {50_000, 20},
	"business":   {250_000, -1},
	"enterprise": {-1, -1},
}

// PlanRank maps plan name → ordinal for comparison.
var PlanRank = map[string]int{
	"selfhosted": 0, "hobby": 0, "starter": 1, "pro": 2, "business": 3, "enterprise": 4,
}

// InvalidateSubCache is a no-op in selfhosted builds.
func InvalidateSubCache() {}

// GetCachedSubscription returns a synthetic "selfhosted" subscription.
func GetCachedSubscription(_ string, _ db.Store) (*models.Subscription, error) {
	return &models.Subscription{Plan: "selfhosted", Status: "active"}, nil
}

// AccountIDFromRequest extracts the AccountID from the JWT claims in context.
func AccountIDFromRequest(r *http.Request) string {
	claims, _ := r.Context().Value(session.UserKey).(*session.Claims)
	if claims != nil {
		return claims.AccountID
	}
	return ""
}

// RequirePlan is a no-op in selfhosted builds.
func RequirePlan(_ db.Store, _ *config.Config, _ string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler { return next }
}

// EnforceCommentQuota is a no-op in selfhosted builds — no monthly comment cap.
func EnforceCommentQuota(_ db.Store, _ *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler { return next }
}

// EnforceSiteLimit caps selfhosted installs at 1 site.
func EnforceSiteLimit(store db.Store, _ *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count, err := store.CountSites()
			if err == nil && count >= 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusPaymentRequired)
				_, _ = w.Write([]byte(`{"error":"site_limit_exceeded"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
