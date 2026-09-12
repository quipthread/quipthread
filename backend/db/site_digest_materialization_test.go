package db

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/quipthread/quipthread/models"
)

func seedDigestSite(t *testing.T, store Store, siteID string, interval *int) {
	t.Helper()
	if err := store.CreateSite(&models.Site{ID: siteID, OwnerID: "owner", Domain: siteID + ".example"}); err != nil {
		t.Fatal(err)
	}
	if interval != nil {
		if err := store.UpdateSite(&models.Site{ID: siteID, Theme: "auto", NotifyInterval: interval}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.UpsertUser(&models.User{ID: "digest-user", DisplayName: "Digest User", Role: "commenter"}); err != nil {
		t.Fatal(err)
	}
}

func pendingDigestComment(siteID, id string, createdAt time.Time) *models.Comment {
	return &models.Comment{ID: id, SiteID: siteID, PageID: "/digest", UserID: "digest-user", Content: id, Status: "pending", CreatedAt: createdAt}
}

func outboxIDForComment(t *testing.T, store Store, commentID string) string {
	t.Helper()
	s := store.(*SQLiteStore)
	var outboxID string
	if err := s.db.QueryRow(`SELECT outbox_id FROM notification_outbox_comments WHERE comment_id = ?`, commentID).Scan(&outboxID); err != nil {
		t.Fatal(err)
	}
	return outboxID
}

func validSiteDigestParent(t *testing.T, siteID string, createdAt time.Time, interval int64) *models.NotificationOutbox {
	t.Helper()
	windowStart, availableAt, err := siteDigestWindow(createdAt, interval)
	if err != nil {
		t.Fatal(err)
	}
	return &models.NotificationOutbox{
		SiteID:       siteID,
		Kind:         NotificationOutboxKindSiteDigest,
		AggregateKey: fmt.Sprintf("site-digest:%d:%d", interval, windowStart),
		AvailableAt:  availableAt,
		CreatedAt:    createdAt,
	}
}

func TestPendingDigestMembershipIsAtomicCanonicalAndIntervalScoped(t *testing.T) {
	store := newPersistentOutboxStore(t)
	interval := 300
	seedDigestSite(t, store, "site-1", &interval)
	base := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	for _, comment := range []*models.Comment{
		pendingDigestComment("site-1", "comment-1", base.Add(time.Minute)),
		pendingDigestComment("site-1", "comment-2", base.Add(2*time.Minute)),
	} {
		if err := store.CreatePendingCommentWithNotification(comment); err != nil {
			t.Fatal(err)
		}
	}
	var outboxCount, membershipCount int
	s := store
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM notification_outbox WHERE site_id = ?`, "site-1").Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM notification_outbox_comments`).Scan(&membershipCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount != 1 || membershipCount != 2 {
		t.Fatalf("same-window digest rows = outbox %d membership %d, want 1 and 2", outboxCount, membershipCount)
	}
	var membershipVersion int
	if err := s.db.QueryRow(`SELECT digest_membership_version FROM notification_outbox LIMIT 1`).Scan(&membershipVersion); err != nil {
		t.Fatal(err)
	}
	if membershipVersion != 1 {
		t.Fatalf("new digest membership version = %d, want 1", membershipVersion)
	}

	interval = 60
	if err := store.UpdateSite(&models.Site{ID: "site-1", Theme: "auto", NotifyInterval: &interval}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreatePendingCommentWithNotification(pendingDigestComment("site-1", "comment-3", base.Add(time.Minute))); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM notification_outbox_comments`).Scan(&membershipCount); err != nil {
		t.Fatal(err)
	}
	if membershipCount != 3 {
		t.Fatalf("interval change membership count = %d, want 3", membershipCount)
	}
	for _, commentID := range []string{"comment-1", "comment-2", "comment-3"} {
		var count int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM notification_outbox_comments WHERE comment_id = ?`, commentID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("comment %q membership count = %d, want 1", commentID, count)
		}
	}
	if _, err := s.db.Exec(`UPDATE comments SET id = ? WHERE id = ?`, "renamed-comment", "comment-1"); err == nil {
		t.Fatal("comment with digest membership was renamed")
	}

	secondParent := outboxItem("site-1", NotificationOutboxKindSiteDigest, "manual-second-parent", base)
	if err := store.EnqueueNotificationOutbox(secondParent); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO notification_outbox_comments (outbox_id, comment_id) VALUES (?, ?)`, secondParent.ID, "comment-1"); err == nil {
		t.Fatal("comment was assigned to two digest parents")
	}
}

func TestPendingDigestMembershipBoundariesAndTenantIsolation(t *testing.T) {
	interval := 300
	firstPath := filepath.Join(t.TempDir(), "first.db")
	secondPath := filepath.Join(t.TempDir(), "second.db")
	first, err := NewSQLiteStoreForTest(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close() //nolint:errcheck,gosec // test cleanup
	second, err := NewSQLiteStoreForTest(secondPath)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close() //nolint:errcheck,gosec // test cleanup
	seedDigestSite(t, first, "same-site", &interval)
	seedDigestSite(t, second, "same-site", &interval)
	boundary := time.Date(2026, 8, 27, 12, 5, 0, 0, time.UTC)
	for _, store := range []Store{first, second} {
		if err := store.CreatePendingCommentWithNotification(pendingDigestComment("same-site", "same-comment-id", boundary)); err != nil {
			t.Fatal(err)
		}
	}
	for _, store := range []*SQLiteStore{first, second} {
		var count int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM notification_outbox_comments WHERE comment_id = ?`, "same-comment-id").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("tenant-local colliding membership count = %d, want 1", count)
		}
	}

	if err := first.CreatePendingCommentWithNotification(pendingDigestComment("same-site", "boundary-before", boundary.Add(-time.Nanosecond))); err != nil {
		t.Fatal(err)
	}
	if err := first.CreatePendingCommentWithNotification(pendingDigestComment("same-site", "boundary-at", boundary)); err != nil {
		t.Fatal(err)
	}
	var boundaryOutboxes int
	if err := first.db.QueryRow(`SELECT COUNT(DISTINCT outbox_id) FROM notification_outbox_comments WHERE comment_id IN (?, ?)`, "boundary-before", "boundary-at").Scan(&boundaryOutboxes); err != nil {
		t.Fatal(err)
	}
	if boundaryOutboxes != 2 {
		t.Fatalf("boundary membership outboxes = %d, want 2", boundaryOutboxes)
	}
}

func TestSiteDigestAggregateKeyValidation(t *testing.T) {
	valid, err := parseSiteDigestAggregateKey("site-digest:300:1800")
	if err != nil || valid.interval != 300 || valid.windowStart != 1800 || !valid.windowEnd.Equal(time.Unix(2100, 0).UTC()) {
		t.Fatalf("valid aggregate = %#v, err=%v", valid, err)
	}
	for _, key := range []string{
		"",
		"site-digest:0300:1800",
		"site-digest:300:1801",
		"site-digest:59:1800",
		"site-digest:300:1800:extra",
		"other:300:1800",
	} {
		if _, err := parseSiteDigestAggregateKey(key); !errors.Is(err, ErrDigestAggregateMalformed) {
			t.Errorf("aggregate %q error = %v, want ErrDigestAggregateMalformed", key, err)
		}
	}
}

func TestMaterializeRejectsAggregateAvailableAtMismatch(t *testing.T) {
	store := newPersistentOutboxStore(t)
	seedDigestSite(t, store, "site-1", nil)
	comment := pendingDigestComment("site-1", "available-mismatch", time.Now().UTC().Add(-10*time.Minute))
	if err := store.CreatePendingCommentWithNotification(comment); err != nil {
		t.Fatal(err)
	}
	parentID := outboxIDForComment(t, store, comment.ID)
	parent, err := store.GetNotificationOutbox(parentID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE notification_outbox SET available_at = ? WHERE id = ?`, parent.AvailableAt.Add(time.Second), parentID); err != nil {
		t.Fatal(err)
	}
	claimNow := time.Now().UTC().Add(time.Hour)
	claimed, err := store.ClaimNotificationOutbox("aggregate-checker", claimNow, time.Hour, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim mismatched parent: %v, %#v", err, claimed)
	}
	if _, err := store.MaterializeClaimedSiteDigest(context.Background(), parentID, "aggregate-checker", claimed[0].Attempts, claimNow); !errors.Is(err, ErrDigestAggregateMalformed) {
		t.Fatalf("available_at mismatch error = %v", err)
	}
}

func TestMaterializeClaimedSiteDigestRequiresClaimAndMembership(t *testing.T) {
	store := newPersistentOutboxStore(t)
	seedDigestSite(t, store, "site-1", nil)
	created := time.Now().UTC().Add(-10 * time.Minute).Truncate(5 * time.Minute).Add(time.Minute)
	earlierComment := pendingDigestComment("site-1", "mapped-comment-earlier", created)
	comment := pendingDigestComment("site-1", "mapped-comment", created.Add(time.Minute))
	if err := store.CreatePendingCommentWithNotification(earlierComment); err != nil {
		t.Fatal(err)
	}
	if err := store.CreatePendingCommentWithNotification(comment); err != nil {
		t.Fatal(err)
	}
	parentID := outboxIDForComment(t, store, comment.ID)
	claimNow := time.Now().UTC().Add(time.Hour)
	claimed, err := store.ClaimNotificationOutbox("materializer", claimNow, time.Hour, 1)
	if err != nil || len(claimed) != 1 || claimed[0].ID != parentID {
		t.Fatalf("claim parent: %v, %#v", err, claimed)
	}
	materialized, err := store.MaterializeClaimedSiteDigest(context.Background(), parentID, "materializer", claimed[0].Attempts, claimNow)
	if err != nil {
		t.Fatal(err)
	}
	if materialized.Site == nil || materialized.Site.SSOSecret != nil || len(materialized.Comments) != 2 || materialized.Comments[0].ID != earlierComment.ID || materialized.Comments[1].ID != comment.ID || materialized.Comments[0].AuthorName != "Digest User" {
		t.Fatalf("materialized digest = %#v", materialized)
	}
	if _, err := store.db.Exec(`UPDATE comments SET status = 'approved' WHERE id = ?`, earlierComment.ID); err != nil {
		t.Fatal(err)
	}
	pendingOnly, err := store.MaterializeClaimedSiteDigest(context.Background(), parentID, "materializer", claimed[0].Attempts, claimNow)
	if err != nil || len(pendingOnly.Comments) != 1 || pendingOnly.Comments[0].ID != comment.ID {
		t.Fatalf("approved member omission = %v, %#v", err, pendingOnly)
	}
	if _, err := store.db.Exec(`UPDATE comments SET status = 'approved' WHERE id = ?`, comment.ID); err != nil {
		t.Fatal(err)
	}
	emptyPending, err := store.MaterializeClaimedSiteDigest(context.Background(), parentID, "materializer", claimed[0].Attempts, claimNow)
	if err != nil || len(emptyPending.Comments) != 0 {
		t.Fatalf("zero pending materialization = %v, %#v", err, emptyPending)
	}
	if _, err := store.MaterializeClaimedSiteDigest(context.Background(), parentID, "materializer", claimed[0].Attempts, *claimed[0].LeaseUntil); !errors.Is(err, ErrNotificationOutboxNotOwned) {
		t.Fatalf("stale materialization = %v", err)
	}

	missingParent := validSiteDigestParent(t, "site-1", created.Add(-time.Hour), DefaultSiteDigestIntervalSeconds)
	if err := store.EnqueueNotificationOutbox(missingParent); err != nil {
		t.Fatal(err)
	}
	legacyCandidate := pendingDigestComment("site-1", "legacy-coalesce-candidate", created.Add(-time.Hour))
	if err := store.CreatePendingCommentWithNotification(legacyCandidate); !errors.Is(err, ErrDigestMembershipUninitialized) {
		t.Fatalf("legacy coalescing error = %v, want ErrDigestMembershipUninitialized", err)
	}
	if got := countCommentsByID(t, store, legacyCandidate.ID); got != 0 {
		t.Fatalf("legacy coalescing left %d candidate comments", got)
	}
	missingClaim, err := store.ClaimNotificationOutbox("materializer-2", claimNow, time.Hour, 1)
	if err != nil || len(missingClaim) != 1 {
		t.Fatalf("claim missing-membership parent: %v, %#v", err, missingClaim)
	}
	if _, err := store.MaterializeClaimedSiteDigest(context.Background(), missingParent.ID, "materializer-2", missingClaim[0].Attempts, claimNow); !errors.Is(err, ErrDigestMembershipUninitialized) {
		t.Fatalf("missing membership error = %v", err)
	}
	if err := store.FinalizeClaimedSiteDigest(context.Background(), missingParent.ID, "materializer-2", missingClaim[0].Attempts, claimNow); !errors.Is(err, ErrDigestMembershipUninitialized) {
		t.Fatalf("legacy finalization error = %v", err)
	}
	uninitialized, err := store.ListUninitializedSiteDigests()
	if err != nil {
		t.Fatal(err)
	}
	foundUninitialized := false
	for _, candidate := range uninitialized {
		if candidate.ID == missingParent.ID && candidate.DigestMembershipVersion == 0 {
			foundUninitialized = true
		}
	}
	if !foundUninitialized {
		t.Fatalf("reconciliation inventory omitted legacy digest %q: %#v", missingParent.ID, uninitialized)
	}

	malformedParent := validSiteDigestParent(t, "site-1", created.Add(-2*time.Hour), DefaultSiteDigestIntervalSeconds)
	if err := store.EnqueueNotificationOutbox(malformedParent); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE notification_outbox SET digest_membership_version = 1 WHERE id = ?`, malformedParent.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateComment(&models.Comment{ID: "approved-mapped", SiteID: "site-1", PageID: "/digest", UserID: "digest-user", Content: "approved", Status: "approved", CreatedAt: created}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO notification_outbox_comments (outbox_id, comment_id) VALUES (?, ?)`, malformedParent.ID, "approved-mapped"); err != nil {
		t.Fatal(err)
	}
	malformedClaim, err := store.ClaimNotificationOutbox("materializer-3", claimNow, time.Hour, 1)
	if err != nil || len(malformedClaim) != 1 {
		t.Fatalf("claim malformed-membership parent: %v, %#v", err, malformedClaim)
	}
	if _, err := store.MaterializeClaimedSiteDigest(context.Background(), malformedParent.ID, "materializer-3", malformedClaim[0].Attempts, claimNow); !errors.Is(err, ErrDigestMembershipMalformed) {
		t.Fatalf("malformed membership error = %v", err)
	}

	emptyParent := validSiteDigestParent(t, "site-1", time.Now().UTC().Add(2*time.Hour), DefaultSiteDigestIntervalSeconds)
	if err := store.EnqueueNotificationOutbox(emptyParent); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE notification_outbox SET digest_membership_version = 1 WHERE id = ?`, emptyParent.ID); err != nil {
		t.Fatal(err)
	}
	emptyClaimNow := time.Now().UTC().Add(4 * time.Hour)
	emptyClaims, err := store.ClaimNotificationOutbox("materializer-4", emptyClaimNow, time.Hour, 10)
	if err != nil {
		t.Fatal(err)
	}
	var emptyClaim *models.NotificationOutbox
	for _, candidate := range emptyClaims {
		if candidate.ID == emptyParent.ID {
			emptyClaim = candidate
		}
	}
	if emptyClaim == nil {
		t.Fatalf("initialized empty digest was not claimed: %#v", emptyClaims)
	}
	emptyMaterialization, err := store.MaterializeClaimedSiteDigest(context.Background(), emptyParent.ID, "materializer-4", emptyClaim.Attempts, emptyClaimNow)
	if err != nil || emptyMaterialization == nil || len(emptyMaterialization.Comments) != 0 {
		t.Fatalf("initialized empty digest = %v, %#v", err, emptyMaterialization)
	}
	if err := store.FinalizeClaimedSiteDigest(context.Background(), emptyParent.ID, "materializer-4", emptyClaim.Attempts, emptyClaimNow); err != nil {
		t.Fatalf("initialized empty digest no-op finalization: %v", err)
	}
	emptyFinal, err := store.GetNotificationOutbox(emptyParent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if emptyFinal.Status != models.NotificationOutboxSent {
		t.Fatalf("empty digest no-op status = %#v", emptyFinal)
	}
}

func TestFinalizeClaimedSiteDigestRequiresAllChannelsSent(t *testing.T) {
	store := newPersistentOutboxStore(t)
	seedDigestSite(t, store, "site-1", nil)
	seedComment := pendingDigestComment("site-1", "finalize-membership", time.Now().UTC().Add(-10*time.Minute))
	if err := store.CreatePendingCommentWithNotification(seedComment); err != nil {
		t.Fatal(err)
	}
	parentID := outboxIDForComment(t, store, seedComment.ID)
	parent, err := store.GetNotificationOutbox(parentID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(time.Second)
	claimedParent, err := store.ClaimNotificationOutbox("parent-worker", now, time.Hour, 1)
	if err != nil || len(claimedParent) != 1 {
		t.Fatalf("claim parent: %v, %#v", err, claimedParent)
	}
	parentGeneration := claimedParent[0].Attempts
	if err := store.FinalizeClaimedSiteDigest(context.Background(), parent.ID, "parent-worker", parentGeneration, now); !errors.Is(err, ErrDigestChannelsNotComplete) {
		t.Fatalf("pending-member no-channel finalization = %v", err)
	}
	if err := store.AckNotificationOutbox(parent.ID, "parent-worker", parentGeneration, now); !errors.Is(err, ErrDigestChannelsNotComplete) {
		t.Fatalf("generic parent ack bypassed channel invariant: %v", err)
	}
	if _, err := store.EnsureNotificationOutboxChannels(parent.ID, []string{"email", "slack"}); err != nil {
		t.Fatal(err)
	}
	claimedChannels, err := store.ClaimNotificationOutboxChannels(parent.ID, "channel-worker", now, time.Minute, 10)
	if err != nil || len(claimedChannels) != 2 {
		t.Fatalf("claim child channels: %v, %#v", err, claimedChannels)
	}
	if err := store.FinalizeClaimedSiteDigest(context.Background(), parent.ID, "parent-worker", parentGeneration, now); !errors.Is(err, ErrDigestChannelsNotComplete) {
		t.Fatalf("leased-channel finalization = %v", err)
	}
	for _, channel := range claimedChannels {
		if err := store.AckNotificationOutboxChannel(channel.ID, "channel-worker", channel.Attempts, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.FinalizeClaimedSiteDigest(context.Background(), parent.ID, "parent-worker", parentGeneration, now); err != nil {
		t.Fatal(err)
	}
	final, err := store.GetNotificationOutbox(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != models.NotificationOutboxSent || final.LeaseOwner != "" || final.LeaseUntil != nil {
		t.Fatalf("finalized parent = %#v", final)
	}
	if _, err := store.EnsureNotificationOutboxChannels(parent.ID, []string{"webhook"}); err == nil {
		t.Fatal("channel creation on sent digest unexpectedly succeeded")
	}
	otherParent := validSiteDigestParent(t, "site-1", time.Now().UTC().Add(time.Hour), DefaultSiteDigestIntervalSeconds)
	if err := store.EnqueueNotificationOutbox(otherParent); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE notification_outbox_channels SET outbox_id = ? WHERE id = ?`, otherParent.ID, claimedChannels[0].ID); err == nil {
		t.Fatal("sent digest channel was reparented")
	}
}

func TestConcurrentDigestFinalizationAndChannelInitializationPreserveInvariant(t *testing.T) {
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
	seedDigestSite(t, first, "site-1", nil)
	comment := pendingDigestComment("site-1", "race-membership", time.Now().UTC().Add(-10*time.Minute))
	if err := first.CreatePendingCommentWithNotification(comment); err != nil {
		t.Fatal(err)
	}
	parentID := outboxIDForComment(t, first, comment.ID)
	if _, err := first.EnsureNotificationOutboxChannels(parentID, []string{"email"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(time.Second)
	parentClaim, err := first.ClaimNotificationOutbox("race-parent", now, time.Hour, 1)
	if err != nil || len(parentClaim) != 1 {
		t.Fatalf("claim race parent: %v, %#v", err, parentClaim)
	}
	childClaim, err := first.ClaimNotificationOutboxChannels(parentID, "race-channel", now, time.Minute, 1)
	if err != nil || len(childClaim) != 1 {
		t.Fatalf("claim race child: %v, %#v", err, childClaim)
	}
	if err := first.AckNotificationOutboxChannel(childClaim[0].ID, "race-channel", childClaim[0].Attempts, now); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		results <- first.FinalizeClaimedSiteDigest(context.Background(), parentID, "race-parent", parentClaim[0].Attempts, now)
	}()
	go func() {
		defer wg.Done()
		<-start
		_, ensureErr := second.EnsureNotificationOutboxChannels(parentID, []string{"slack"})
		results <- ensureErr
	}()
	close(start)
	wg.Wait()
	close(results)
	for range results {
		// One operation may legitimately lose the race; the invariant is
		// checked from the committed parent and child state below.
	}

	parent, err := first.GetNotificationOutbox(parentID)
	if err != nil {
		t.Fatal(err)
	}
	children, err := first.ListNotificationOutboxChannels(parentID)
	if err != nil {
		t.Fatal(err)
	}
	if parent.Status == models.NotificationOutboxSent {
		for _, child := range children {
			if child.Status != models.NotificationOutboxChannelSent {
				t.Fatalf("sent parent has unsent child after concurrent initialization: %#v", child)
			}
		}
	}
}

func TestMaterializeAndFinalizeRejectInvalidInputs(t *testing.T) {
	store := newPersistentOutboxStore(t)
	if _, err := store.MaterializeClaimedSiteDigest(context.Background(), "", "owner", 1, time.Now()); !errors.Is(err, ErrInvalidNotificationOutbox) {
		t.Errorf("invalid materialization ID = %v", err)
	}
	if _, err := store.MaterializeClaimedSiteDigest(context.Background(), "id", "owner", 1, time.Time{}); !errors.Is(err, ErrInvalidNotificationOutbox) {
		t.Errorf("invalid materialization time = %v", err)
	}
	if err := store.FinalizeClaimedSiteDigest(context.Background(), "id", "owner", 1, time.Time{}); !errors.Is(err, ErrInvalidNotificationOutbox) {
		t.Errorf("invalid finalization time = %v", err)
	}
	if _, err := store.MaterializeClaimedSiteDigest(nil, "id", "owner", 1, time.Now()); !errors.Is(err, ErrInvalidSiteDigestParent) { //nolint:staticcheck // Intentionally verify that a nil context is rejected.
		t.Errorf("nil materialization context = %v", err)
	}
	if err := store.FinalizeClaimedSiteDigest(nil, "id", "owner", 1, time.Now()); !errors.Is(err, ErrInvalidSiteDigestParent) { //nolint:staticcheck // Intentionally verify that a nil context is rejected.
		t.Errorf("nil finalization context = %v", err)
	}
}
