-- +goose Up
CREATE TABLE cloud_plan_limits (
    plan TEXT PRIMARY KEY,
    comments_limit INTEGER NOT NULL CHECK (comments_limit >= -1),
    sites_limit INTEGER NOT NULL CHECK (sites_limit >= -1)
);
CREATE TABLE cloud_quota_enabled (id INTEGER PRIMARY KEY CHECK (id = 1));
CREATE TABLE cloud_comment_usage (
    month TEXT PRIMARY KEY,
    used INTEGER NOT NULL CHECK (used >= 0)
);
CREATE VIEW cloud_effective_limits AS
SELECT comments_limit, sites_limit FROM cloud_plan_limits
WHERE plan = COALESCE((SELECT CASE WHEN status IN ('active', 'trialing') AND plan IN
    ('hobby','starter','pro','business','enterprise') THEN plan ELSE 'hobby' END
    FROM subscriptions WHERE id = 'account'), 'hobby');

-- +goose StatementBegin
CREATE TRIGGER cloud_comment_quota AFTER INSERT ON comments
WHEN EXISTS (SELECT 1 FROM cloud_quota_enabled)
BEGIN
    SELECT RAISE(ABORT, 'quipthread_monthly_limit_exceeded') WHERE COALESCE((SELECT comments_limit FROM cloud_effective_limits),0) != -1
        AND COALESCE((SELECT used FROM cloud_comment_usage WHERE month=strftime('%Y-%m','now')),0)
            >= COALESCE((SELECT comments_limit FROM cloud_effective_limits),0)
;
    INSERT INTO cloud_comment_usage(month,used) VALUES(strftime('%Y-%m','now'),1)
        ON CONFLICT(month) DO UPDATE SET used=used+1;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER cloud_site_quota AFTER INSERT ON sites
WHEN EXISTS (SELECT 1 FROM cloud_quota_enabled)
BEGIN
    SELECT RAISE(ABORT, 'quipthread_site_limit_exceeded') WHERE COALESCE((SELECT sites_limit FROM cloud_effective_limits),0) != -1
        AND (SELECT COUNT(*) FROM sites) > COALESCE((SELECT sites_limit FROM cloud_effective_limits),0)
;
END;
-- +goose StatementEnd
