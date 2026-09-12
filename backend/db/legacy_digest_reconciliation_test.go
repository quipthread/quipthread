package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/quipthread/quipthread/models"
)

func legacyDigestFixture(t *testing.T, id, status string, available time.Time) (*SQLiteStore, *models.NotificationOutbox) {
	t.Helper()
	store := newPersistentOutboxStore(t)
	parent := outboxItem("site-legacy", NotificationOutboxKindSiteDigest, "site-digest:300:1800", available)
	parent.ID = id
	if err := store.EnqueueNotificationOutbox(parent); err != nil {
		t.Fatal(err)
	}
	if status == models.NotificationOutboxLeased {
		leaseUntil := available.Add(time.Hour)
		if _, err := store.db.Exec(`UPDATE notification_outbox SET status = ?, lease_owner = ?, lease_until = ?, attempts = ? WHERE id = ?`, status, "worker-a", leaseUntil, 7, id); err != nil {
			t.Fatal(err)
		}
	} else if _, err := store.db.Exec(`UPDATE notification_outbox SET status = ? WHERE id = ?`, status, id); err != nil {
		t.Fatal(err)
	}
	return store, parent
}

func addLegacyComment(t *testing.T, store *SQLiteStore, id string, createdAt time.Time) {
	t.Helper()
	if err := store.CreateSite(&models.Site{ID: "site-legacy", OwnerID: "owner", Domain: "legacy.example"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertUser(&models.User{ID: "user", DisplayName: "Legacy User", Role: "commenter"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO comments
		(id, site_id, page_id, page_url, page_title, parent_id, user_id, content, status, imported, disqus_author, created_at, updated_at)
		VALUES (?, 'site-legacy', '/page', '', '', '', 'user', 'content must never be emitted', 'pending', 0, '', ?, ?)`, id, createdAt.UTC(), createdAt.UTC()); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyDigestInventoryIncludesFutureSentAndLeased(t *testing.T) {
	store := newPersistentOutboxStore(t)
	now := time.Unix(2000, 0).UTC()
	for _, item := range []struct {
		id     string
		status string
	}{
		{"legacy-pending", models.NotificationOutboxPending},
		{"legacy-sent", models.NotificationOutboxSent},
		{"legacy-leased", models.NotificationOutboxLeased},
	} {
		_ = legacyDigestFixtureWithStore(t, store, item.id, item.status, now.Add(time.Hour))
	}
	inventory, err := store.ListUninitializedSiteDigestsContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory) != 3 {
		t.Fatalf("legacy inventory count = %d, want 3", len(inventory))
	}
}

func legacyDigestFixtureWithStore(t *testing.T, store *SQLiteStore, id, status string, available time.Time) *models.NotificationOutbox {
	t.Helper()
	parent := outboxItem("site-"+id, NotificationOutboxKindSiteDigest, "site-digest:300:1800", available)
	parent.ID = id
	if err := store.EnqueueNotificationOutbox(parent); err != nil {
		t.Fatal(err)
	}
	if status == models.NotificationOutboxLeased {
		if _, err := store.db.Exec(`UPDATE notification_outbox SET status = ?, lease_owner = ?, lease_until = ?, attempts = ? WHERE id = ?`, status, "worker-a", available.Add(time.Hour), 7, id); err != nil {
			t.Fatal(err)
		}
	} else if _, err := store.db.Exec(`UPDATE notification_outbox SET status = ? WHERE id = ?`, status, id); err != nil {
		t.Fatal(err)
	}
	return parent
}

func TestLegacyDigestReconcileReleasesAtOriginalAvailabilityAndIsIdempotent(t *testing.T) {
	store, parent := legacyDigestFixture(t, "legacy-reconcile", models.NotificationOutboxPending, time.Unix(2100, 0).UTC())
	commentID := "legacy-comment"
	addLegacyComment(t, store, commentID, time.Unix(1900, 0).UTC())
	now := time.Unix(2000, 0).UTC()
	if err := store.ReconcileLegacySiteDigestContext(context.Background(), parent.ID, "", 0, []string{commentID}, now); err != nil {
		t.Fatal(err)
	}
	updated, err := store.GetNotificationOutbox(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != models.NotificationOutboxPending || updated.DigestMembershipVersion != 1 || !updated.AvailableAt.Equal(parent.AvailableAt) || updated.LeaseOwner != "" || updated.LeaseUntil != nil {
		t.Fatalf("reconciled parent = %#v", updated)
	}
	if err := store.ReconcileLegacySiteDigestContext(context.Background(), parent.ID, "", 0, []string{commentID}, now.Add(time.Minute)); err != nil {
		t.Fatalf("identical retry: %v", err)
	}
	if err := store.ReconcileLegacySiteDigestContext(context.Background(), parent.ID, "", 0, nil, now.Add(2*time.Minute)); !errors.Is(err, ErrLegacyDigestConflict) {
		t.Fatalf("conflicting retry error = %v, want conflict", err)
	}
}

func TestLegacyDigestReconcileFencesLeasedParent(t *testing.T) {
	store, parent := legacyDigestFixture(t, "legacy-leased-reconcile", models.NotificationOutboxLeased, time.Unix(2100, 0).UTC())
	addLegacyComment(t, store, "legacy-leased-comment", time.Unix(1900, 0).UTC())
	now := time.Unix(2200, 0).UTC()
	if err := store.ReconcileLegacySiteDigestContext(context.Background(), parent.ID, "wrong-owner", 7, []string{"legacy-leased-comment"}, now); !errors.Is(err, ErrNotificationOutboxNotOwned) {
		t.Fatalf("wrong owner error = %v, want not owned", err)
	}
	if err := store.ReconcileLegacySiteDigestContext(context.Background(), parent.ID, "worker-a", 7, []string{"legacy-leased-comment"}, now); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyDigestExpiredLeaseCanBeReclaimedBeforeReconcile(t *testing.T) {
	store, parent := legacyDigestFixture(t, "legacy-expired", models.NotificationOutboxLeased, time.Unix(2100, 0).UTC())
	addLegacyComment(t, store, "legacy-expired-comment", time.Unix(1900, 0).UTC())
	now := time.Unix(2200, 0).UTC()
	if _, err := store.db.Exec(`UPDATE notification_outbox SET lease_until = ? WHERE id = ?`, now.Add(-time.Second), parent.ID); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimLegacySiteDigestContext(context.Background(), parent.ID, "repair-owner", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReconcileLegacySiteDigestContext(context.Background(), parent.ID, claimed.LeaseOwner, claimed.Attempts, []string{"legacy-expired-comment"}, now); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyDigestRetireHandlesPendingSentAndLeased(t *testing.T) {
	now := time.Unix(2200, 0).UTC()
	for _, item := range []struct {
		id     string
		status string
		owner  string
		gen    int
	}{
		{"legacy-retire-pending", models.NotificationOutboxPending, "", 0},
		{"legacy-retire-sent", models.NotificationOutboxSent, "", 0},
		{"legacy-retire-leased", models.NotificationOutboxLeased, "worker-a", 7},
	} {
		store, parent := legacyDigestFixture(t, item.id, item.status, time.Unix(2100, 0).UTC())
		if err := store.RetireLegacySiteDigestContext(context.Background(), parent.ID, item.owner, item.gen, now); err != nil {
			t.Fatalf("retire %s: %v", item.status, err)
		}
		updated, err := store.GetNotificationOutbox(parent.ID)
		if err != nil {
			t.Fatal(err)
		}
		if updated.Status != models.NotificationOutboxSent || updated.DigestMembershipVersion != 1 || !updated.AvailableAt.Equal(parent.AvailableAt) {
			t.Fatalf("retired parent %s = %#v", item.status, updated)
		}
	}
}

func TestLegacyDigestInventoryHonorsCancellation(t *testing.T) {
	store := newPersistentOutboxStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.ListUninitializedSiteDigestsContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled inventory error = %v, want context.Canceled", err)
	}
}
