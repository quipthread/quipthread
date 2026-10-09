package plans

import "github.com/quipthread/quipthread/models"

type Limits struct {
	Comments int
	Sites    int
	Rank     int
}

//go:generate go run ./internal/gencatalog

// Catalog returns every plan's limits and rank, keyed by plan id. The data is
// generated from packages/plans/catalog.json, which the website, dashboard,
// and Stripe setup also read. -1 means unlimited.
func Catalog() map[string]Limits {
	out := make(map[string]Limits, len(catalog))
	for id, limits := range catalog {
		out[id] = limits
	}
	return out
}

func Effective(sub *models.Subscription) string {
	if sub == nil {
		return "hobby"
	}
	// past_due keeps the plan while Stripe retries the payment; the plan
	// drops only once Stripe ends the subscription.
	switch sub.Status {
	case "active", "trialing", "past_due":
	default:
		return "hobby"
	}
	if _, ok := Catalog()[sub.Plan]; !ok {
		return "hobby"
	}
	return sub.Plan
}
