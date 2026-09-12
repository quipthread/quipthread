package notifications

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/quipthread/quipthread/cloud"
	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/models"
)

const (
	DefaultCloudTenantDeliveryRetryDelay = time.Minute
	DefaultCloudTenantDeliveryTokenTTL   = 24 * time.Hour
	MaxCloudTenantDeliveryRetryDelay     = time.Hour
	MaxCloudTenantDeliveryTokenTTL       = 7 * 24 * time.Hour
)

var (
	ErrCloudTenantDeliveryInvalidOptions = errors.New("cloud tenant delivery: invalid options")
	ErrCloudTenantDeliveryInvalidPage    = errors.New("cloud tenant delivery: invalid pagination response")
)

// CloudTenantDeliveryLease is the only tenant-content seam used by the
// delivery pass. The outer pass never receives this interface; it sees only
// account IDs and delivery summaries.
type CloudTenantDeliveryLease interface {
	ClaimNotificationOutboxContext(ctx context.Context, owner string, now time.Time, leaseFor time.Duration, limit int) ([]*models.NotificationOutbox, error)
	MaterializeClaimedSiteDigest(ctx context.Context, outboxID, owner string, generation int, now time.Time) (*models.SiteDigestMaterialization, error)
	FinalizeClaimedSiteDigest(ctx context.Context, outboxID, owner string, generation int, now time.Time) error
	EnsureMaterializedSiteDigestApprovalTokens(ctx context.Context, materialized *models.SiteDigestMaterialization, options db.SiteDigestApprovalTokenOptions) ([]*models.ApprovalToken, error)
	GetUser(ownerID string) (*models.User, error)
	GetUserContext(ctx context.Context, ownerID string) (*models.User, error)
	ListNotificationOutboxChannelsContext(ctx context.Context, outboxID string) ([]*models.NotificationOutboxChannel, error)
	EnsureNotificationOutboxChannelsContext(ctx context.Context, outboxID string, channels []string) ([]*models.NotificationOutboxChannel, error)
	ClaimNotificationOutboxChannelsContext(ctx context.Context, outboxID, owner string, now time.Time, leaseFor time.Duration, limit int) ([]*models.NotificationOutboxChannel, error)
	AckNotificationOutboxChannelContext(ctx context.Context, id, owner string, generation int, now time.Time) error
	RetryNotificationOutboxChannelContext(ctx context.Context, id, owner string, generation int, availableAt, now time.Time, lastError string) error
	RetryNotificationOutboxContext(ctx context.Context, id, owner string, generation int, availableAt, now time.Time, lastError string) error
	SkipNotificationOutboxChannelsContext(ctx context.Context, outboxID, owner string, generation int, now time.Time) error
	Release() error
}

// CloudTenantDeliveryOpener resolves exactly one tenant store lease for a
// locator. Implementations must not return a process-global tenant store.
type CloudTenantDeliveryOpener func(ctx context.Context, locator cloud.AccountLocator) (CloudTenantDeliveryLease, error)

// CloudTenantDeliveryPassOptions controls one disabled delivery pass. The
// channel list is snapshotted during option normalization and reused for every
// parent in this pass, so retries cannot acquire newly configured channels.
type CloudTenantDeliveryPassOptions struct {
	PageSize          int
	TenantConcurrency int
	TenantTimeout     time.Duration
	ParentClaimLimit  int
	ChildClaimLimit   int
	ParentLease       time.Duration
	ChildLease        time.Duration
	RetryDelay        time.Duration
	TokenTTL          time.Duration
	LeaseOwner        string
	Now               func() time.Time

	Config                    *config.Config
	ChannelNames              []string
	BaseURL                   string
	ApprovalTokenHMACKey      string
	ApprovalTokenLocatorStore db.ApprovalTokenLocatorStore
	SenderFactory             func(CloudTenantDeliveryLease) ChannelSender
}

// CloudTenantDeliveryResult is metadata-only. It contains no tenant locator,
// comments, approval tokens, recipients, or provider errors.
type CloudTenantDeliveryResult struct {
	AccountID        string
	Opened           bool
	Released         bool
	ReleaseFailed    bool
	ParentsClaimed   int
	ParentsFinalized int
	ChildrenSent     int
	TimedOut         bool
	Failure          string // "open", "claim", "materialize", "tokens", "channel", "retry", "finalize", "timeout", or "release"
}

type CloudTenantDeliveryPassReport struct {
	LeaseOwner string
	Pages      int
	Accounts   int
	Tenants    []CloudTenantDeliveryResult
}

type normalizedCloudTenantDeliveryOptions struct {
	pageSize          int
	tenantConcurrency int
	tenantTimeout     time.Duration
	parentClaimLimit  int
	childClaimLimit   int
	parentLease       time.Duration
	childLease        time.Duration
	retryDelay        time.Duration
	tokenTTL          time.Duration
	leaseOwner        string
	now               func() time.Time
	channels          []string
	baseURL           string
	hmacKey           string
	locatorStore      db.ApprovalTokenLocatorStore
	senderFactory     func(CloudTenantDeliveryLease) ChannelSender
}

// CloudEligibleChannelNames returns the cloud provider channels eligible for
// a new digest. SMTP and SES are deliberately excluded.
func CloudEligibleChannelNames(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	channels := make([]string, 0, 5)
	switch strings.ToLower(cfg.EmailProvider) {
	case "resend", "postmark", "sendgrid":
		if cfg.EmailAPIKey != "" && cfg.SMTPFrom != "" {
			channels = append(channels, ChannelEmail)
		}
	}
	if cfg.SlackWebhookURL != "" {
		channels = append(channels, ChannelSlack)
	}
	if cfg.DiscordWebhookURL != "" {
		channels = append(channels, ChannelDiscord)
	}
	if cfg.TelegramBotToken != "" && cfg.TelegramChatID != "" {
		channels = append(channels, ChannelTelegram)
	}
	if cfg.WebhookURL != "" {
		channels = append(channels, ChannelWebhook)
	}
	return channels
}

// NewCloudTenantChannelSenderFactory builds providers after the tenant lease
// has been opened. Email recipient lookup therefore closes over that tenant's
// store rather than a process-global db.Store.
func NewCloudTenantChannelSenderFactory(cfg *config.Config) func(CloudTenantDeliveryLease) ChannelSender {
	return func(lease CloudTenantDeliveryLease) ChannelSender {
		ownerEmail := func(ownerID string) string {
			if lease == nil {
				return ""
			}
			user, err := lease.GetUser(ownerID)
			if err != nil || user == nil {
				return ""
			}
			return user.Email
		}
		ownerEmailContext := func(ctx context.Context, ownerID string) (string, error) {
			if lease == nil {
				return "", ErrRecipientUnavailable
			}
			user, err := lease.GetUserContext(ctx, ownerID)
			if err != nil {
				return "", err
			}
			if user == nil || user.Email == "" {
				return "", ErrRecipientUnavailable
			}
			return user.Email, nil
		}
		notifiers := make(map[string]Notifier)
		if cfg != nil {
			for _, channel := range CloudEligibleChannelNames(cfg) {
				switch channel {
				case ChannelEmail:
					notifiers[channel] = NewContextEmailAPINotifier(cfg, ownerEmailContext, ownerEmail)
				case ChannelSlack:
					notifiers[channel] = NewSlackNotifier(cfg)
				case ChannelDiscord:
					notifiers[channel] = NewDiscordNotifier(cfg)
				case ChannelTelegram:
					notifiers[channel] = NewTelegramNotifier(cfg)
				case ChannelWebhook:
					notifiers[channel] = NewWebhookNotifier(cfg)
				}
			}
		}
		return NewNamedChannelSender(notifiers)
	}
}

// RunCloudTenantDeliveryPass performs one disabled, tenant-aware delivery
// pass. It completes and validates control-plane locator enumeration before it
// opens a tenant or invokes a provider. It never runs from startup.
func RunCloudTenantDeliveryPass(ctx context.Context, source CloudAccountLocatorSource, opener CloudTenantDeliveryOpener, options CloudTenantDeliveryPassOptions) (CloudTenantDeliveryPassReport, error) {
	var report CloudTenantDeliveryPassReport
	if ctx == nil || source == nil || opener == nil {
		return report, fmt.Errorf("%w: context, source, and opener are required", ErrCloudTenantDeliveryInvalidOptions)
	}
	opts, err := normalizeCloudTenantDeliveryOptions(options)
	if err != nil {
		return report, err
	}
	report.LeaseOwner = opts.leaseOwner

	// Do not process pages as they arrive. A later locator error or mismatch
	// must prevent every provider call in this pass.
	locators, pages, err := collectCloudTenantDeliveryLocators(ctx, source, opts.pageSize)
	if err != nil {
		return report, err
	}
	report.Pages = pages
	report.Accounts = len(locators)

	results, err := processCloudTenantDeliveryPage(ctx, locators, opener, opts)
	report.Tenants = results
	sort.Slice(report.Tenants, func(i, j int) bool { return report.Tenants[i].AccountID < report.Tenants[j].AccountID })
	if err != nil {
		return report, err
	}
	return report, nil
}

func normalizeCloudTenantDeliveryOptions(options CloudTenantDeliveryPassOptions) (normalizedCloudTenantDeliveryOptions, error) {
	opts := normalizedCloudTenantDeliveryOptions{
		pageSize:          options.PageSize,
		tenantConcurrency: options.TenantConcurrency,
		tenantTimeout:     options.TenantTimeout,
		parentClaimLimit:  options.ParentClaimLimit,
		childClaimLimit:   options.ChildClaimLimit,
		parentLease:       options.ParentLease,
		childLease:        options.ChildLease,
		retryDelay:        options.RetryDelay,
		tokenTTL:          options.TokenTTL,
		leaseOwner:        options.LeaseOwner,
		now:               options.Now,
		baseURL:           options.BaseURL,
		hmacKey:           options.ApprovalTokenHMACKey,
		locatorStore:      options.ApprovalTokenLocatorStore,
		senderFactory:     options.SenderFactory,
	}
	if options.Config != nil {
		if opts.baseURL == "" {
			opts.baseURL = options.Config.BaseURL
		}
		if opts.hmacKey == "" {
			opts.hmacKey = options.Config.ApprovalTokenHMACKey
		}
	}
	if opts.pageSize == 0 {
		opts.pageSize = DefaultCloudTenantPageSize
	}
	if opts.tenantConcurrency == 0 {
		opts.tenantConcurrency = DefaultCloudTenantConcurrency
	}
	if opts.tenantTimeout == 0 {
		opts.tenantTimeout = DefaultCloudTenantTimeout
	}
	if opts.parentClaimLimit == 0 {
		opts.parentClaimLimit = DefaultCloudTenantClaimLimit
	}
	if opts.childClaimLimit == 0 {
		opts.childClaimLimit = DefaultCloudTenantClaimLimit
	}
	if opts.parentLease == 0 {
		opts.parentLease = DefaultCloudTenantLease
	}
	if opts.childLease == 0 {
		opts.childLease = DefaultCloudTenantLease
	}
	if opts.retryDelay == 0 {
		opts.retryDelay = DefaultCloudTenantDeliveryRetryDelay
	}
	if opts.tokenTTL == 0 {
		opts.tokenTTL = DefaultCloudTenantDeliveryTokenTTL
	}
	if opts.now == nil {
		opts.now = func() time.Time { return time.Now().UTC() }
	}
	if opts.leaseOwner == "" {
		opts.leaseOwner = "cloud-delivery-" + uuid.NewString()
	}
	if opts.senderFactory == nil {
		return normalizedCloudTenantDeliveryOptions{}, fmt.Errorf("%w: sender factory is required", ErrCloudTenantDeliveryInvalidOptions)
	}
	if opts.pageSize < 1 || opts.pageSize > MaxCloudTenantPageSize ||
		opts.tenantConcurrency < 1 || opts.tenantConcurrency > MaxCloudTenantConcurrency ||
		opts.tenantTimeout <= 0 || opts.tenantTimeout > MaxCloudTenantTimeout ||
		opts.parentClaimLimit < 1 || opts.parentClaimLimit > MaxCloudTenantClaimLimit ||
		opts.childClaimLimit < 1 || opts.childClaimLimit > MaxCloudTenantClaimLimit ||
		opts.parentLease <= 0 || opts.parentLease > MaxCloudTenantLease ||
		opts.childLease <= 0 || opts.childLease > MaxCloudTenantLease ||
		opts.retryDelay <= 0 || opts.retryDelay > MaxCloudTenantDeliveryRetryDelay ||
		opts.tokenTTL <= 0 || opts.tokenTTL > MaxCloudTenantDeliveryTokenTTL {
		return normalizedCloudTenantDeliveryOptions{}, fmt.Errorf("%w: option out of bounds", ErrCloudTenantDeliveryInvalidOptions)
	}
	if opts.parentLease < opts.tenantTimeout || opts.childLease < opts.tenantTimeout {
		return normalizedCloudTenantDeliveryOptions{}, fmt.Errorf("%w: parent and child leases must cover tenant timeout", ErrCloudTenantDeliveryInvalidOptions)
	}
	channels := options.ChannelNames
	if len(channels) == 0 {
		channels = CloudEligibleChannelNames(options.Config)
	}
	var err error
	opts.channels, err = snapshotCloudDeliveryChannels(channels, options.Config)
	if err != nil {
		return normalizedCloudTenantDeliveryOptions{}, err
	}
	return opts, nil
}

func snapshotCloudDeliveryChannels(channels []string, cfg *config.Config) ([]string, error) {
	seen := make(map[string]struct{}, len(channels))
	snapshot := make([]string, 0, len(channels))
	eligible := make(map[string]struct{})
	if cfg != nil {
		for _, channel := range CloudEligibleChannelNames(cfg) {
			eligible[channel] = struct{}{}
		}
	}
	for _, channel := range channels {
		if !isNotificationChannel(channel) || (cfg != nil && !containsChannel(eligible, channel)) {
			return nil, fmt.Errorf("%w: invalid or ineligible channel", ErrCloudTenantDeliveryInvalidOptions)
		}
		if _, exists := seen[channel]; exists {
			continue
		}
		seen[channel] = struct{}{}
		snapshot = append(snapshot, channel)
	}
	return snapshot, nil
}

func containsChannel(channels map[string]struct{}, channel string) bool {
	_, ok := channels[channel]
	return ok
}

func collectCloudTenantDeliveryLocators(ctx context.Context, source CloudAccountLocatorSource, pageSize int) ([]*cloud.AccountLocator, int, error) {
	var all []*cloud.AccountLocator
	cursor := ""
	pages := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, pages, fmt.Errorf("cloud tenant delivery canceled: %w", err)
		}
		locators, nextCursor, err := source.ListAccountLocators(ctx, cursor, pageSize)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, pages, fmt.Errorf("cloud tenant delivery canceled: %w", ctxErr)
			}
			return nil, pages, fmt.Errorf("cloud tenant delivery: locator enumeration failed")
		}
		if err := validateCloudDeliveryLocatorPage(locators, cursor, nextCursor, pageSize); err != nil {
			return nil, pages, err
		}
		pages++
		all = append(all, locators...)
		if nextCursor == "" {
			return all, pages, nil
		}
		cursor = nextCursor
	}
}

func validateCloudDeliveryLocatorPage(locators []*cloud.AccountLocator, incomingCursor, nextCursor string, pageSize int) error {
	if len(locators) > pageSize {
		return fmt.Errorf("%w: page exceeds limit", ErrCloudTenantDeliveryInvalidPage)
	}
	if len(locators) == 0 {
		if nextCursor != "" {
			return fmt.Errorf("%w: empty page has a cursor", ErrCloudTenantDeliveryInvalidPage)
		}
		return nil
	}
	previous := incomingCursor
	for _, locator := range locators {
		if locator == nil || locator.ID == "" || locator.ID != strings.TrimSpace(locator.ID) || len(locator.ID) > cloud.MaxAccountLocatorCursorBytes || locator.ID <= previous {
			return fmt.Errorf("%w: malformed locator page", ErrCloudTenantDeliveryInvalidPage)
		}
		previous = locator.ID
	}
	if nextCursor != "" && nextCursor != locators[len(locators)-1].ID {
		return fmt.Errorf("%w: invalid next cursor", ErrCloudTenantDeliveryInvalidPage)
	}
	return nil
}

func processCloudTenantDeliveryPage(ctx context.Context, locators []*cloud.AccountLocator, opener CloudTenantDeliveryOpener, options normalizedCloudTenantDeliveryOptions) ([]CloudTenantDeliveryResult, error) {
	if len(locators) == 0 {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("cloud tenant delivery canceled: %w", err)
		}
		return []CloudTenantDeliveryResult{}, nil
	}
	workerCount := options.tenantConcurrency
	if workerCount > len(locators) {
		workerCount = len(locators)
	}
	jobs := make(chan *cloud.AccountLocator)
	results := make(chan CloudTenantDeliveryResult, len(locators))
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case locator, ok := <-jobs:
					if !ok {
						return
					}
					results <- processCloudTenantDelivery(ctx, locator, opener, options)
				}
			}
		}()
	}
	stopped := false
	for _, locator := range locators {
		select {
		case <-ctx.Done():
			stopped = true
		case jobs <- locator:
		}
		if stopped {
			break
		}
	}
	close(jobs)
	workers.Wait()
	close(results)
	pageResults := make([]CloudTenantDeliveryResult, 0, len(locators))
	for result := range results {
		pageResults = append(pageResults, result)
	}
	if err := ctx.Err(); err != nil {
		return pageResults, fmt.Errorf("cloud tenant delivery canceled: %w", err)
	}
	return pageResults, nil
}

func processCloudTenantDelivery(ctx context.Context, locator *cloud.AccountLocator, opener CloudTenantDeliveryOpener, options normalizedCloudTenantDeliveryOptions) (result CloudTenantDeliveryResult) {
	result.AccountID = locator.ID
	tenantCtx, cancel := context.WithTimeout(ctx, options.tenantTimeout)
	defer cancel()
	lease, err := opener(tenantCtx, *locator)
	if err != nil {
		if lease != nil {
			if releaseErr := lease.Release(); releaseErr != nil {
				result.ReleaseFailed = true
			} else {
				result.Released = true
			}
		}
		result.Failure, result.TimedOut = cloudDeliveryFailure(tenantCtx, err, "open")
		return result
	}
	if lease == nil {
		result.Failure = "open"
		return result
	}
	result.Opened = true
	defer func() {
		if releaseErr := lease.Release(); releaseErr != nil {
			result.ReleaseFailed = true
			if result.Failure == "" {
				result.Failure = "release"
			}
		} else {
			result.Released = true
		}
	}()

	sender := options.senderFactory(lease)
	if sender == nil {
		result.Failure = "channel"
		return result
	}
	now := options.now()
	parents, err := lease.ClaimNotificationOutboxContext(tenantCtx, options.leaseOwner, now, options.parentLease, options.parentClaimLimit)
	if err != nil {
		result.Failure, result.TimedOut = cloudDeliveryFailure(tenantCtx, err, "claim")
		return result
	}
	result.ParentsClaimed = len(parents)
	for _, parent := range parents {
		if err := deliverClaimedCloudParent(tenantCtx, lease, sender, locator.ID, parent, options, &result); err != nil && result.Failure == "" {
			// Reports cross the operational boundary; never copy provider or DB
			// error text into the tenant result.
			result.Failure = "parent"
		}
		if errors.Is(tenantCtx.Err(), context.DeadlineExceeded) {
			result.TimedOut = true
			result.Failure = "timeout"
			break
		}
	}
	return result
}

func deliverClaimedCloudParent(ctx context.Context, lease CloudTenantDeliveryLease, sender ChannelSender, accountID string, parent *models.NotificationOutbox, options normalizedCloudTenantDeliveryOptions, result *CloudTenantDeliveryResult) error {
	if parent == nil || parent.Kind != db.NotificationOutboxKindSiteDigest {
		return retryCloudParent(ctx, lease, parent, options, "invalid_parent")
	}
	now := options.now()
	materialized, err := lease.MaterializeClaimedSiteDigest(ctx, parent.ID, options.leaseOwner, parent.Attempts, now)
	if err != nil {
		return retryCloudParent(ctx, lease, parent, options, "materialize")
	}
	if materialized == nil || materialized.Outbox == nil || materialized.Site == nil {
		return retryCloudParent(ctx, lease, parent, options, "materialize")
	}
	if len(materialized.Comments) == 0 {
		if err := lease.SkipNotificationOutboxChannelsContext(ctx, parent.ID, options.leaseOwner, parent.Attempts, now); err != nil {
			return retryCloudParent(ctx, lease, parent, options, "skip")
		}
		if err := lease.FinalizeClaimedSiteDigest(ctx, parent.ID, options.leaseOwner, parent.Attempts, now); err != nil {
			return retryCloudParent(ctx, lease, parent, options, "finalize")
		}
		result.ParentsFinalized++
		return nil
	}

	children, err := lease.ListNotificationOutboxChannelsContext(ctx, parent.ID)
	if err != nil {
		return retryCloudParent(ctx, lease, parent, options, "channels")
	}
	if len(children) == 0 {
		if len(options.channels) == 0 {
			return retryCloudParent(ctx, lease, parent, options, "not_configured")
		}
		children, err = lease.EnsureNotificationOutboxChannelsContext(ctx, parent.ID, options.channels)
		if err != nil {
			return retryCloudParent(ctx, lease, parent, options, "channels")
		}
	}
	if allCloudChildrenSent(children) {
		if err := lease.FinalizeClaimedSiteDigest(ctx, parent.ID, options.leaseOwner, parent.Attempts, now); err != nil {
			return retryCloudParent(ctx, lease, parent, options, "finalize")
		}
		result.ParentsFinalized++
		return nil
	}

	claimed, err := lease.ClaimNotificationOutboxChannelsContext(ctx, parent.ID, options.leaseOwner, now, options.childLease, options.childClaimLimit)
	if err != nil {
		return retryCloudParent(ctx, lease, parent, options, "claim_children")
	}
	if len(claimed) == 0 {
		return retryCloudParentAt(ctx, lease, parent, options, nextCloudChildBoundary(children, now, options.retryDelay), "children_pending")
	}

	tokens, err := lease.EnsureMaterializedSiteDigestApprovalTokens(ctx, materialized, db.SiteDigestApprovalTokenOptions{
		AccountID:    accountID,
		ExpiresAt:    now.Add(options.tokenTTL),
		Now:          now,
		HMACKey:      options.hmacKey,
		LocatorStore: options.locatorStore,
	})
	if err != nil {
		return retryCloudParent(ctx, lease, parent, options, "tokens")
	}
	batch, err := cloudDeliveryBatch(materialized, tokens, options.baseURL)
	if err != nil {
		return retryCloudParent(ctx, lease, parent, options, "batch")
	}

	for _, child := range claimed {
		if child == nil || child.OutboxID != parent.ID {
			continue
		}
		providerErr := sender.Send(ctx, child.Channel, batch)
		if providerErr != nil {
			result.Failure = "channel"
			retryCloudChild(ctx, lease, child, options, parent.LeaseUntil, safePersistedChannelError(child.Channel, providerErr))
			continue
		}
		if ackErr := lease.AckNotificationOutboxChannelContext(ctx, child.ID, options.leaseOwner, child.Attempts, options.now()); ackErr != nil {
			result.Failure = "retry"
			retryCloudChild(ctx, lease, child, options, parent.LeaseUntil, safePersistedChannelError(child.Channel, ackErr))
			continue
		}
		child.Status = models.NotificationOutboxChannelSent
		result.ChildrenSent++
	}

	children, err = lease.ListNotificationOutboxChannelsContext(ctx, parent.ID)
	if err != nil {
		return retryCloudParent(ctx, lease, parent, options, "channels")
	}
	if !allCloudChildrenSent(children) {
		return retryCloudParentAt(ctx, lease, parent, options, nextCloudChildBoundary(children, options.now(), options.retryDelay), "children_pending")
	}
	if err := lease.FinalizeClaimedSiteDigest(ctx, parent.ID, options.leaseOwner, parent.Attempts, options.now()); err != nil {
		return retryCloudParent(ctx, lease, parent, options, "finalize")
	}
	result.ParentsFinalized++
	return nil
}

func cloudDeliveryBatch(materialized *models.SiteDigestMaterialization, tokens []*models.ApprovalToken, baseURL string) (Batch, error) {
	if materialized == nil || materialized.Site == nil || strings.TrimSpace(baseURL) == "" {
		return Batch{}, ErrChannelInvalidRequest
	}
	byComment := make(map[string]*models.ApprovalToken, len(tokens))
	for _, token := range tokens {
		if token == nil || token.CommentID == "" {
			return Batch{}, ErrChannelInvalidRequest
		}
		if _, exists := byComment[token.CommentID]; exists {
			return Batch{}, ErrChannelInvalidRequest
		}
		byComment[token.CommentID] = token
	}
	base := strings.TrimRight(baseURL, "/")
	approve := make(map[string]string, len(materialized.Comments))
	reject := make(map[string]string, len(materialized.Comments))
	for _, comment := range materialized.Comments {
		token := byComment[comment.ID]
		if token == nil || token.Token == "" {
			return Batch{}, ErrChannelInvalidRequest
		}
		approve[comment.ID] = fmt.Sprintf("%s/approve/%s?action=approve", base, token.Token)
		reject[comment.ID] = fmt.Sprintf("%s/approve/%s?action=reject", base, token.Token)
	}
	batch := Batch{Site: materialized.Site, Comments: materialized.Comments, ApproveURLs: approve, RejectURLs: reject}
	if err := validateBatch(batch); err != nil {
		return Batch{}, ErrChannelInvalidRequest
	}
	return batch, nil
}

func allCloudChildrenSent(children []*models.NotificationOutboxChannel) bool {
	if len(children) == 0 {
		return false
	}
	for _, child := range children {
		if child == nil || (child.Status != models.NotificationOutboxChannelSent && child.Status != models.NotificationOutboxChannelSkipped) {
			return false
		}
	}
	return true
}

func nextCloudChildBoundary(children []*models.NotificationOutboxChannel, now time.Time, retryDelay time.Duration) time.Time {
	var earliest time.Time
	for _, child := range children {
		if child == nil || child.Status == models.NotificationOutboxChannelSent {
			continue
		}
		boundary := child.AvailableAt
		if child.Status == models.NotificationOutboxChannelLeased && child.LeaseUntil != nil {
			boundary = *child.LeaseUntil
		}
		if boundary.IsZero() || !boundary.After(now) {
			boundary = now.Add(retryDelay)
		}
		if earliest.IsZero() || boundary.Before(earliest) {
			earliest = boundary
		}
	}
	if earliest.IsZero() {
		return now.Add(retryDelay)
	}
	return earliest
}

func retryCloudChild(ctx context.Context, lease CloudTenantDeliveryLease, child *models.NotificationOutboxChannel, options normalizedCloudTenantDeliveryOptions, parentLeaseUntil *time.Time, lastError string) {
	if child == nil {
		return
	}
	if ctx.Err() != nil {
		return
	}
	now := options.now()
	availableAt := now.Add(options.retryDelay)
	if parentLeaseUntil != nil && parentLeaseUntil.After(availableAt) {
		availableAt = *parentLeaseUntil
	}
	if err := lease.RetryNotificationOutboxChannelContext(ctx, child.ID, options.leaseOwner, child.Attempts, availableAt, now, lastError); err != nil {
		return
	}
	child.Status = models.NotificationOutboxChannelPending
	child.AvailableAt = availableAt
	child.LeaseUntil = nil
}

func retryCloudParent(ctx context.Context, lease CloudTenantDeliveryLease, parent *models.NotificationOutbox, options normalizedCloudTenantDeliveryOptions, reason string) error {
	return retryCloudParentAt(ctx, lease, parent, options, options.now().Add(options.retryDelay), reason)
}

func retryCloudParentAt(ctx context.Context, lease CloudTenantDeliveryLease, parent *models.NotificationOutbox, options normalizedCloudTenantDeliveryOptions, availableAt time.Time, reason string) error {
	if parent == nil {
		return errors.New("invalid_parent")
	}
	// Site-digest materialization fences the parent's available_at to the
	// immutable digest window. The existing context-aware parent retry API
	// changes that value, which would make the next materialization permanently
	// invalid. Child retries carry the retry boundary; the claimed parent is
	// deliberately left leased for expiry and safe reclaim.
	_ = ctx
	_ = lease
	_ = options
	_ = availableAt
	return errors.New(reason)
}

func safePersistedChannelError(channel string, err error) string {
	safe := safeChannelError(channel, err)
	if safe == nil {
		return string(ChannelErrorProvider)
	}
	return safe.Error()
}

func cloudDeliveryFailure(ctx context.Context, err error, failure string) (string, bool) {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "timeout", true
	}
	return failure, false
}

// CloudTenantDeliveryStoreLease adapts one tenant db.Store to the narrow
// delivery lease. The release callback is normally the cache lease release;
// Release is fenced locally so it can only run once.
type CloudTenantDeliveryStoreLease struct {
	store       db.Store
	release     func() error
	releaseOnce sync.Once
	releaseErr  error
}

var _ CloudTenantDeliveryLease = (*CloudTenantDeliveryStoreLease)(nil)

func NewCloudTenantDeliveryStoreLease(store db.Store, release func() error) *CloudTenantDeliveryStoreLease {
	return &CloudTenantDeliveryStoreLease{store: store, release: release}
}

func (l *CloudTenantDeliveryStoreLease) Release() error {
	l.releaseOnce.Do(func() {
		if l.release != nil {
			l.releaseErr = l.release()
		}
	})
	return l.releaseErr
}

func (l *CloudTenantDeliveryStoreLease) ClaimNotificationOutboxContext(ctx context.Context, owner string, now time.Time, leaseFor time.Duration, limit int) ([]*models.NotificationOutbox, error) {
	return l.store.ClaimNotificationOutboxContext(ctx, owner, now, leaseFor, limit)
}
func (l *CloudTenantDeliveryStoreLease) MaterializeClaimedSiteDigest(ctx context.Context, id, owner string, generation int, now time.Time) (*models.SiteDigestMaterialization, error) {
	return l.store.MaterializeClaimedSiteDigest(ctx, id, owner, generation, now)
}
func (l *CloudTenantDeliveryStoreLease) FinalizeClaimedSiteDigest(ctx context.Context, id, owner string, generation int, now time.Time) error {
	return l.store.FinalizeClaimedSiteDigest(ctx, id, owner, generation, now)
}
func (l *CloudTenantDeliveryStoreLease) EnsureMaterializedSiteDigestApprovalTokens(ctx context.Context, materialized *models.SiteDigestMaterialization, options db.SiteDigestApprovalTokenOptions) ([]*models.ApprovalToken, error) {
	return l.store.EnsureMaterializedSiteDigestApprovalTokens(ctx, materialized, options)
}
func (l *CloudTenantDeliveryStoreLease) GetUser(ownerID string) (*models.User, error) {
	return l.store.GetUser(ownerID)
}
func (l *CloudTenantDeliveryStoreLease) GetUserContext(ctx context.Context, ownerID string) (*models.User, error) {
	return l.store.GetUserContext(ctx, ownerID)
}
func (l *CloudTenantDeliveryStoreLease) ListNotificationOutboxChannelsContext(ctx context.Context, id string) ([]*models.NotificationOutboxChannel, error) {
	return l.store.ListNotificationOutboxChannelsContext(ctx, id)
}
func (l *CloudTenantDeliveryStoreLease) EnsureNotificationOutboxChannelsContext(ctx context.Context, id string, channels []string) ([]*models.NotificationOutboxChannel, error) {
	return l.store.EnsureNotificationOutboxChannelsContext(ctx, id, channels)
}
func (l *CloudTenantDeliveryStoreLease) ClaimNotificationOutboxChannelsContext(ctx context.Context, id, owner string, now time.Time, leaseFor time.Duration, limit int) ([]*models.NotificationOutboxChannel, error) {
	return l.store.ClaimNotificationOutboxChannelsContext(ctx, id, owner, now, leaseFor, limit)
}
func (l *CloudTenantDeliveryStoreLease) AckNotificationOutboxChannelContext(ctx context.Context, id, owner string, generation int, now time.Time) error {
	return l.store.AckNotificationOutboxChannelContext(ctx, id, owner, generation, now)
}
func (l *CloudTenantDeliveryStoreLease) RetryNotificationOutboxChannelContext(ctx context.Context, id, owner string, generation int, availableAt, now time.Time, lastError string) error {
	return l.store.RetryNotificationOutboxChannelContext(ctx, id, owner, generation, availableAt, now, lastError)
}
func (l *CloudTenantDeliveryStoreLease) RetryNotificationOutboxContext(ctx context.Context, id, owner string, generation int, availableAt, now time.Time, lastError string) error {
	return l.store.RetryNotificationOutboxContext(ctx, id, owner, generation, availableAt, now, lastError)
}
func (l *CloudTenantDeliveryStoreLease) SkipNotificationOutboxChannelsContext(ctx context.Context, id, owner string, generation int, now time.Time) error {
	return l.store.SkipNotificationOutboxChannelsContext(ctx, id, owner, generation, now)
}
