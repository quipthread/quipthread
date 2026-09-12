package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/quipthread/quipthread/plans"
)

// EnableCloudQuotas activates database-enforced limits only on cloud stores.
// The seed preserves visible current-month usage when upgrading an existing DB.
func EnableCloudQuotas(ctx context.Context, store Store) error {
	capable, ok := store.(interface{ enableCloudQuotas(context.Context) error })
	if !ok {
		return errors.New("store does not support atomic cloud quotas")
	}
	return capable.enableCloudQuotas(ctx)
}

func (s *sqlStore) enableCloudQuotas(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // Expected after commit.
	for name, limit := range plans.Catalog() {
		if _, err := tx.ExecContext(ctx, `INSERT INTO cloud_plan_limits(plan, comments_limit, sites_limit) VALUES(?,?,?) ON CONFLICT(plan) DO UPDATE SET comments_limit=excluded.comments_limit, sites_limit=excluded.sites_limit`, name, limit.Comments, limit.Sites); err != nil {
			return fmt.Errorf("configure cloud limits: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO cloud_comment_usage(month, used) SELECT strftime('%Y-%m','now'), COUNT(*) FROM comments WHERE created_at >= strftime('%Y-%m-01','now') AND created_at < strftime('%Y-%m-01','now','+1 month')`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO cloud_quota_enabled(id) VALUES(1)`); err != nil {
		return err
	}
	return tx.Commit()
}

func QuotaErrorCode(err error) string {
	if err == nil {
		return ""
	}
	for _, code := range []string{"monthly_limit_exceeded", "site_limit_exceeded"} {
		if strings.Contains(err.Error(), "quipthread_"+code) {
			return code
		}
	}
	return ""
}
