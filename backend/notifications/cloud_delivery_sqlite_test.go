package notifications

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/quipthread/quipthread/cloud"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/models"
)

type sqliteApprovalLocatorSink struct {
	mu      sync.Mutex
	created []*models.ApprovalTokenLocator
}

func (s *sqliteApprovalLocatorSink) GetApprovalTokenContext(context.Context, string) (*models.ApprovalTokenLocator, error) {
	return nil, nil
}

func (s *sqliteApprovalLocatorSink) CreateApprovalTokenContext(_ context.Context, locator *models.ApprovalTokenLocator) error {
	s.mu.Lock()
	s.created = append(s.created, locator)
	s.mu.Unlock()
	return nil
}

func TestRunCloudTenantDeliveryPassRealSQLitePartialFailureReclaimsOnlyFailedChannel(t *testing.T) {
	store, err := db.NewSQLiteStoreForTest(filepath.Join(t.TempDir(), "tenant.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close() //nolint:errcheck,gosec // test cleanup

	if err := store.CreateSite(&models.Site{ID: "site-real", OwnerID: "owner-real", Domain: "real.example.test"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertUser(&models.User{ID: "commenter-real", DisplayName: "Commenter", Role: "commenter"}); err != nil {
		t.Fatal(err)
	}
	createdAt := time.Unix(700, 0).UTC()
	comment := &models.Comment{ID: "comment-real", SiteID: "site-real", PageID: "/real", UserID: "commenter-real", Content: "tenant-private-content", Status: "pending", CreatedAt: createdAt}
	if err := store.CreatePendingCommentWithNotification(comment); err != nil {
		t.Fatal(err)
	}
	parents, err := store.ListUninitializedSiteDigests()
	if err != nil || len(parents) != 0 {
		t.Fatalf("initialized digest inventory = %d, err=%v", len(parents), err)
	}

	now := time.Now().UTC().Add(time.Minute)
	source := &deliverySource{pages: []struct {
		locators []*cloud.AccountLocator
		next     string
	}{{locators: []*cloud.AccountLocator{{ID: "account-real"}}}}}
	sink := &sqliteApprovalLocatorSink{}
	sender := &deliverySender{err: map[string]error{ChannelSlack: errors.New("raw webhook URL credential recipient content")}}
	var releases int
	opener := func(context.Context, cloud.AccountLocator) (CloudTenantDeliveryLease, error) {
		return NewCloudTenantDeliveryStoreLease(store, func() error { releases++; return nil }), nil
	}
	opts := deliveryOptions(&now, sender, ChannelEmail, ChannelSlack)
	opts.ApprovalTokenHMACKey = strings.Repeat("h", 32)
	opts.ApprovalTokenLocatorStore = sink
	if _, err := RunCloudTenantDeliveryPass(context.Background(), source, opener, opts); err != nil {
		t.Fatal(err)
	}

	parent, err := store.ListUninitializedSiteDigests()
	if err != nil || len(parent) != 0 {
		t.Fatalf("digest inventory after delivery = %d, err=%v", len(parent), err)
	}
	rows, err := store.ListNotificationOutboxChannelsContext(context.Background(), findRealParentID(t, store))
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Channel == ChannelEmail && row.Status != models.NotificationOutboxChannelSent {
			t.Fatalf("email child after first pass = %+v", row)
		}
		if row.Channel == ChannelSlack && row.Status != models.NotificationOutboxChannelPending {
			t.Fatalf("slack child after first pass = %+v", row)
		}
		if strings.Contains(row.LastError, "raw") || strings.Contains(row.LastError, "credential") || strings.Contains(row.LastError, "recipient") {
			t.Fatalf("unsafe persisted child error = %q", row.LastError)
		}
	}
	if len(sink.created) != 1 || sink.created[0].TokenHash == "" || strings.Contains(sink.created[0].TokenHash, "tenant-private-content") {
		t.Fatalf("approval locator projection = %+v", sink.created)
	}

	now = now.Add(2 * time.Minute)
	sender.err = map[string]error{}
	if _, err := RunCloudTenantDeliveryPass(context.Background(), source, opener, deliveryOptionsWithSink(&now, sender, sink)); err != nil {
		t.Fatal(err)
	}
	if sender.callCount(ChannelEmail) != 1 || sender.callCount(ChannelSlack) != 2 || releases != 2 {
		debugParent, _ := store.GetNotificationOutbox(findRealParentID(t, store))
		debugChildren, _ := store.ListNotificationOutboxChannelsContext(context.Background(), debugParent.ID)
		t.Fatalf("calls email=%d slack=%d releases=%d parent=%+v children=%+v", sender.callCount(ChannelEmail), sender.callCount(ChannelSlack), releases, debugParent, debugChildren)
	}
	finalParent, err := store.GetNotificationOutbox(findRealParentID(t, store))
	if err != nil || finalParent.Status != models.NotificationOutboxSent {
		t.Fatalf("final parent = %+v, err=%v", finalParent, err)
	}
}

func TestRunCloudTenantDeliveryPassRealSQLiteSkipsOutstandingZeroPendingChildren(t *testing.T) {
	store, err := db.NewSQLiteStoreForTest(filepath.Join(t.TempDir(), "tenant.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close() //nolint:errcheck,gosec // test cleanup
	if err := store.CreateSite(&models.Site{ID: "site-zero-real", OwnerID: "owner-zero", Domain: "zero.example.test"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertUser(&models.User{ID: "commenter-zero", DisplayName: "Commenter", Role: "commenter"}); err != nil {
		t.Fatal(err)
	}
	comment := &models.Comment{ID: "comment-zero-real", SiteID: "site-zero-real", PageID: "/zero", UserID: "commenter-zero", Content: "no-longer-pending", Status: "pending", CreatedAt: time.Unix(700, 0).UTC()}
	if err := store.CreatePendingCommentWithNotification(comment); err != nil {
		t.Fatal(err)
	}
	parent, err := store.GetNotificationOutboxByKey("site-zero-real", db.NotificationOutboxKindSiteDigest, "site-digest:300:600")
	if err != nil || parent == nil {
		t.Fatalf("read zero-pending parent: %v, %#v", err, parent)
	}
	if _, err := store.EnsureNotificationOutboxChannelsContext(context.Background(), parent.ID, []string{ChannelEmail, ChannelSlack}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateComment(&models.Comment{ID: comment.ID, Status: "approved"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(time.Minute)
	source := &deliverySource{pages: []struct {
		locators []*cloud.AccountLocator
		next     string
	}{{locators: []*cloud.AccountLocator{{ID: "account-zero-real"}}}}}
	sender := &deliverySender{}
	opener := func(context.Context, cloud.AccountLocator) (CloudTenantDeliveryLease, error) {
		return NewCloudTenantDeliveryStoreLease(store, nil), nil
	}
	if _, err := RunCloudTenantDeliveryPass(context.Background(), source, opener, deliveryOptions(&now, sender, ChannelEmail, ChannelSlack)); err != nil {
		t.Fatal(err)
	}
	rows, err := store.ListNotificationOutboxChannelsContext(context.Background(), parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Status != models.NotificationOutboxChannelSkipped {
			t.Fatalf("zero-pending real child = %+v", row)
		}
	}
	final, err := store.GetNotificationOutbox(parent.ID)
	if err != nil || final.Status != models.NotificationOutboxSent || len(sender.calls) != 0 {
		t.Fatalf("zero-pending final=%+v sends=%v err=%v", final, sender.calls, err)
	}
}

func deliveryOptionsWithSink(now *time.Time, sender ChannelSender, sink db.ApprovalTokenLocatorStore) CloudTenantDeliveryPassOptions {
	opts := deliveryOptions(now, sender, ChannelEmail, ChannelSlack)
	opts.ApprovalTokenHMACKey = strings.Repeat("h", 32)
	opts.ApprovalTokenLocatorStore = sink
	return opts
}

func findRealParentID(t *testing.T, store *db.SQLiteStore) string {
	t.Helper()
	parent, err := store.GetNotificationOutboxByKey("site-real", db.NotificationOutboxKindSiteDigest, "site-digest:300:600")
	if err != nil || parent == nil {
		t.Fatalf("read real parent: %v, %#v", err, parent)
	}
	return parent.ID
}
