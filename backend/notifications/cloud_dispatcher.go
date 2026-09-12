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
)

const (
	DefaultCloudTenantPageSize    = 50
	MaxCloudTenantPageSize        = 100
	DefaultCloudTenantConcurrency = 4
	MaxCloudTenantConcurrency     = 32
	DefaultCloudTenantTimeout     = 30 * time.Second
	MaxCloudTenantTimeout         = 5 * time.Minute
	DefaultCloudTenantClaimLimit  = 100
	MaxCloudTenantClaimLimit      = 100
	DefaultCloudTenantLease       = 5 * time.Minute
	MaxCloudTenantLease           = time.Hour
)

var (
	ErrCloudTenantPassInvalidOptions = errors.New("cloud tenant pass: invalid options")
	ErrCloudTenantPassInvalidPage    = errors.New("cloud tenant pass: invalid pagination response")
)

// CloudTenantLease is the narrow tenant seam used by the orchestration pass.
// It exposes only a count-returning outbox claim and release. It does not
// expose db.Store, comments, recipients, notification payloads, or providers.
type CloudTenantLease interface {
	Claim(ctx context.Context, owner string, now time.Time, leaseFor time.Duration, limit int) (claimed int, err error)
	Release() error
}

// CloudTenantOpener opens a lease for one tenant locator. Implementations must
// honor ctx in both opening and claiming; the dispatcher never receives a
// global/master tenant-content store.
type CloudTenantOpener func(ctx context.Context, locator cloud.AccountLocator) (CloudTenantLease, error)

// CloudAccountLocatorSource is intentionally narrower than cloud.Store. A
// cloud.Store satisfies it, while a tenant-content db.Store cannot.
type CloudAccountLocatorSource interface {
	ListAccountLocators(ctx context.Context, cursor string, limit int) (locators []*cloud.AccountLocator, nextCursor string, err error)
}

type CloudTenantPassOptions struct {
	PageSize          int
	TenantConcurrency int
	TenantTimeout     time.Duration
	ClaimLimit        int
	LeaseDuration     time.Duration
	LeaseOwner        string // test seam; empty generates a process-unique owner
	Now               func() time.Time
}

// CloudTenantResult is deliberately metadata-only. AccountID identifies the
// route; the report never contains DB URLs, outbox rows, comments, or content.
type CloudTenantResult struct {
	AccountID     string
	Opened        bool
	Claimed       int
	Released      bool
	ReleaseFailed bool
	TimedOut      bool
	Failure       string // "open", "claim", "timeout", or "release"
}

type CloudTenantPassReport struct {
	LeaseOwner string
	Pages      int
	Accounts   int
	Claimed    int
	Tenants    []CloudTenantResult
}

// RunCloudTenantDispatcherPass performs one disabled, orchestration-only
// tenant traversal. It pages control-plane locators and claims tenant-local
// outbox work, but never invokes a Notifier, acknowledges/retries work, or
// creates approval links. Per-tenant failures are recorded in the report;
// pagination and context failures are returned as pass-level errors.
func RunCloudTenantDispatcherPass(ctx context.Context, source CloudAccountLocatorSource, opener CloudTenantOpener, options CloudTenantPassOptions) (CloudTenantPassReport, error) {
	var report CloudTenantPassReport
	if ctx == nil || source == nil || opener == nil {
		return report, fmt.Errorf("%w: context, source, and opener are required", ErrCloudTenantPassInvalidOptions)
	}
	opts, err := normalizeCloudTenantPassOptions(options)
	if err != nil {
		return report, err
	}
	report.LeaseOwner = opts.leaseOwner

	cursor := ""
	for {
		if err := ctx.Err(); err != nil {
			return report, fmt.Errorf("cloud tenant pass canceled: %w", err)
		}
		locators, nextCursor, err := source.ListAccountLocators(ctx, cursor, opts.pageSize)
		if err != nil {
			return report, fmt.Errorf("list cloud tenant locators: %w", err)
		}
		if err := validateCloudLocatorPage(locators, cursor, nextCursor, opts.pageSize); err != nil {
			return report, err
		}
		report.Pages++
		report.Accounts += len(locators)

		pageResults, err := processCloudTenantPage(ctx, locators, opener, opts)
		report.Tenants = append(report.Tenants, pageResults...)
		for _, result := range pageResults {
			report.Claimed += result.Claimed
		}
		if err != nil {
			return report, err
		}
		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	sort.Slice(report.Tenants, func(i, j int) bool {
		return report.Tenants[i].AccountID < report.Tenants[j].AccountID
	})
	return report, nil
}

// RunCloudTenantPass is the concise alias used by callers that treat the
// dispatcher as a pass runner.
func RunCloudTenantPass(ctx context.Context, source CloudAccountLocatorSource, opener CloudTenantOpener, options CloudTenantPassOptions) (CloudTenantPassReport, error) {
	return RunCloudTenantDispatcherPass(ctx, source, opener, options)
}

type normalizedCloudTenantPassOptions struct {
	pageSize          int
	tenantConcurrency int
	tenantTimeout     time.Duration
	claimLimit        int
	leaseDuration     time.Duration
	leaseOwner        string
	now               func() time.Time
}

func normalizeCloudTenantPassOptions(options CloudTenantPassOptions) (normalizedCloudTenantPassOptions, error) {
	opts := normalizedCloudTenantPassOptions{
		pageSize:          options.PageSize,
		tenantConcurrency: options.TenantConcurrency,
		tenantTimeout:     options.TenantTimeout,
		claimLimit:        options.ClaimLimit,
		leaseDuration:     options.LeaseDuration,
		leaseOwner:        options.LeaseOwner,
		now:               options.Now,
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
	if opts.claimLimit == 0 {
		opts.claimLimit = DefaultCloudTenantClaimLimit
	}
	if opts.leaseDuration == 0 {
		opts.leaseDuration = DefaultCloudTenantLease
	}
	if opts.now == nil {
		opts.now = func() time.Time { return time.Now().UTC() }
	}
	if opts.leaseOwner == "" {
		opts.leaseOwner = "cloud-dispatcher-" + uuid.NewString()
	}

	if opts.pageSize < 1 || opts.pageSize > MaxCloudTenantPageSize ||
		opts.tenantConcurrency < 1 || opts.tenantConcurrency > MaxCloudTenantConcurrency ||
		opts.tenantTimeout <= 0 || opts.tenantTimeout > MaxCloudTenantTimeout ||
		opts.claimLimit < 1 || opts.claimLimit > MaxCloudTenantClaimLimit ||
		opts.leaseDuration <= 0 {
		return normalizedCloudTenantPassOptions{}, fmt.Errorf("%w: page size, concurrency, timeout, claim limit, or lease duration out of bounds", ErrCloudTenantPassInvalidOptions)
	}
	if opts.leaseDuration > MaxCloudTenantLease {
		return normalizedCloudTenantPassOptions{}, fmt.Errorf("%w: lease duration exceeds %s", ErrCloudTenantPassInvalidOptions, MaxCloudTenantLease)
	}
	return opts, nil
}

func validateCloudLocatorPage(locators []*cloud.AccountLocator, incomingCursor, nextCursor string, pageSize int) error {
	if len(locators) > pageSize {
		return fmt.Errorf("%w: received %d locators for page size %d", ErrCloudTenantPassInvalidPage, len(locators), pageSize)
	}
	if len(locators) == 0 {
		if nextCursor != "" {
			return fmt.Errorf("%w: empty page must have an empty next cursor", ErrCloudTenantPassInvalidPage)
		}
		return nil
	}
	previousID := incomingCursor
	for index, locator := range locators {
		if locator == nil {
			return fmt.Errorf("%w: locator %d is nil", ErrCloudTenantPassInvalidPage, index)
		}
		if locator.ID == "" || locator.ID != strings.TrimSpace(locator.ID) {
			return fmt.Errorf("%w: locator %d has a blank or malformed ID", ErrCloudTenantPassInvalidPage, index)
		}
		if err := cloud.ValidateAccountID(locator.ID); err != nil {
			return fmt.Errorf("%w: locator %d has an invalid account ID: %w", ErrCloudTenantPassInvalidPage, index, err)
		}
		if locator.ID <= previousID {
			return fmt.Errorf("%w: locator ID %q is not strictly beyond %q", ErrCloudTenantPassInvalidPage, locator.ID, previousID)
		}
		previousID = locator.ID
	}
	if nextCursor != "" && nextCursor != locators[len(locators)-1].ID {
		return fmt.Errorf("%w: next cursor %q must equal final returned ID %q", ErrCloudTenantPassInvalidPage, nextCursor, locators[len(locators)-1].ID)
	}
	return nil
}

func processCloudTenantPage(ctx context.Context, locators []*cloud.AccountLocator, opener CloudTenantOpener, options normalizedCloudTenantPassOptions) ([]CloudTenantResult, error) {
	if len(locators) == 0 {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("cloud tenant pass canceled: %w", err)
		}
		return []CloudTenantResult{}, nil
	}

	workerCount := options.tenantConcurrency
	if workerCount > len(locators) {
		workerCount = len(locators)
	}
	jobs := make(chan *cloud.AccountLocator)
	results := make(chan CloudTenantResult, len(locators))
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
					results <- processCloudTenant(ctx, locator, opener, options)
				}
			}
		}()
	}

	sendStopped := false
	for _, locator := range locators {
		select {
		case <-ctx.Done():
			sendStopped = true
		case jobs <- locator:
		}
		if sendStopped {
			break
		}
	}
	close(jobs)
	workers.Wait()
	close(results)

	pageResults := make([]CloudTenantResult, 0, len(locators))
	for result := range results {
		pageResults = append(pageResults, result)
	}
	if err := ctx.Err(); err != nil {
		return pageResults, fmt.Errorf("cloud tenant pass canceled: %w", err)
	}
	return pageResults, nil
}

func processCloudTenant(ctx context.Context, locator *cloud.AccountLocator, opener CloudTenantOpener, options normalizedCloudTenantPassOptions) (result CloudTenantResult) {
	result.AccountID = locator.ID
	tenantCtx, cancel := context.WithTimeout(ctx, options.tenantTimeout)
	defer cancel()

	lease, err := opener(tenantCtx, *locator)
	if err != nil {
		result.Failure, result.TimedOut = cloudTenantFailure(tenantCtx, err, "open")
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

	claimed, err := lease.Claim(tenantCtx, options.leaseOwner, options.now(), options.leaseDuration, options.claimLimit)
	if err != nil {
		result.Failure, result.TimedOut = cloudTenantFailure(tenantCtx, err, "claim")
		return result
	}
	if claimed < 0 {
		result.Failure = "claim"
		return result
	}
	result.Claimed = claimed
	return result
}

func cloudTenantFailure(ctx context.Context, err error, failure string) (string, bool) {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "timeout", true
	}
	return failure, false
}
