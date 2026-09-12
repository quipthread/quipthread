package notifications

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/quipthread/quipthread/cloud"
	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/models"
)

type deliverySource struct {
	mu    sync.Mutex
	pages []struct {
		locators []*cloud.AccountLocator
		next     string
	}
	completed bool
}

func (s *deliverySource) ListAccountLocators(_ context.Context, cursor string, _ int) ([]*cloud.AccountLocator, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := 0
	if cursor != "" {
		index = 1
	}
	if index >= len(s.pages) {
		s.completed = true
		return nil, "", nil
	}
	page := s.pages[index]
	if page.next == "" {
		s.completed = true
	}
	return page.locators, page.next, nil
}

type deliveryLease struct {
	mu sync.Mutex

	accountID string
	parent    *models.NotificationOutbox
	material  *models.SiteDigestMaterialization
	children  []*models.NotificationOutboxChannel
	tokens    []*models.ApprovalToken

	releaseCalls int
	ensureCalls  int
	ackFailures  int
	tokensErr    error
	slowMaterial bool
	userLookup   func(context.Context, string) (*models.User, error)
	events       *[]string
	eventsMu     *sync.Mutex
}

func (l *deliveryLease) event(value string) {
	if l.events == nil || l.eventsMu == nil {
		return
	}
	l.eventsMu.Lock()
	*l.events = append(*l.events, value)
	l.eventsMu.Unlock()
}

func (l *deliveryLease) ClaimNotificationOutboxContext(ctx context.Context, owner string, now time.Time, leaseFor time.Duration, _ int) ([]*models.NotificationOutbox, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.parent == nil || (l.parent.Status != models.NotificationOutboxPending && (l.parent.LeaseUntil == nil || l.parent.LeaseUntil.After(now))) {
		return nil, nil
	}
	l.parent.Status = models.NotificationOutboxLeased
	l.parent.LeaseOwner = owner
	l.parent.Attempts++
	leaseUntil := now.Add(leaseFor)
	l.parent.LeaseUntil = &leaseUntil
	return []*models.NotificationOutbox{cloneDeliveryParent(l.parent)}, nil
}

func (l *deliveryLease) MaterializeClaimedSiteDigest(ctx context.Context, _ string, _ string, _ int, _ time.Time) (*models.SiteDigestMaterialization, error) {
	l.event("materialize:" + l.accountID)
	if l.slowMaterial {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	material := *l.material
	material.Outbox = cloneDeliveryParent(l.parent)
	return &material, nil
}

func (l *deliveryLease) FinalizeClaimedSiteDigest(ctx context.Context, _ string, _ string, _ int, _ time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.parent.Status = models.NotificationOutboxSent
	l.parent.LeaseOwner = ""
	l.parent.LeaseUntil = nil
	l.event("finalize:" + l.accountID)
	return nil
}

func (l *deliveryLease) SkipNotificationOutboxChannelsContext(ctx context.Context, _ string, _ string, _ int, _ time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, child := range l.children {
		if child.Status == models.NotificationOutboxChannelPending || child.Status == models.NotificationOutboxChannelLeased {
			child.Status = models.NotificationOutboxChannelSkipped
			child.LeaseOwner = ""
			child.LeaseUntil = nil
		}
	}
	return nil
}

func (l *deliveryLease) EnsureMaterializedSiteDigestApprovalTokens(ctx context.Context, _ *models.SiteDigestMaterialization, _ db.SiteDigestApprovalTokenOptions) ([]*models.ApprovalToken, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l.event("tokens:" + l.accountID)
	if l.tokensErr != nil {
		return nil, l.tokensErr
	}
	return l.tokens, nil
}

func (l *deliveryLease) GetUser(ownerID string) (*models.User, error) {
	return &models.User{ID: l.accountID + ":owner", Email: l.accountID + "@example.test"}, nil
}

func (l *deliveryLease) GetUserContext(ctx context.Context, ownerID string) (*models.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if l.userLookup != nil {
		return l.userLookup(ctx, ownerID)
	}
	return &models.User{ID: l.accountID + ":owner", Email: l.accountID + "@example.test"}, nil
}

func (l *deliveryLease) ListNotificationOutboxChannelsContext(ctx context.Context, _ string) ([]*models.NotificationOutboxChannel, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return cloneDeliveryChildren(l.children), nil
}

func (l *deliveryLease) EnsureNotificationOutboxChannelsContext(ctx context.Context, parentID string, channels []string) ([]*models.NotificationOutboxChannel, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ensureCalls++
	now := time.Unix(1000, 0).UTC()
	for _, channel := range channels {
		l.children = append(l.children, &models.NotificationOutboxChannel{ID: parentID + ":" + channel, OutboxID: parentID, Channel: channel, Status: models.NotificationOutboxChannelPending, AvailableAt: now})
	}
	return cloneDeliveryChildren(l.children), nil
}

func (l *deliveryLease) ClaimNotificationOutboxChannelsContext(ctx context.Context, _ string, owner string, now time.Time, leaseFor time.Duration, _ int) ([]*models.NotificationOutboxChannel, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	claimed := make([]*models.NotificationOutboxChannel, 0)
	for _, child := range l.children {
		if child.Status == models.NotificationOutboxChannelSent || child.AvailableAt.After(now) || (child.Status == models.NotificationOutboxChannelLeased && child.LeaseUntil != nil && child.LeaseUntil.After(now)) {
			continue
		}
		child.Status = models.NotificationOutboxChannelLeased
		child.LeaseOwner = owner
		child.Attempts++
		leaseUntil := now.Add(leaseFor)
		child.LeaseUntil = &leaseUntil
		claimed = append(claimed, cloneDeliveryChild(child))
	}
	return claimed, nil
}

func (l *deliveryLease) AckNotificationOutboxChannelContext(ctx context.Context, id, owner string, generation int, now time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ackFailures > 0 {
		l.ackFailures--
		return errors.New("ack contained tenant locator and credential")
	}
	for _, child := range l.children {
		if child.ID == id {
			if child.Status != models.NotificationOutboxChannelLeased || child.LeaseOwner != owner || child.Attempts != generation || child.LeaseUntil == nil || !child.LeaseUntil.After(now) {
				return errors.New("stale child generation")
			}
			child.Status = models.NotificationOutboxChannelSent
			child.LeaseOwner = ""
			child.LeaseUntil = nil
			return nil
		}
	}
	return errors.New("missing child")
}

func (l *deliveryLease) RetryNotificationOutboxChannelContext(ctx context.Context, id, owner string, generation int, availableAt, now time.Time, lastError string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if lastError == "" {
		return errors.New("empty retry error")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, child := range l.children {
		if child.ID == id {
			if child.Status != models.NotificationOutboxChannelLeased || child.LeaseOwner != owner || child.Attempts != generation || child.LeaseUntil == nil || !child.LeaseUntil.After(now) {
				return errors.New("stale child generation")
			}
			child.Status = models.NotificationOutboxChannelPending
			child.AvailableAt = availableAt
			child.LeaseOwner = ""
			child.LeaseUntil = nil
			child.LastError = lastError
			return nil
		}
	}
	return errors.New("missing child")
}

func (l *deliveryLease) RetryNotificationOutboxContext(ctx context.Context, _ string, _ string, _ int, availableAt, _ time.Time, lastError string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.parent.Status = models.NotificationOutboxPending
	l.parent.AvailableAt = availableAt
	l.parent.LeaseOwner = ""
	l.parent.LeaseUntil = nil
	l.parent.LastError = lastError
	return nil
}

func (l *deliveryLease) Release() error {
	l.mu.Lock()
	l.releaseCalls++
	l.mu.Unlock()
	return nil
}

type deliverySender struct {
	mu         sync.Mutex
	calls      []string
	events     *[]string
	eventsMu   *sync.Mutex
	batchCheck func(Batch) error
	err        map[string]error
	check      func() error
}

type delayedDeliverySender struct {
	deliverySender
	delay time.Duration
}

func (s *delayedDeliverySender) Send(ctx context.Context, channel string, batch Batch) error {
	time.Sleep(s.delay)
	return s.deliverySender.Send(ctx, channel, batch)
}

func (s *deliverySender) Send(_ context.Context, channel string, batch Batch) error {
	if s.batchCheck != nil {
		if err := s.batchCheck(batch); err != nil {
			return err
		}
	}
	if s.check != nil {
		if err := s.check(); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.calls = append(s.calls, channel)
	s.mu.Unlock()
	if s.events != nil && s.eventsMu != nil {
		s.eventsMu.Lock()
		*s.events = append(*s.events, "send:"+channel)
		s.eventsMu.Unlock()
	}
	return s.err[channel]
}

func (s *deliverySender) callCount(channel string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, called := range s.calls {
		if called == channel {
			count++
		}
	}
	return count
}

func newDeliveryLease(accountID string, pending bool) *deliveryLease {
	parent := &models.NotificationOutbox{ID: accountID + ":parent", SiteID: accountID + ":site", Kind: db.NotificationOutboxKindSiteDigest, Status: models.NotificationOutboxPending, AvailableAt: time.Unix(1000, 0).UTC()}
	comments := []*models.Comment{}
	if pending {
		comments = []*models.Comment{{ID: accountID + ":comment", SiteID: parent.SiteID, Content: accountID + "-private-content", Status: "pending"}}
	}
	return &deliveryLease{
		accountID: accountID,
		parent:    parent,
		material: &models.SiteDigestMaterialization{
			Outbox:   parent,
			Site:     &models.Site{ID: parent.SiteID, Domain: accountID + ".example.test"},
			Comments: comments,
		},
		tokens: []*models.ApprovalToken{{Token: "token-" + accountID, CommentID: accountID + ":comment", ExpiresAt: time.Unix(2000, 0).UTC()}},
	}
}

func deliveryOptions(now *time.Time, sender ChannelSender, channels ...string) CloudTenantDeliveryPassOptions {
	return CloudTenantDeliveryPassOptions{
		PageSize: 2, TenantConcurrency: 2, TenantTimeout: time.Second,
		ParentClaimLimit: 10, ChildClaimLimit: 10, ParentLease: time.Minute, ChildLease: time.Minute,
		RetryDelay: time.Second, TokenTTL: time.Hour, LeaseOwner: "delivery-test-owner", Now: func() time.Time { return *now },
		ChannelNames: channels, BaseURL: "https://approval.example.test",
		SenderFactory: func(CloudTenantDeliveryLease) ChannelSender { return sender },
	}
}

func TestRunCloudTenantDeliveryPassCompletesLocatorsBeforeSending(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	source := &deliverySource{pages: []struct {
		locators []*cloud.AccountLocator
		next     string
	}{{locators: []*cloud.AccountLocator{{ID: "tenant-a"}}, next: "tenant-a"}, {locators: []*cloud.AccountLocator{{ID: "tenant-b"}}}}}
	leases := map[string]*deliveryLease{"tenant-a": newDeliveryLease("tenant-a", true), "tenant-b": newDeliveryLease("tenant-b", true)}
	var events []string
	var eventsMu sync.Mutex
	sender := &deliverySender{events: &events, eventsMu: &eventsMu}
	options := deliveryOptions(&now, sender, ChannelWebhook)
	options.SenderFactory = func(lease CloudTenantDeliveryLease) ChannelSender {
		tenant := lease.(*deliveryLease)
		return &deliverySender{batchCheck: func(batch Batch) error {
			if batch.Site.ID != tenant.accountID+":site" || len(batch.Comments) != 1 || batch.Comments[0].Content != tenant.accountID+"-private-content" {
				return errors.New("tenant content crossed delivery leases")
			}
			return nil
		}}
	}
	report, err := RunCloudTenantDeliveryPass(context.Background(), source, func(_ context.Context, locator cloud.AccountLocator) (CloudTenantDeliveryLease, error) {
		return leases[locator.ID], nil
	}, options)
	if err != nil || len(report.Tenants) != 2 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
	source.mu.Lock()
	completed := source.completed
	source.mu.Unlock()
	if !completed {
		t.Fatal("provider ran before locator enumeration completed")
	}
}

func TestRunCloudTenantDeliveryPassZeroPendingFinalizesWithoutProvider(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	lease := newDeliveryLease("tenant-zero", false)
	lease.children = []*models.NotificationOutboxChannel{
		{ID: "pending-child", OutboxID: lease.parent.ID, Channel: ChannelEmail, Status: models.NotificationOutboxChannelPending, AvailableAt: now},
		{ID: "leased-child", OutboxID: lease.parent.ID, Channel: ChannelSlack, Status: models.NotificationOutboxChannelLeased, LeaseOwner: "old-owner", LeaseUntil: func() *time.Time { until := now.Add(time.Hour); return &until }()},
	}
	sender := &deliverySender{}
	source := &deliverySource{pages: []struct {
		locators []*cloud.AccountLocator
		next     string
	}{{locators: []*cloud.AccountLocator{{ID: "tenant-zero"}}}}}
	_, err := RunCloudTenantDeliveryPass(context.Background(), source, func(context.Context, cloud.AccountLocator) (CloudTenantDeliveryLease, error) { return lease, nil }, deliveryOptions(&now, sender, ChannelWebhook))
	if err != nil || sender.callCount(ChannelWebhook) != 0 || lease.parent.Status != models.NotificationOutboxSent {
		t.Fatalf("err=%v calls=%d parent=%s", err, sender.callCount(ChannelWebhook), lease.parent.Status)
	}
	for _, child := range lease.children {
		if child.Status != models.NotificationOutboxChannelSkipped {
			t.Fatalf("zero-pending child was not skipped: %+v", child)
		}
	}
}

func TestRunCloudTenantDeliveryPassPartialChannelRetryAndSnapshot(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	lease := newDeliveryLease("tenant-partial", true)
	sender := &deliverySender{err: map[string]error{ChannelSlack: errors.New("webhook secret content recipient")}}
	source := &deliverySource{pages: []struct {
		locators []*cloud.AccountLocator
		next     string
	}{{locators: []*cloud.AccountLocator{{ID: "tenant-partial"}}}}}
	opts := deliveryOptions(&now, sender, ChannelEmail, ChannelSlack)
	if _, err := RunCloudTenantDeliveryPass(context.Background(), source, func(context.Context, cloud.AccountLocator) (CloudTenantDeliveryLease, error) { return lease, nil }, opts); err != nil {
		t.Fatal(err)
	}
	if sender.callCount(ChannelEmail) != 1 || sender.callCount(ChannelSlack) != 1 || lease.ensureCalls != 1 {
		t.Fatalf("calls email=%d slack=%d ensure=%d", sender.callCount(ChannelEmail), sender.callCount(ChannelSlack), lease.ensureCalls)
	}
	for _, child := range lease.children {
		if child.Channel == ChannelEmail && child.Status != models.NotificationOutboxChannelSent {
			t.Fatalf("email child=%+v", child)
		}
		if child.Channel == ChannelSlack && child.Status != models.NotificationOutboxChannelPending {
			t.Fatalf("slack child=%+v", child)
		}
	}
	// A changed configuration must not add a new child to an existing parent.
	now = now.Add(2 * time.Minute)
	opts = deliveryOptions(&now, sender, ChannelEmail, ChannelSlack, ChannelWebhook)
	if _, err := RunCloudTenantDeliveryPass(context.Background(), source, func(context.Context, cloud.AccountLocator) (CloudTenantDeliveryLease, error) { return lease, nil }, opts); err != nil {
		t.Fatal(err)
	}
	if lease.ensureCalls != 1 {
		t.Fatalf("new channel was configured on retry; ensure calls=%d", lease.ensureCalls)
	}
}

func TestRunCloudTenantDeliveryPassAckFailureRetriesAtLeastOnce(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	lease := newDeliveryLease("tenant-ack", true)
	lease.ackFailures = 1
	sender := &deliverySender{}
	source := &deliverySource{pages: []struct {
		locators []*cloud.AccountLocator
		next     string
	}{{locators: []*cloud.AccountLocator{{ID: "tenant-ack"}}}}}
	opts := deliveryOptions(&now, sender, ChannelEmail)
	opener := func(context.Context, cloud.AccountLocator) (CloudTenantDeliveryLease, error) { return lease, nil }
	if _, err := RunCloudTenantDeliveryPass(context.Background(), source, opener, opts); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := RunCloudTenantDeliveryPass(context.Background(), source, opener, deliveryOptions(&now, sender, ChannelEmail)); err != nil {
		t.Fatal(err)
	}
	if sender.callCount(ChannelEmail) != 2 || lease.parent.Status != models.NotificationOutboxSent {
		t.Fatalf("calls=%d parent=%s", sender.callCount(ChannelEmail), lease.parent.Status)
	}
}

func TestRunCloudTenantDeliveryPassReclaimedSentChildrenDoNotResend(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	lease := newDeliveryLease("tenant-reclaimed", true)
	lease.children = []*models.NotificationOutboxChannel{{ID: "sent", OutboxID: lease.parent.ID, Channel: ChannelEmail, Status: models.NotificationOutboxChannelSent}}
	sender := &deliverySender{}
	source := &deliverySource{pages: []struct {
		locators []*cloud.AccountLocator
		next     string
	}{{locators: []*cloud.AccountLocator{{ID: "tenant-reclaimed"}}}}}
	if _, err := RunCloudTenantDeliveryPass(context.Background(), source, func(context.Context, cloud.AccountLocator) (CloudTenantDeliveryLease, error) { return lease, nil }, deliveryOptions(&now, sender, ChannelEmail, ChannelSlack)); err != nil {
		t.Fatal(err)
	}
	if sender.callCount(ChannelEmail) != 0 || sender.callCount(ChannelSlack) != 0 || lease.parent.Status != models.NotificationOutboxSent {
		t.Fatalf("calls email=%d slack=%d parent=%s", sender.callCount(ChannelEmail), sender.callCount(ChannelSlack), lease.parent.Status)
	}
}

func TestRunCloudTenantDeliveryPassApprovalLocatorFailureSendsNothing(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	lease := newDeliveryLease("tenant-locator-failure", true)
	lease.tokensErr = errors.New("control-plane locator mismatch with raw token")
	sender := &deliverySender{}
	source := &deliverySource{pages: []struct {
		locators []*cloud.AccountLocator
		next     string
	}{{locators: []*cloud.AccountLocator{{ID: "tenant-locator-failure"}}}}}
	if _, err := RunCloudTenantDeliveryPass(context.Background(), source, func(context.Context, cloud.AccountLocator) (CloudTenantDeliveryLease, error) { return lease, nil }, deliveryOptions(&now, sender, ChannelEmail)); err != nil {
		t.Fatal(err)
	}
	if len(sender.calls) != 0 {
		t.Fatalf("provider calls after locator failure = %v", sender.calls)
	}
}

func TestNewCloudTenantChannelSenderFactoryBindsEmailRecipientToLease(t *testing.T) {
	cfg := &config.Config{EmailProvider: "resend", EmailAPIKey: "api-key", SMTPFrom: "from@example.test"}
	factory := NewCloudTenantChannelSenderFactory(cfg)
	a := factory(newDeliveryLease("tenant-a", true)).(*NamedChannelSender)
	b := factory(newDeliveryLease("tenant-b", true)).(*NamedChannelSender)
	emailA := a.notifiers[ChannelEmail].(*EmailAPINotifier)
	emailB := b.notifiers[ChannelEmail].(*EmailAPINotifier)
	if got := emailA.ownerEmail("owner"); got != "tenant-a@example.test" {
		t.Fatalf("tenant-a recipient = %q", got)
	}
	if got := emailB.ownerEmail("owner"); got != "tenant-b@example.test" {
		t.Fatalf("tenant-b recipient = %q", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := emailA.ownerEmailContext(ctx, "owner"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled recipient lookup error = %v", err)
	}
}

func TestRunCloudTenantDeliveryPassCancelledRecipientLookupSkipsProviderAndReleases(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	lease := newDeliveryLease("tenant-recipient", true)
	lease.userLookup = func(ctx context.Context, _ string) (*models.User, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	var providerCalls int
	baseFactory := NewCloudTenantChannelSenderFactory(&config.Config{
		EmailProvider: "resend", EmailAPIKey: "api-key", SMTPFrom: "from@example.test",
	})
	options := deliveryOptions(&now, &deliverySender{}, ChannelEmail)
	options.TenantTimeout = 25 * time.Millisecond
	options.ParentLease = time.Second
	options.ChildLease = time.Second
	options.SenderFactory = func(tenantLease CloudTenantDeliveryLease) ChannelSender {
		sender := baseFactory(tenantLease).(*NamedChannelSender)
		email := sender.notifiers[ChannelEmail].(*EmailAPINotifier)
		email.providerSend = func(context.Context, *config.Config, string, string, string) error {
			providerCalls++
			return nil
		}
		return sender
	}
	source := &deliverySource{pages: []struct {
		locators []*cloud.AccountLocator
		next     string
	}{{locators: []*cloud.AccountLocator{{ID: lease.accountID}}}}}
	started := time.Now()
	if _, err := RunCloudTenantDeliveryPass(context.Background(), source, func(context.Context, cloud.AccountLocator) (CloudTenantDeliveryLease, error) {
		return lease, nil
	}, options); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("cancelled recipient lookup did not release promptly: %s", elapsed)
	}
	if providerCalls != 0 {
		t.Fatalf("provider calls after cancelled recipient lookup = %d", providerCalls)
	}
	if lease.releaseCalls != 1 {
		t.Fatalf("release calls = %d, want 1", lease.releaseCalls)
	}
}

func TestRunCloudTenantDeliveryPassLocatorMismatchSendsNothing(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	sender := &deliverySender{}
	source := &deliverySource{pages: []struct {
		locators []*cloud.AccountLocator
		next     string
	}{{locators: []*cloud.AccountLocator{{ID: "tenant-mismatch"}}, next: "wrong"}}}
	opens := 0
	_, err := RunCloudTenantDeliveryPass(context.Background(), source, func(context.Context, cloud.AccountLocator) (CloudTenantDeliveryLease, error) {
		opens++
		return nil, nil
	}, deliveryOptions(&now, sender, ChannelEmail))
	if !errors.Is(err, ErrCloudTenantDeliveryInvalidPage) || opens != 0 || len(sender.calls) != 0 {
		t.Fatalf("err=%v opens=%d calls=%v", err, opens, sender.calls)
	}
}

func TestRunCloudTenantDeliveryPassTimeoutIsolatesTenantAndReleases(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	slow := newDeliveryLease("tenant-slow", true)
	slow.slowMaterial = true
	healthy := newDeliveryLease("tenant-healthy", true)
	sender := &deliverySender{}
	source := &deliverySource{pages: []struct {
		locators []*cloud.AccountLocator
		next     string
	}{{locators: []*cloud.AccountLocator{{ID: "tenant-healthy"}, {ID: "tenant-slow"}}}}}
	leases := map[string]*deliveryLease{"tenant-slow": slow, "tenant-healthy": healthy}
	opts := deliveryOptions(&now, sender, ChannelEmail)
	opts.TenantTimeout = 10 * time.Millisecond
	report, err := RunCloudTenantDeliveryPass(context.Background(), source, func(_ context.Context, locator cloud.AccountLocator) (CloudTenantDeliveryLease, error) {
		return leases[locator.ID], nil
	}, opts)
	if err != nil || sender.callCount(ChannelEmail) != 1 || slow.releaseCalls != 1 || healthy.releaseCalls != 1 {
		t.Fatalf("report=%+v err=%v sends=%d releases=%d/%d", report, err, sender.callCount(ChannelEmail), slow.releaseCalls, healthy.releaseCalls)
	}
	results := make(map[string]CloudTenantDeliveryResult)
	for _, result := range report.Tenants {
		results[result.AccountID] = result
	}
	if !results["tenant-slow"].TimedOut || results["tenant-healthy"].TimedOut {
		t.Fatalf("timeout results=%+v", results)
	}
}

func TestRunCloudTenantDeliveryPassRequiresLeasesToCoverTenantTimeout(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	source := &deliverySource{pages: []struct {
		locators []*cloud.AccountLocator
		next     string
	}{{}}}
	base := deliveryOptions(&now, &deliverySender{}, ChannelEmail)
	base.TenantTimeout = time.Second
	base.ParentLease = time.Second - time.Nanosecond
	if _, err := RunCloudTenantDeliveryPass(context.Background(), source, func(context.Context, cloud.AccountLocator) (CloudTenantDeliveryLease, error) { return nil, nil }, base); !errors.Is(err, ErrCloudTenantDeliveryInvalidOptions) {
		t.Fatalf("parent lease validation error = %v", err)
	}
	base.ParentLease = time.Second
	base.ChildLease = time.Second - time.Nanosecond
	if _, err := RunCloudTenantDeliveryPass(context.Background(), source, func(context.Context, cloud.AccountLocator) (CloudTenantDeliveryLease, error) { return nil, nil }, base); !errors.Is(err, ErrCloudTenantDeliveryInvalidOptions) {
		t.Fatalf("child lease validation error = %v", err)
	}
}

func TestRunCloudTenantDeliveryPassDelayedProviderReclaimsExpiredLease(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	lease := newDeliveryLease("tenant-delayed", true)
	firstSender := &delayedDeliverySender{delay: 20 * time.Millisecond}
	source := &deliverySource{pages: []struct {
		locators []*cloud.AccountLocator
		next     string
	}{{locators: []*cloud.AccountLocator{{ID: "tenant-delayed"}}}}}
	opener := func(context.Context, cloud.AccountLocator) (CloudTenantDeliveryLease, error) { return lease, nil }
	opts := deliveryOptions(&now, firstSender, ChannelEmail)
	opts.TenantTimeout = 10 * time.Millisecond
	opts.ParentLease = 10 * time.Millisecond
	opts.ChildLease = 10 * time.Millisecond
	if _, err := RunCloudTenantDeliveryPass(context.Background(), source, opener, opts); err != nil {
		t.Fatal(err)
	}
	if firstSender.callCount(ChannelEmail) != 1 {
		t.Fatalf("expired-provider attempts = %d, want 1", firstSender.callCount(ChannelEmail))
	}
	if lease.parent.Status != models.NotificationOutboxLeased && lease.parent.Status != models.NotificationOutboxPending {
		t.Fatalf("parent after cancellation = %s, want leased or pending", lease.parent.Status)
	}

	now = now.Add(time.Minute)
	secondSender := &deliverySender{}
	if _, err := RunCloudTenantDeliveryPass(context.Background(), source, opener, deliveryOptions(&now, secondSender, ChannelEmail)); err != nil {
		t.Fatal(err)
	}
	if secondSender.callCount(ChannelEmail) != 1 || lease.parent.Status != models.NotificationOutboxSent {
		t.Fatalf("reclaim calls=%d parent=%s", secondSender.callCount(ChannelEmail), lease.parent.Status)
	}
}

func TestCloudEligibleChannelNamesExcludeSMTPAndSES(t *testing.T) {
	if got := fmt.Sprint(CloudEligibleChannelNames(&config.Config{SMTPHost: "smtp", SMTPFrom: "from"})); got != "[]" {
		t.Fatalf("SMTP channels=%s", got)
	}
	if got := fmt.Sprint(CloudEligibleChannelNames(&config.Config{EmailProvider: "ses", EmailAPIKey: "key", SMTPFrom: "from"})); got != "[]" {
		t.Fatalf("SES channels=%s", got)
	}
}

func cloneDeliveryParent(parent *models.NotificationOutbox) *models.NotificationOutbox {
	copy := *parent
	if parent.LeaseUntil != nil {
		leaseUntil := *parent.LeaseUntil
		copy.LeaseUntil = &leaseUntil
	}
	return &copy
}

func cloneDeliveryChild(child *models.NotificationOutboxChannel) *models.NotificationOutboxChannel {
	copy := *child
	if child.LeaseUntil != nil {
		leaseUntil := *child.LeaseUntil
		copy.LeaseUntil = &leaseUntil
	}
	return &copy
}

func cloneDeliveryChildren(children []*models.NotificationOutboxChannel) []*models.NotificationOutboxChannel {
	clones := make([]*models.NotificationOutboxChannel, 0, len(children))
	for _, child := range children {
		clones = append(clones, cloneDeliveryChild(child))
	}
	return clones
}
