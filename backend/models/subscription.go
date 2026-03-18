package models

import "time"

// Subscription holds the current Stripe billing state for this installation.
// A single row (id = "account") is maintained in the subscriptions table.
type Subscription struct {
	StripeCustomerID string
	StripeSubID      string
	Plan             string // hobby | starter | pro | business
	Status           string // active | trialing | past_due | canceled
	Interval         string // month | year (empty for hobby)
	TrialEndsAt      *time.Time
	CurrentPeriodEnd *time.Time
	UpdatedAt        time.Time
}
