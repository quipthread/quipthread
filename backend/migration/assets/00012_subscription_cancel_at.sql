-- +goose Up
-- When a subscription is set to end (canceled in the portal), so billing can
-- show it instead of a renewal date.
ALTER TABLE subscriptions ADD COLUMN cancel_at DATETIME;

-- A past_due subscription keeps its plan while Stripe retries payment, as in
-- plans.Effective; the quota view previously fell back to hobby.
DROP VIEW cloud_effective_limits;
CREATE VIEW cloud_effective_limits AS
SELECT comments_limit, sites_limit FROM cloud_plan_limits
WHERE plan = COALESCE((SELECT CASE WHEN status IN ('active', 'trialing', 'past_due') AND plan IN
    ('hobby','starter','pro','business','enterprise') THEN plan ELSE 'hobby' END
    FROM subscriptions WHERE id = 'account'), 'hobby');
