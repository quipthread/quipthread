package db

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/quipthread/quipthread/models"
)

func seedPendingCommentTenant(t *testing.T, store Store, siteID string, interval *int) {
	t.Helper()
	if err := store.CreateSite(&models.Site{ID: siteID, OwnerID: "owner", Domain: siteID + ".example"}); err != nil {
		t.Fatal(err)
	}
	if interval != nil {
		if err := store.UpdateSite(&models.Site{ID: siteID, Theme: "auto", NotifyInterval: interval}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.UpsertUser(&models.User{ID: "commenter", DisplayName: "Commenter", Role: "commenter"}); err != nil {
		t.Fatal(err)
	}
}

func pendingComment(siteID, id string, createdAt time.Time) *models.Comment {
	return &models.Comment{
		ID:        id,
		SiteID:    siteID,
		PageID:    "/page",
		UserID:    "commenter",
		Content:   id,
		Status:    "pending",
		CreatedAt: createdAt,
	}
}

func readDigestRows(t *testing.T, store Store) (count int, aggregate string, availableAt time.Time) {
	t.Helper()
	s := store.(*SQLiteStore)
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM notification_outbox`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count > 0 {
		if err := s.db.QueryRow(`SELECT aggregate_key, available_at FROM notification_outbox LIMIT 1`).Scan(&aggregate, &availableAt); err != nil {
			t.Fatal(err)
		}
	}
	return
}

func TestCreatePendingCommentWithNotificationDefaultAndConfiguredWindows(t *testing.T) {
	store := newTestStore(t)
	seedPendingCommentTenant(t, store, "default-site", nil)
	created := time.Date(2026, 8, 27, 12, 3, 17, 500000000, time.UTC)
	if err := store.CreatePendingCommentWithNotification(pendingComment("default-site", "default-comment", created)); err != nil {
		t.Fatal(err)
	}
	count, aggregate, availableAt := readDigestRows(t, store)
	if count != 1 || aggregate != "site-digest:300:1787832000" || !availableAt.Equal(time.Date(2026, 8, 27, 12, 5, 0, 0, time.UTC)) {
		t.Fatalf("default digest = count %d aggregate %q available %v", count, aggregate, availableAt)
	}

	interval := 60
	seedPendingCommentTenant(t, store, "configured-site", &interval)
	configuredCreated := time.Date(2026, 8, 27, 12, 3, 17, 0, time.UTC)
	if err := store.CreatePendingCommentWithNotification(pendingComment("configured-site", "configured-comment", configuredCreated)); err != nil {
		t.Fatal(err)
	}
	var configuredAggregate string
	var configuredAvailable time.Time
	if err := store.(*SQLiteStore).db.QueryRow(`SELECT aggregate_key, available_at FROM notification_outbox WHERE site_id = ?`, "configured-site").Scan(&configuredAggregate, &configuredAvailable); err != nil {
		t.Fatal(err)
	}
	if configuredAggregate != "site-digest:60:1787832180" || !configuredAvailable.Equal(time.Date(2026, 8, 27, 12, 4, 0, 0, time.UTC)) {
		t.Fatalf("configured digest = aggregate %q available %v", configuredAggregate, configuredAvailable)
	}
}

func TestCreatePendingCommentWithNotificationCoalescesAndSeparatesAdjacentWindows(t *testing.T) {
	store := newTestStore(t)
	interval := 300
	seedPendingCommentTenant(t, store, "site-1", &interval)
	base := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	for _, comment := range []*models.Comment{
		pendingComment("site-1", "same-window-1", base.Add(2*time.Minute)),
		pendingComment("site-1", "same-window-2", base.Add(4*time.Minute)),
		pendingComment("site-1", "adjacent-window", base.Add(5*time.Minute)),
	} {
		if err := store.CreatePendingCommentWithNotification(comment); err != nil {
			t.Fatal(err)
		}
	}
	var comments, outbox int
	s := store.(*SQLiteStore)
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM comments WHERE site_id = ?`, "site-1").Scan(&comments); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM notification_outbox WHERE site_id = ?`, "site-1").Scan(&outbox); err != nil {
		t.Fatal(err)
	}
	if comments != 3 || outbox != 2 {
		t.Fatalf("coalesced rows = comments %d outbox %d, want 3 and 2", comments, outbox)
	}
}

func TestCreatePendingCommentWithNotificationFileBackedConnectionsCoalesce(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "comments.db")
	first, err := NewSQLiteStoreForTest(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close() //nolint:errcheck,gosec // test cleanup
	second, err := NewSQLiteStoreForTest(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close() //nolint:errcheck,gosec // test cleanup
	seedPendingCommentTenant(t, first, "site-1", nil)

	created := time.Date(2026, 8, 27, 12, 3, 0, 0, time.UTC)
	comments := []*models.Comment{
		pendingComment("site-1", "connection-1", created),
		pendingComment("site-1", "connection-2", created),
	}
	stores := []*SQLiteStore{first, second}
	errs := make(chan error, len(stores))
	var wg sync.WaitGroup
	for i, store := range stores {
		wg.Add(1)
		go func(store *SQLiteStore, comment *models.Comment) {
			defer wg.Done()
			errs <- store.CreatePendingCommentWithNotification(comment)
		}(store, comments[i])
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent file-backed create: %v", err)
		}
	}

	var commentCount, outboxCount int
	if err := first.db.QueryRow(`SELECT COUNT(*) FROM comments WHERE site_id = ?`, "site-1").Scan(&commentCount); err != nil {
		t.Fatal(err)
	}
	if err := first.db.QueryRow(`SELECT COUNT(*) FROM notification_outbox WHERE site_id = ?`, "site-1").Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if commentCount != 2 || outboxCount != 1 {
		t.Fatalf("concurrent file-backed rows = comments %d outbox %d, want 2 and 1", commentCount, outboxCount)
	}
}

func TestCreatePendingCommentWithNotificationRetriesSQLiteBusyWithStableComment(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "comments.db")
	first, err := NewSQLiteStoreForTest(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close() //nolint:errcheck,gosec // test cleanup
	second, err := NewSQLiteStoreForTest(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close() //nolint:errcheck,gosec // test cleanup
	seedPendingCommentTenant(t, first, "site-1", nil)

	lock, err := first.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lock.Exec(`UPDATE sites SET theme = ? WHERE id = ?`, "locked", "site-1"); err != nil {
		lock.Rollback() //nolint:errcheck,gosec // test cleanup
		t.Fatal(err)
	}

	created := time.Date(2026, 8, 27, 12, 3, 0, 0, time.UTC)
	comment := pendingComment("site-1", "stable-retry", created)
	result := make(chan error, 1)
	go func() { result <- second.CreatePendingCommentWithNotification(comment) }()
	time.Sleep(25 * time.Millisecond)
	if err := lock.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatalf("retryable SQLite conflict was not retried: %v", err)
	}

	stored, err := second.GetComment(comment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil || !stored.CreatedAt.Equal(created) || !stored.UpdatedAt.Equal(comment.UpdatedAt) {
		t.Fatalf("retried comment = %#v, want stable ID/timestamps from request", stored)
	}
}

func TestCreatePendingCommentWithNotificationRejectsLeasedOrSentDigest(t *testing.T) {
	for _, status := range []string{models.NotificationOutboxLeased, models.NotificationOutboxSent} {
		t.Run(status, func(t *testing.T) {
			store := newTestStore(t)
			seedPendingCommentTenant(t, store, "site-1", nil)
			created := time.Date(2026, 8, 27, 12, 3, 0, 0, time.UTC)
			if err := store.CreatePendingCommentWithNotification(pendingComment("site-1", "existing", created)); err != nil {
				t.Fatal(err)
			}
			s := store.(*SQLiteStore)
			if status == models.NotificationOutboxLeased {
				if _, err := s.db.Exec(`UPDATE notification_outbox SET status = ?, lease_until = ?, lease_owner = ?`, status, created.Add(time.Minute), "test-worker"); err != nil {
					t.Fatal(err)
				}
			} else if _, err := s.db.Exec(`UPDATE notification_outbox SET status = ?`, status); err != nil {
				t.Fatal(err)
			}

			candidate := pendingComment("site-1", "must-rollback", created.Add(time.Minute))
			err := store.CreatePendingCommentWithNotification(candidate)
			if !errors.Is(err, ErrDigestOutboxNotPending) {
				t.Fatalf("status=%s error = %v, want ErrDigestOutboxNotPending", status, err)
			}
			var comments int
			if err := s.db.QueryRow(`SELECT COUNT(*) FROM comments WHERE id = ?`, candidate.ID).Scan(&comments); err != nil {
				t.Fatal(err)
			}
			if comments != 0 {
				t.Fatalf("status=%s left %d candidate comments", status, comments)
			}
		})
	}
}

func TestCreatePendingCommentWithNotificationRollsBackCommentOnOutboxFailure(t *testing.T) {
	store := newTestStore(t)
	seedPendingCommentTenant(t, store, "site-1", nil)
	s := store.(*SQLiteStore)
	if _, err := s.db.Exec(`
		CREATE TRIGGER fail_notification_outbox
		BEFORE INSERT ON notification_outbox
		BEGIN SELECT RAISE(ABORT, 'forced outbox failure'); END`); err != nil {
		t.Fatal(err)
	}
	defer s.db.Exec(`DROP TRIGGER fail_notification_outbox`) //nolint:errcheck,gosec // test cleanup

	if err := store.CreatePendingCommentWithNotification(pendingComment("site-1", "rolled-back", time.Date(2026, 8, 27, 12, 3, 0, 0, time.UTC))); err == nil {
		t.Fatal("transaction unexpectedly succeeded")
	}
	var comments, outbox int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM comments WHERE id = 'rolled-back'`).Scan(&comments); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM notification_outbox`).Scan(&outbox); err != nil {
		t.Fatal(err)
	}
	if comments != 0 || outbox != 0 {
		t.Fatalf("rollback left comments=%d outbox=%d", comments, outbox)
	}
}

func TestCreatePendingCommentWithNotificationRejectsNonPending(t *testing.T) {
	store := newTestStore(t)
	if err := store.CreatePendingCommentWithNotification(&models.Comment{Status: "approved"}); !errors.Is(err, ErrInvalidPendingComment) {
		t.Fatalf("non-pending error = %v, want ErrInvalidPendingComment", err)
	}
}

func TestCreatePendingCommentWithNotificationRejectsUnsafeIntervals(t *testing.T) {
	for _, interval := range []int{MinSiteDigestIntervalSeconds - 1, MaxSiteDigestIntervalSeconds + 1} {
		t.Run(fmt.Sprintf("%d", interval), func(t *testing.T) {
			store := newTestStore(t)
			seedPendingCommentTenant(t, store, "site-1", &interval)
			err := store.CreatePendingCommentWithNotification(pendingComment("site-1", "unsafe", time.Date(2026, 8, 27, 12, 3, 0, 0, time.UTC)))
			if !errors.Is(err, ErrInvalidSiteDigestInterval) {
				t.Fatalf("interval=%d error = %v, want ErrInvalidSiteDigestInterval", interval, err)
			}
			if comments := countCommentsByID(t, store, "unsafe"); comments != 0 {
				t.Fatalf("interval=%d left %d comments", interval, comments)
			}
		})
	}
}

func TestSiteDigestWindowRejectsTimestampOverflow(t *testing.T) {
	if _, _, err := siteDigestWindow(time.Unix(math.MaxInt64, 0), DefaultSiteDigestIntervalSeconds); !errors.Is(err, ErrInvalidSiteDigestInterval) {
		t.Fatalf("max timestamp error = %v, want ErrInvalidSiteDigestInterval", err)
	}
	if _, _, err := siteDigestWindow(time.Unix(math.MinInt64, 0), DefaultSiteDigestIntervalSeconds); !errors.Is(err, ErrInvalidSiteDigestInterval) {
		t.Fatalf("min timestamp error = %v, want ErrInvalidSiteDigestInterval", err)
	}
}

func countCommentsByID(t *testing.T, store Store, id string) int {
	t.Helper()
	var count int
	if err := store.(*SQLiteStore).db.QueryRow(`SELECT COUNT(*) FROM comments WHERE id = ?`, id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
