package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/quipthread/quipthread/models"
)

type zeroPendingDigestChannels struct {
	parent       *models.NotificationOutbox
	claim        *models.NotificationOutbox
	materialized *models.SiteDigestMaterialization
	channels     []*models.NotificationOutboxChannel
	now          time.Time
}

func seedZeroPendingDigestChannels(t *testing.T, store Store) zeroPendingDigestChannels {
	t.Helper()
	seedDigestSite(t, store, "site-skipped", nil)
	now := time.Now().UTC().Add(time.Minute)
	comment := pendingDigestComment("site-skipped", "comment-skipped", now.Add(-10*time.Minute))
	if err := store.CreatePendingCommentWithNotification(comment); err != nil {
		t.Fatal(err)
	}
	parentID := outboxIDForComment(t, store, comment.ID)
	parent, err := store.GetNotificationOutbox(parentID)
	if err != nil || parent == nil {
		t.Fatalf("read parent: %v, %#v", err, parent)
	}
	channels, err := store.EnsureNotificationOutboxChannels(parent.ID, []string{"email", "slack"})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimNotificationOutbox("digest-owner", now, time.Hour, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim parent: %v, %#v", err, claimed)
	}
	materialized, err := store.MaterializeClaimedSiteDigest(context.Background(), parent.ID, "digest-owner", claimed[0].Attempts, now)
	if err != nil || len(materialized.Comments) != 1 {
		t.Fatalf("materialize pending digest: %v, %#v", err, materialized)
	}
	if err := store.UpdateComment(&models.Comment{ID: comment.ID, Status: "approved"}); err != nil {
		t.Fatal(err)
	}
	zero, err := store.MaterializeClaimedSiteDigest(context.Background(), parent.ID, "digest-owner", claimed[0].Attempts, now)
	if err != nil || len(zero.Comments) != 0 {
		t.Fatalf("materialize zero-pending digest: %v, %#v", err, zero)
	}
	return zeroPendingDigestChannels{parent: parent, claim: claimed[0], materialized: zero, channels: channels, now: now}
}

func TestSkipNotificationOutboxChannelsReconcilesZeroPendingDigest(t *testing.T) {
	store := newPersistentOutboxStore(t)
	fixture := seedZeroPendingDigestChannels(t, store)
	claimedChannels, err := store.ClaimNotificationOutboxChannels(fixture.parent.ID, "channel-owner", fixture.now, time.Hour, 10)
	if err != nil || len(claimedChannels) != 2 {
		t.Fatalf("claim child channels: %v, %#v", err, claimedChannels)
	}
	if err := store.AckNotificationOutboxChannel(claimedChannels[0].ID, "channel-owner", claimedChannels[0].Attempts, fixture.now); err != nil {
		t.Fatal(err)
	}
	if err := store.SkipNotificationOutboxChannelsContext(context.Background(), fixture.parent.ID, fixture.claim.LeaseOwner, fixture.claim.Attempts, fixture.now); err != nil {
		t.Fatalf("skip outstanding channels: %v", err)
	}
	rows, err := store.ListNotificationOutboxChannelsContext(context.Background(), fixture.parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	sent, skipped := 0, 0
	for _, row := range rows {
		switch row.Status {
		case models.NotificationOutboxChannelSent:
			sent++
		case models.NotificationOutboxChannelSkipped:
			skipped++
		default:
			t.Fatalf("outstanding child survived skip: %#v", row)
		}
	}
	if sent != 1 || skipped != 1 {
		t.Fatalf("child terminal states = sent %d skipped %d", sent, skipped)
	}
	if err := store.FinalizeClaimedSiteDigest(context.Background(), fixture.parent.ID, fixture.claim.LeaseOwner, fixture.claim.Attempts, fixture.now); err != nil {
		t.Fatalf("finalize skipped digest: %v", err)
	}
	final, err := store.GetNotificationOutbox(fixture.parent.ID)
	if err != nil || final.Status != models.NotificationOutboxSent {
		t.Fatalf("final parent: %v, %#v", err, final)
	}
}

func TestSkippedNotificationOutboxChannelsAreTerminalAndFenced(t *testing.T) {
	store := newPersistentOutboxStore(t)
	fixture := seedZeroPendingDigestChannels(t, store)
	if err := store.SkipNotificationOutboxChannels(fixture.parent.ID, fixture.claim.LeaseOwner, fixture.claim.Attempts, fixture.now); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimNotificationOutboxChannels(fixture.parent.ID, "channel-owner", fixture.now, time.Hour, 10)
	if err != nil || len(claimed) != 0 {
		t.Fatalf("skipped channels were claimable: %v, %#v", err, claimed)
	}
	rows, err := store.ListNotificationOutboxChannels(fixture.parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Status != models.NotificationOutboxChannelSkipped {
			t.Fatalf("channel was not skipped: %#v", row)
		}
		if err := store.RetryNotificationOutboxChannel(row.ID, "channel-owner", 1, fixture.now.Add(time.Minute), fixture.now, ""); !errors.Is(err, ErrNotificationOutboxChannelNotOwned) {
			t.Fatalf("retry skipped channel = %v", err)
		}
		if err := store.AckNotificationOutboxChannel(row.ID, "channel-owner", 1, fixture.now); !errors.Is(err, ErrNotificationOutboxChannelNotOwned) {
			t.Fatalf("ack skipped channel = %v", err)
		}
	}
}

func TestSkipNotificationOutboxChannelsGenerationFenceAndContextCancellation(t *testing.T) {
	path := t.TempDir() + "/tenant.db"
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
	fixture := seedZeroPendingDigestChannels(t, first)
	if err := second.SkipNotificationOutboxChannelsContext(context.Background(), fixture.parent.ID, "wrong-owner", fixture.claim.Attempts, fixture.now); !errors.Is(err, ErrNotificationOutboxNotOwned) {
		t.Fatalf("wrong-owner skip = %v", err)
	}
	if err := second.SkipNotificationOutboxChannelsContext(context.Background(), fixture.parent.ID, fixture.claim.LeaseOwner, fixture.claim.Attempts+1, fixture.now); !errors.Is(err, ErrNotificationOutboxNotOwned) {
		t.Fatalf("wrong-generation skip = %v", err)
	}

	lock, err := first.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback() //nolint:errcheck,gosec // test cleanup
	if _, err := lock.ExecContext(context.Background(), `UPDATE sites SET theme = theme WHERE id = ?`, "site-skipped"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	started := time.Now()
	go func() {
		result <- second.SkipNotificationOutboxChannelsContext(ctx, fixture.parent.ID, fixture.claim.LeaseOwner, fixture.claim.Attempts, fixture.now)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled skip = %v", err)
		}
		if time.Since(started) > time.Second {
			t.Fatal("canceled skip did not return promptly")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled skip did not return")
	}
	if err := lock.Rollback(); err != nil {
		t.Fatal(err)
	}
	rows, err := second.ListNotificationOutboxChannelsContext(context.Background(), fixture.parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Status != models.NotificationOutboxChannelPending {
			t.Fatalf("canceled skip mutated child: %#v", row)
		}
	}
}

func TestSkippedNotificationOutboxChannelMigrationReopen(t *testing.T) {
	path := t.TempDir() + "/tenant.db"
	store, err := NewSQLiteStoreForTest(path)
	if err != nil {
		t.Fatal(err)
	}
	parent := validSiteDigestParent(t, "site-migration-skipped", time.Now().UTC(), DefaultSiteDigestIntervalSeconds)
	if err := store.EnqueueNotificationOutbox(parent); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO notification_outbox_channels (id, outbox_id, channel, status, available_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, "skipped-reopen", parent.ID, "email", models.NotificationOutboxChannelSkipped, time.Now().UTC(), time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewSQLiteStoreForTest(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close() //nolint:errcheck,gosec // test cleanup
	channel, err := reopened.GetNotificationOutboxChannel("skipped-reopen")
	if err != nil || channel == nil || channel.Status != models.NotificationOutboxChannelSkipped {
		t.Fatalf("reopened skipped channel: %v, %#v", err, channel)
	}
}
