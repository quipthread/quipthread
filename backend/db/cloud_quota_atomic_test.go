package db

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/quipthread/quipthread/models"
)

func quotaFixture(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := NewSQLiteStoreForTest(filepath.Join(t.TempDir(), "quota.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.CreateSite(&models.Site{ID: "site", OwnerID: "owner", Domain: "example.com"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertUser(&models.User{ID: "user", DisplayName: "User"}); err != nil {
		t.Fatal(err)
	}
	if err := EnableCloudQuotas(t.Context(), s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCloudQuotaConcurrentWritersShareLastSlot(t *testing.T) {
	s := quotaFixture(t)
	if _, err := s.db.ExecContext(t.Context(), `UPDATE cloud_comment_usage SET used=999`); err != nil {
		t.Fatal(err)
	}
	second, err := NewSQLiteStoreForTest(s.dbPathForQuotaTest(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { second.Close() })
	if _, err := second.db.ExecContext(t.Context(), `PRAGMA busy_timeout=5000`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(t.Context(), `PRAGMA busy_timeout=5000`); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	outcomes := make(chan error, 12)
	for i := range 12 {
		wg.Go(func() {
			target := s
			if i%2 == 0 {
				target = second
			}
			outcomes <- target.CreateComment(&models.Comment{ID: fmt.Sprint(i), SiteID: "site", UserID: "user", Content: "post", Status: "approved"})
		})
	}
	wg.Wait()
	close(outcomes)
	accepted := 0
	for err := range outcomes {
		if err == nil {
			accepted++
		} else if QuotaErrorCode(err) != "monthly_limit_exceeded" {
			t.Fatal(err)
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted=%d", accepted)
	}
}

func TestCloudQuotaImportRollsBackWholeBatch(t *testing.T) {
	s := quotaFixture(t)
	if _, err := s.db.ExecContext(t.Context(), `UPDATE cloud_comment_usage SET used=999`); err != nil {
		t.Fatal(err)
	}
	comments := []*models.Comment{{ID: "a", UserID: "user", Content: "a", Status: "approved", Imported: true}, {ID: "b", UserID: "user", Content: "b", Status: "approved", Imported: true}}
	n, err := s.ImportComments("site", comments)
	if n != 0 || QuotaErrorCode(err) != "monthly_limit_exceeded" {
		t.Fatalf("import=%d, %v", n, err)
	}
	used, err := s.CountCommentsThisMonth()
	if err != nil || used != 999 {
		t.Fatalf("usage=%d, %v", used, err)
	}
	var count int
	if err := s.db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM comments`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("persisted=%d, %v", count, err)
	}
}

func TestCloudQuotaUpgradeAndDowngradeUseCurrentSubscription(t *testing.T) {
	s := quotaFixture(t)
	if _, err := s.db.ExecContext(t.Context(), `UPDATE cloud_comment_usage SET used=1000`); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		plan, status string
		allowed      bool
	}{{"starter", "active", true}, {"pro", "trialing", true}, {"business", "canceled", false}, {"unknown", "active", false}, {"hobby", "active", false}} {
		if err := s.UpsertSubscription(&models.Subscription{Plan: tc.plan, Status: tc.status}); err != nil {
			t.Fatal(err)
		}
		err := s.CreateComment(&models.Comment{SiteID: "site", UserID: "user", Content: "post", Status: "approved"})
		if (err == nil) != tc.allowed {
			t.Fatalf("%s/%s: %v", tc.plan, tc.status, err)
		}
	}
}

func TestCloudQuotaDuplicateImportDoesNotConsumeUsage(t *testing.T) {
	s := quotaFixture(t)
	c := &models.Comment{ID: "existing", SiteID: "site", UserID: "user", Content: "post", Status: "approved"}
	if err := s.CreateComment(c); err != nil {
		t.Fatal(err)
	}
	n, err := s.ImportComments("site", []*models.Comment{c})
	if err != nil || n != 0 {
		t.Fatalf("import=%d, %v", n, err)
	}
	used, err := s.CountCommentsThisMonth()
	if err != nil || used != 1 {
		t.Fatalf("usage=%d, %v", used, err)
	}
}

func (s *SQLiteStore) dbPathForQuotaTest(t *testing.T) string {
	t.Helper()
	var seq int
	var name, path string
	if err := s.db.QueryRowContext(t.Context(), `PRAGMA database_list`).Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCloudQuotaNewMonthAndReopeningPreserveCounter(t *testing.T) {
	s := quotaFixture(t)
	if _, err := s.db.ExecContext(t.Context(), `UPDATE cloud_comment_usage SET month='2000-01',used=1000`); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateComment(&models.Comment{ID: "current", SiteID: "site", UserID: "user", Content: "new month", Status: "approved"}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteComment("current"); err != nil {
		t.Fatal(err)
	}
	if err := EnableCloudQuotas(t.Context(), s); err != nil {
		t.Fatal(err)
	}
	used, err := s.CountCommentsThisMonth()
	if err != nil || used != 1 {
		t.Fatalf("usage=%d, %v", used, err)
	}
}
