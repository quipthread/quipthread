package middleware

import "github.com/quipthread/quipthread/plans"

// PlanLimits maps plan → [commentsPerMonth, sitesLimit] (-1 = unlimited),
// from the shared plan catalog plus the self-hosted build's own plan: one
// site and no comment limit.
var PlanLimits = func() map[string][2]int {
	result := map[string][2]int{"selfhosted": {-1, 1}}
	for name, limits := range plans.Catalog() {
		result[name] = [2]int{limits.Comments, limits.Sites}
	}
	return result
}()

// PlanRank maps plan → ordinal for plan comparisons.
var PlanRank = func() map[string]int {
	result := map[string]int{"selfhosted": 0}
	for name, limits := range plans.Catalog() {
		result[name] = limits.Rank
	}
	return result
}()
