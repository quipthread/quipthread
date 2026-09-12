package db

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/quipthread/quipthread/models"
)

func newPersistentOutboxStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := NewSQLiteStoreForTest(fmt.Sprintf("%s/tenant.db", t.TempDir()))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { store.Close() }) //nolint:errcheck,gosec // test cleanup
	return store
}

func outboxItem(site, kind, aggregate string, available time.Time) *models.NotificationOutbox {
	return &models.NotificationOutbox{
		SiteID:       site,
		Kind:         kind,
		AggregateKey: aggregate,
		AvailableAt:  available,
	}
}

func TestNotificationOutboxEnqueueIsIdempotentConcurrently(t *testing.T) {
	store := newPersistentOutboxStore(t)
	const callers = 12
	ids := make(chan string, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			item := outboxItem("site-1", "comment.pending", "comment-1", time.Unix(100, 0).UTC())
			if err := store.EnqueueNotificationOutbox(item); err != nil {
				t.Errorf("enqueue: %v", err)
				return
			}
			ids <- item.ID
		}()
	}
	wg.Wait()
	close(ids)
	var first string
	for id := range ids {
		if first == "" {
			first = id
		} else if id != first {
			t.Errorf("concurrent enqueue returned IDs %q and %q", first, id)
		}
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM notification_outbox`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("outbox row count = %d, want 1", count)
	}
}

func TestNotificationOutboxClaimLimitOrderingAndExpiryRecovery(t *testing.T) {
	store := newPersistentOutboxStore(t)
	base := time.Unix(1000, 0).UTC()
	for _, item := range []*models.NotificationOutbox{
		{ID: "id-late", SiteID: "site-1", Kind: "kind", AggregateKey: "late", AvailableAt: base.Add(2 * time.Second), CreatedAt: base},
		{ID: "id-early-b", SiteID: "site-1", Kind: "kind", AggregateKey: "early-b", AvailableAt: base, CreatedAt: base},
		{ID: "id-early-a", SiteID: "site-1", Kind: "kind", AggregateKey: "early-a", AvailableAt: base, CreatedAt: base},
	} {
		if err := store.EnqueueNotificationOutbox(item); err != nil {
			t.Fatal(err)
		}
	}
	claimed, err := store.ClaimNotificationOutbox("worker-1", base.Add(time.Second), time.Minute, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 2 {
		t.Fatalf("claimed %d rows, want 2", len(claimed))
	}
	if claimed[0].AggregateKey != "early-a" || claimed[1].AggregateKey != "early-b" {
		t.Fatalf("claim order = %q, %q", claimed[0].AggregateKey, claimed[1].AggregateKey)
	}
	if claimed[0].Attempts != 1 || claimed[1].Attempts != 1 {
		t.Fatalf("claim attempts = %d, %d; want 1", claimed[0].Attempts, claimed[1].Attempts)
	}

	if _, err := store.ClaimNotificationOutbox("worker-2", base.Add(time.Second), time.Minute, 10); err != nil {
		t.Fatal(err)
	}
	// The first two leases are still live. At expiry, a new owner can recover
	// them and the previously unavailable row is also ready.
	claimed, err = store.ClaimNotificationOutbox("worker-3", base.Add(61*time.Second), time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 3 {
		t.Fatalf("recovery claim count = %d, want 3", len(claimed))
	}
}

func TestNotificationOutboxConcurrentClaimersDoNotDuplicate(t *testing.T) {
	store := newPersistentOutboxStore(t)
	now := time.Unix(2000, 0).UTC()
	const jobs = 12
	for i := range jobs {
		if err := store.EnqueueNotificationOutbox(outboxItem("site-1", "kind", fmt.Sprintf("job-%02d", i), now)); err != nil {
			t.Fatal(err)
		}
	}

	claimedIDs := make(chan string, jobs)
	var wg sync.WaitGroup
	for i := range 4 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			items, err := store.ClaimNotificationOutbox(fmt.Sprintf("worker-%d", i), now, time.Minute, 3)
			if err != nil {
				t.Errorf("claimer %d: %v", i, err)
				return
			}
			for _, item := range items {
				claimedIDs <- item.ID
			}
		}(i)
	}
	wg.Wait()
	close(claimedIDs)
	seen := make(map[string]bool)
	for id := range claimedIDs {
		if seen[id] {
			t.Errorf("row %q claimed twice", id)
		}
		seen[id] = true
	}
	if len(seen) != jobs {
		t.Fatalf("claimed %d unique rows, want %d", len(seen), jobs)
	}
}

func TestNotificationOutboxConcurrentClaimersAcrossIndependentStores(t *testing.T) {
	path := fmt.Sprintf("%s/tenant.db", t.TempDir())
	first, err := NewSQLiteStoreForTest(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close() //nolint:errcheck,gosec // test cleanup
	second, err := NewSQLiteStoreForTest(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close() //nolint:errcheck,gosec // test cleanup

	now := time.Unix(2500, 0).UTC()
	const jobs = 10
	for i := range jobs {
		if err := first.EnqueueNotificationOutbox(outboxItem("site-1", "kind", fmt.Sprintf("independent-%02d", i), now)); err != nil {
			t.Fatal(err)
		}
	}

	type claimResult struct {
		items []*models.NotificationOutbox
		err   error
	}
	results := make(chan claimResult, 2)
	var wg sync.WaitGroup
	for i, candidate := range []*SQLiteStore{first, second} {
		wg.Add(1)
		go func(i int, candidate *SQLiteStore) {
			defer wg.Done()
			items, claimErr := candidate.ClaimNotificationOutbox(fmt.Sprintf("independent-worker-%d", i), now, time.Minute, jobs)
			results <- claimResult{items: items, err: claimErr}
		}(i, candidate)
	}
	wg.Wait()
	close(results)
	seen := make(map[string]bool)
	for result := range results {
		if result.err != nil {
			t.Fatalf("independent store claim: %v", result.err)
		}
		for _, item := range result.items {
			if seen[item.ID] {
				t.Fatalf("independent stores claimed %q twice", item.ID)
			}
			seen[item.ID] = true
		}
	}
	if len(seen) != jobs {
		t.Fatalf("independent stores claimed %d unique rows, want %d", len(seen), jobs)
	}
}

func TestNotificationOutboxOwnerCheckedAckAndRetry(t *testing.T) {
	store := newPersistentOutboxStore(t)
	now := time.Unix(3000, 0).UTC()
	item := outboxItem("site-1", "kind", "aggregate", now)
	if err := store.EnqueueNotificationOutbox(item); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimNotificationOutbox("owner-a", now, time.Minute, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: %v, rows=%d", err, len(claimed))
	}
	firstGeneration := claimed[0].Attempts
	expiry := claimed[0].LeaseUntil
	if expiry == nil {
		t.Fatal("claim returned no lease expiry")
	}
	for _, boundary := range []time.Time{*expiry, expiry.Add(time.Nanosecond)} {
		if err := store.AckNotificationOutbox(item.ID, "owner-a", firstGeneration, boundary); !errors.Is(err, ErrNotificationOutboxNotOwned) {
			t.Fatalf("ack at/after expiry error = %v", err)
		}
		if err := store.RetryNotificationOutbox(item.ID, "owner-a", firstGeneration, boundary.Add(time.Minute), boundary, "expired"); !errors.Is(err, ErrNotificationOutboxNotOwned) {
			t.Fatalf("retry at/after expiry error = %v", err)
		}
		stored, stateErr := store.GetNotificationOutbox(item.ID)
		if stateErr != nil {
			t.Fatal(stateErr)
		}
		if stored.Status != models.NotificationOutboxLeased || stored.Attempts != firstGeneration || stored.LeaseOwner != "owner-a" || stored.LeaseUntil == nil || !stored.LeaseUntil.Equal(*expiry) {
			t.Fatalf("expired transition mutated row: %#v", stored)
		}
	}
	if err := store.AckNotificationOutbox(item.ID, "owner-a", firstGeneration, time.Time{}); !errors.Is(err, ErrInvalidNotificationOutbox) {
		t.Fatalf("zero-time ack error = %v", err)
	}
	if err := store.AckNotificationOutbox(item.ID, "owner-b", claimed[0].Attempts, now); !errors.Is(err, ErrNotificationOutboxNotOwned) {
		t.Fatalf("wrong-owner ack error = %v", err)
	}
	future := now.Add(10 * time.Minute)
	if err := store.RetryNotificationOutbox(item.ID, "owner-b", claimed[0].Attempts, future, now, "no"); !errors.Is(err, ErrNotificationOutboxNotOwned) {
		t.Fatalf("wrong-owner retry error = %v", err)
	}
	if err := store.RetryNotificationOutbox(item.ID, "owner-a", claimed[0].Attempts, future, now, "temporary failure"); err != nil {
		t.Fatal(err)
	}
	stored, err := store.GetNotificationOutbox(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != models.NotificationOutboxPending || stored.LeaseOwner != "" || stored.LeaseUntil != nil || stored.LastError != "temporary failure" || !stored.AvailableAt.Equal(future) {
		t.Fatalf("retry state = %#v", stored)
	}
	if got, err := store.ClaimNotificationOutbox("owner-a", now, time.Minute, 1); err != nil {
		t.Fatal(err)
	} else if len(got) != 0 {
		t.Fatalf("future retry claimed early: %#v", got)
	}
	claimed, err = store.ClaimNotificationOutbox("owner-a", future, time.Minute, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim retried row: %v, rows=%d", err, len(claimed))
	}
	if err := store.AckNotificationOutbox(item.ID, "owner-a", firstGeneration, future); !errors.Is(err, ErrNotificationOutboxNotOwned) {
		t.Fatalf("stale-generation ack error = %v", err)
	}
	if err := store.AckNotificationOutbox(item.ID, "owner-a", claimed[0].Attempts, future); err != nil {
		t.Fatal(err)
	}
	stored, err = store.GetNotificationOutbox(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != models.NotificationOutboxSent || stored.LastError != "" || stored.LeaseOwner != "" || stored.LeaseUntil != nil {
		t.Fatalf("ack state = %#v", stored)
	}
}

func TestNotificationOutboxSameOwnerReclaimFencesStaleGeneration(t *testing.T) {
	store := newPersistentOutboxStore(t)
	now := time.Unix(3500, 0).UTC()
	item := outboxItem("site-1", "kind", "reclaimed", now)
	if err := store.EnqueueNotificationOutbox(item); err != nil {
		t.Fatal(err)
	}
	first, err := store.ClaimNotificationOutbox("same-owner", now, time.Minute, 1)
	if err != nil || len(first) != 1 {
		t.Fatalf("first claim: %v, rows=%d", err, len(first))
	}
	reclaimNow := first[0].LeaseUntil.Add(time.Nanosecond)
	second, err := store.ClaimNotificationOutbox("same-owner", reclaimNow, time.Minute, 1)
	if err != nil || len(second) != 1 {
		t.Fatalf("reclaim: %v, rows=%d", err, len(second))
	}
	if second[0].Attempts == first[0].Attempts {
		t.Fatalf("reclaim generation did not advance: %d", second[0].Attempts)
	}
	if err := store.RetryNotificationOutbox(item.ID, "same-owner", first[0].Attempts, reclaimNow.Add(time.Second), reclaimNow, "stale"); !errors.Is(err, ErrNotificationOutboxNotOwned) {
		t.Fatalf("stale-generation retry error = %v", err)
	}
	if err := store.AckNotificationOutbox(item.ID, "same-owner", second[0].Attempts, reclaimNow); err != nil {
		t.Fatalf("current-generation ack: %v", err)
	}
}

func TestNotificationOutboxRejectsInvalidValues(t *testing.T) {
	store := newPersistentOutboxStore(t)
	tests := []*models.NotificationOutbox{
		nil,
		{Status: models.NotificationOutboxSent, SiteID: "site", Kind: "kind", AggregateKey: "key"},
		{SiteID: " ", Kind: "kind", AggregateKey: "key"},
		{SiteID: "site", Kind: "kind", AggregateKey: "key", Attempts: 1},
	}
	for i, item := range tests {
		if err := store.EnqueueNotificationOutbox(item); !errors.Is(err, ErrInvalidNotificationOutbox) && !errors.Is(err, ErrInvalidNotificationOutboxStatus) {
			t.Errorf("invalid enqueue %d error = %v", i, err)
		}
	}
	item := outboxItem("site", "kind", "key", time.Unix(4000, 0).UTC())
	if err := store.EnqueueNotificationOutbox(item); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimNotificationOutbox("worker", time.Unix(4000, 0).UTC(), time.Minute, 0); !errors.Is(err, ErrInvalidNotificationOutbox) {
		t.Errorf("zero claim limit error = %v", err)
	}
	if _, err := store.ClaimNotificationOutbox("worker", time.Unix(4000, 0).UTC(), time.Minute, NotificationOutboxMaxClaimLimit+1); !errors.Is(err, ErrInvalidNotificationOutbox) {
		t.Errorf("oversized claim limit error = %v", err)
	}
	if err := store.RetryNotificationOutbox(item.ID, "nobody", 1, time.Time{}, time.Time{}, ""); !errors.Is(err, ErrInvalidNotificationOutbox) {
		t.Errorf("invalid retry error = %v", err)
	}
	if err := store.RetryNotificationOutbox(item.ID, "worker", 1, time.Unix(4000, 0).UTC(), time.Time{}, ""); !errors.Is(err, ErrInvalidNotificationOutbox) {
		t.Errorf("zero retry time error = %v", err)
	}
	if err := store.RetryNotificationOutbox(item.ID, "worker", 1, time.Unix(4000, 0).UTC(), time.Unix(4000, 0).UTC(), ""); !errors.Is(err, ErrInvalidNotificationOutbox) {
		t.Errorf("non-future retry availability error = %v", err)
	}
	tooLong := make([]byte, NotificationOutboxMaxErrorBytes+1)
	if err := store.RetryNotificationOutbox(item.ID, "worker", 1, time.Unix(4000, 0).UTC().Add(time.Minute), time.Unix(4000, 0).UTC(), string(tooLong)); !errors.Is(err, ErrInvalidNotificationOutbox) {
		t.Errorf("oversized error = %v", err)
	}
}
