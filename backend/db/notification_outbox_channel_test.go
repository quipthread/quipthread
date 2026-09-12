package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/quipthread/quipthread/models"
)

func newChannelOutboxStore(t *testing.T) (*SQLiteStore, string) {
	t.Helper()
	store := newPersistentOutboxStore(t)
	item := outboxItem("site-1", "site_digest", "digest-1", time.Now().UTC())
	if err := store.EnqueueNotificationOutbox(item); err != nil {
		t.Fatalf("enqueue parent outbox: %v", err)
	}
	return store, item.ID
}

func TestNotificationOutboxChannelsMigrationReopenAndIdempotentCreate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tenant.db")
	store, err := NewSQLiteStoreForTest(path)
	if err != nil {
		t.Fatal(err)
	}
	parent := outboxItem("site-1", "site_digest", "digest-migration", time.Unix(100, 0).UTC())
	if err := store.EnqueueNotificationOutbox(parent); err != nil {
		t.Fatal(err)
	}
	created, err := store.EnsureNotificationOutboxChannels(parent.ID, []string{"email", "slack", "email"})
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 2 || created[0].Channel != "email" || created[1].Channel != "slack" {
		t.Fatalf("created channels = %#v", created)
	}
	again, err := store.EnsureNotificationOutboxChannels(parent.ID, []string{"slack", "email"})
	if err != nil {
		t.Fatal(err)
	}
	createdIDs := map[string]string{created[0].Channel: created[0].ID, created[1].Channel: created[1].ID}
	if len(again) != 2 || again[0].ID != createdIDs[again[0].Channel] || again[1].ID != createdIDs[again[1].Channel] {
		t.Fatalf("idempotent channels = %#v, first=%#v", again, created)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewSQLiteStoreForTest(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close() //nolint:errcheck,gosec // test cleanup
	listed, err := reopened.ListNotificationOutboxChannels(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].Channel != "email" || listed[1].Channel != "slack" {
		t.Fatalf("reopened channels = %#v", listed)
	}
	var tableCount int
	if err := reopened.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'notification_outbox_channels'`).Scan(&tableCount); err != nil {
		t.Fatal(err)
	}
	if tableCount != 1 {
		t.Fatalf("channel table count = %d, want 1", tableCount)
	}
}

func TestNotificationOutboxChannelsPartialSuccessRetryIsolation(t *testing.T) {
	store, parentID := newChannelOutboxStore(t)
	_, err := store.EnsureNotificationOutboxChannels(parentID, []string{"email", "slack", "webhook"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(time.Second)
	claimed, err := store.ClaimNotificationOutboxChannels(parentID, "worker-a", now, time.Minute, 10)
	if err != nil || len(claimed) != 3 {
		t.Fatalf("claim channels: %v, rows=%d", err, len(claimed))
	}
	byChannel := make(map[string]*models.NotificationOutboxChannel, len(claimed))
	for _, channel := range claimed {
		byChannel[channel.Channel] = channel
	}
	if err := store.AckNotificationOutboxChannel(byChannel["email"].ID, "worker-a", byChannel["email"].Attempts, now); err != nil {
		t.Fatal(err)
	}
	retryAt := now.Add(5 * time.Second)
	if err := store.RetryNotificationOutboxChannel(byChannel["slack"].ID, "worker-a", byChannel["slack"].Attempts, retryAt, now, "slack unavailable"); err != nil {
		t.Fatal(err)
	}

	email, err := store.GetNotificationOutboxChannel(byChannel["email"].ID)
	if err != nil {
		t.Fatal(err)
	}
	slack, err := store.GetNotificationOutboxChannel(byChannel["slack"].ID)
	if err != nil {
		t.Fatal(err)
	}
	webhook, err := store.GetNotificationOutboxChannel(byChannel["webhook"].ID)
	if err != nil {
		t.Fatal(err)
	}
	if email.Status != models.NotificationOutboxChannelSent || email.LastError != "" {
		t.Fatalf("successful channel mutated by retry: %#v", email)
	}
	if slack.Status != models.NotificationOutboxChannelPending || slack.LastError != "slack unavailable" || !slack.AvailableAt.Equal(retryAt) {
		t.Fatalf("failed channel retry state = %#v", slack)
	}
	if webhook.Status != models.NotificationOutboxChannelLeased || webhook.LeaseOwner != "worker-a" {
		t.Fatalf("unrelated channel mutated by retry: %#v", webhook)
	}

	reclaimed, err := store.ClaimNotificationOutboxChannels(parentID, "worker-b", retryAt, time.Minute, 10)
	if err != nil || len(reclaimed) != 1 || reclaimed[0].Channel != "slack" {
		t.Fatalf("isolated retry claim = %v, %#v", err, reclaimed)
	}
	if err := store.AckNotificationOutboxChannel(reclaimed[0].ID, "worker-b", reclaimed[0].Attempts, retryAt); err != nil {
		t.Fatal(err)
	}
	email, _ = store.GetNotificationOutboxChannel(email.ID)
	if email.Status != models.NotificationOutboxChannelSent {
		t.Fatalf("successful channel was resent/mutated: %#v", email)
	}
}

func TestNotificationOutboxChannelsConcurrentAndStaleClaimants(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tenant.db")
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
	parent := outboxItem("site-1", "site_digest", "digest-independent", time.Now().UTC())
	if err := first.EnqueueNotificationOutbox(parent); err != nil {
		t.Fatal(err)
	}
	if _, err := first.EnsureNotificationOutboxChannels(parent.ID, []string{"email", "slack", "webhook"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(time.Second)
	type result struct {
		items []*models.NotificationOutboxChannel
		err   error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for i, candidate := range []*SQLiteStore{first, second} {
		wg.Add(1)
		go func(candidate *SQLiteStore, owner string) {
			defer wg.Done()
			items, err := candidate.ClaimNotificationOutboxChannels(parent.ID, owner, now, time.Minute, 10)
			results <- result{items: items, err: err}
		}(candidate, fmt.Sprintf("worker-%c", 'a'+i))
	}
	wg.Wait()
	close(results)
	seen := make(map[string]bool)
	var firstClaim *models.NotificationOutboxChannel
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		for _, item := range result.items {
			if seen[item.ID] {
				t.Fatalf("channel %q claimed twice", item.ID)
			}
			seen[item.ID] = true
			if firstClaim == nil {
				firstClaim = item
			}
		}
	}
	if len(seen) != 3 || firstClaim == nil {
		t.Fatalf("concurrent channel claims = %d, want 3", len(seen))
	}

}

func TestNotificationOutboxChannelSameOwnerReclaimAndExpiryFencing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tenant.db")
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
	parent := outboxItem("site-1", "site_digest", "digest-fencing", time.Now().UTC())
	if err := first.EnqueueNotificationOutbox(parent); err != nil {
		t.Fatal(err)
	}
	created, err := first.EnsureNotificationOutboxChannels(parent.ID, []string{"email"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(time.Second)
	initial, err := first.ClaimNotificationOutboxChannels(parent.ID, "same-owner", now, time.Minute, 1)
	if err != nil || len(initial) != 1 {
		t.Fatalf("initial channel claim: %v, rows=%d", err, len(initial))
	}
	firstGeneration := initial[0].Attempts
	if initial[0].LeaseUntil == nil {
		t.Fatal("initial channel claim has no lease expiry")
	}
	expiry := *initial[0].LeaseUntil
	for _, boundary := range []time.Time{expiry, expiry.Add(time.Nanosecond)} {
		if err := first.AckNotificationOutboxChannel(initial[0].ID, "same-owner", firstGeneration, boundary); !errors.Is(err, ErrNotificationOutboxChannelNotOwned) {
			t.Fatalf("ack at %v = %v", boundary, err)
		}
		if err := first.RetryNotificationOutboxChannel(initial[0].ID, "same-owner", firstGeneration, boundary.Add(time.Minute), boundary, "expired"); !errors.Is(err, ErrNotificationOutboxChannelNotOwned) {
			t.Fatalf("retry at %v = %v", boundary, err)
		}
	}
	stillLeased, err := first.GetNotificationOutboxChannel(created[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if stillLeased.Status != models.NotificationOutboxChannelLeased || stillLeased.Attempts != firstGeneration {
		t.Fatalf("expired transition mutated channel: %#v", stillLeased)
	}

	reclaimed, err := second.ClaimNotificationOutboxChannels(parent.ID, "same-owner", expiry.Add(time.Nanosecond), time.Minute, 1)
	if err != nil || len(reclaimed) != 1 {
		t.Fatalf("same-owner reclaim: %v, rows=%d", err, len(reclaimed))
	}
	if reclaimed[0].ID != initial[0].ID || reclaimed[0].Attempts != firstGeneration+1 {
		t.Fatalf("reclaim = %#v, want same row generation %d", reclaimed[0], firstGeneration+1)
	}
	currentExpiry := *reclaimed[0].LeaseUntil
	if err := first.AckNotificationOutboxChannel(reclaimed[0].ID, "same-owner", firstGeneration, expiry.Add(time.Second)); !errors.Is(err, ErrNotificationOutboxChannelNotOwned) {
		t.Fatalf("stale-generation ack = %v", err)
	}
	if err := first.RetryNotificationOutboxChannel(reclaimed[0].ID, "same-owner", firstGeneration, expiry.Add(2*time.Minute), expiry.Add(time.Second), "stale"); !errors.Is(err, ErrNotificationOutboxChannelNotOwned) {
		t.Fatalf("stale-generation retry = %v", err)
	}
	if err := second.AckNotificationOutboxChannel(reclaimed[0].ID, "same-owner", reclaimed[0].Attempts, currentExpiry.Add(-time.Nanosecond)); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationOutboxChannelLeaseBoundaryUsesCanonicalPrecision(t *testing.T) {
	store := newPersistentOutboxStore(t)
	base := time.Date(2030, time.January, 2, 3, 4, 5, 123456789, time.UTC)
	parent := outboxItem("site-precision", "site_digest", "digest-precision", base.Add(-time.Second))
	if err := store.EnqueueNotificationOutbox(parent); err != nil {
		t.Fatal(err)
	}
	channels, err := store.EnsureNotificationOutboxChannels(parent.ID, []string{"email"})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimNotificationOutboxChannels(parent.ID, "precision-owner", base, time.Minute, 1)
	if err != nil || len(claimed) != 1 || claimed[0].LeaseUntil == nil {
		t.Fatalf("precision lease claim: %v, %#v", err, claimed)
	}
	expectedExpiry := base.Add(time.Minute).Truncate(notificationOutboxLeaseTimePrecision)
	if !claimed[0].LeaseUntil.Equal(expectedExpiry) {
		t.Fatalf("canonical lease expiry = %v, want %v", *claimed[0].LeaseUntil, expectedExpiry)
	}
	if err := store.AckNotificationOutboxChannel(channels[0].ID, "precision-owner", claimed[0].Attempts, expectedExpiry); !errors.Is(err, ErrNotificationOutboxChannelNotOwned) {
		t.Fatalf("ack at canonical expiry = %v, want not owned", err)
	}
	if err := store.AckNotificationOutboxChannel(channels[0].ID, "precision-owner", claimed[0].Attempts, expectedExpiry.Add(-time.Nanosecond)); err != nil {
		t.Fatalf("ack immediately before canonical expiry: %v", err)
	}
}

func TestNotificationOutboxChannelsRejectInvalidDataAndPreserveParentIntegrity(t *testing.T) {
	store, parentID := newChannelOutboxStore(t)
	invalid := [][]string{nil, {" "}, {string(make([]byte, NotificationOutboxMaxKeyBytes+1))}}
	for i, channels := range invalid {
		if _, err := store.EnsureNotificationOutboxChannels(parentID, channels); !errors.Is(err, ErrInvalidNotificationOutboxChannel) {
			t.Errorf("invalid channels %d error = %v", i, err)
		}
	}
	tooMany := make([]string, NotificationOutboxMaxChannelCount+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("channel-%d", i)
	}
	if _, err := store.EnsureNotificationOutboxChannels(parentID, tooMany); !errors.Is(err, ErrInvalidNotificationOutbox) {
		t.Errorf("too many channels error = %v", err)
	}
	if _, err := store.EnsureNotificationOutboxChannels("missing-parent", []string{"email"}); !errors.Is(err, ErrNotificationOutboxChannelNotFound) {
		t.Errorf("missing parent error = %v", err)
	}
	if _, err := store.ClaimNotificationOutboxChannels(parentID, "worker", time.Now(), time.Minute, 0); !errors.Is(err, ErrInvalidNotificationOutbox) {
		t.Errorf("zero channel claim limit error = %v", err)
	}

	channels, err := store.EnsureNotificationOutboxChannels(parentID, []string{"email"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AckNotificationOutboxChannel(channels[0].ID, "worker", 0, time.Now()); !errors.Is(err, ErrInvalidNotificationOutbox) {
		t.Errorf("zero generation error = %v", err)
	}
	if err := store.RetryNotificationOutboxChannel(channels[0].ID, "worker", 1, time.Now(), time.Now(), ""); !errors.Is(err, ErrInvalidNotificationOutbox) {
		t.Errorf("invalid retry time error = %v", err)
	}
	tooLong := string(make([]byte, NotificationOutboxMaxErrorBytes+1))
	future := time.Now().Add(time.Minute)
	if err := store.RetryNotificationOutboxChannel(channels[0].ID, "worker", 1, future, time.Now(), tooLong); !errors.Is(err, ErrInvalidNotificationOutbox) {
		t.Errorf("oversized channel error error = %v", err)
	}

	if _, err := store.db.Exec(`DELETE FROM notification_outbox WHERE id = ?`, parentID); err != nil {
		t.Fatal(err)
	}
	var childCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM notification_outbox_channels WHERE outbox_id = ?`, parentID).Scan(&childCount); err != nil {
		t.Fatal(err)
	}
	if childCount != 0 {
		t.Fatalf("cascade left %d child rows", childCount)
	}
	if _, err := store.db.Exec(`INSERT INTO notification_outbox_channels (id, outbox_id, channel, available_at, created_at, updated_at) VALUES ('orphan', ?, 'email', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, parentID); err == nil {
		t.Fatal("orphan channel insert unexpectedly succeeded")
	}
}

func TestNotificationOutboxChannelsIndependentConnectionsEnforceParentIntegrity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tenant.db")
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
	parent := outboxItem("site-1", "site_digest", "digest-integrity", time.Now().UTC())
	if err := first.EnqueueNotificationOutbox(parent); err != nil {
		t.Fatal(err)
	}
	if _, err := first.EnsureNotificationOutboxChannels(parent.ID, []string{"email"}); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	firstConn, err := first.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer firstConn.Close()
	secondConn, err := second.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer secondConn.Close()
	if _, err := firstConn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	if _, err := secondConn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	for name, conn := range map[string]*sql.Conn{"first": firstConn, "second": secondConn} {
		if _, err := conn.ExecContext(ctx, `INSERT INTO notification_outbox_channels (id, outbox_id, channel, available_at, created_at, updated_at) VALUES (?, 'missing-parent', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, "orphan-"+name, "email"); err == nil {
			t.Fatalf("%s connection inserted orphan channel", name)
		}
	}
	if _, err := secondConn.ExecContext(ctx, `UPDATE notification_outbox SET id = ? WHERE id = ?`, "renamed-parent", parent.ID); err == nil {
		t.Fatal("independent connection renamed parent with channel children")
	}
	var parentCount int
	if err := firstConn.QueryRowContext(ctx, `SELECT COUNT(*) FROM notification_outbox WHERE id = ?`, parent.ID).Scan(&parentCount); err != nil {
		t.Fatal(err)
	}
	if parentCount != 1 {
		t.Fatalf("guarded parent row count = %d, want 1", parentCount)
	}
	if _, err := secondConn.ExecContext(ctx, `DELETE FROM notification_outbox WHERE id = ?`, parent.ID); err != nil {
		t.Fatal(err)
	}
	var childCount int
	if err := firstConn.QueryRowContext(ctx, `SELECT COUNT(*) FROM notification_outbox_channels WHERE outbox_id = ?`, parent.ID).Scan(&childCount); err != nil {
		t.Fatal(err)
	}
	if childCount != 0 {
		t.Fatalf("independent connection cascade left %d child rows", childCount)
	}
}
