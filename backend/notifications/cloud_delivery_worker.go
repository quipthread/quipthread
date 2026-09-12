package notifications

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// CloudTenantDeliverySummary contains only bounded operational counters and
// allowlisted failure classifications. It is safe to send to logs/metrics.
type CloudTenantDeliverySummary struct {
	Pages            int
	Accounts         int
	Tenants          int
	ParentsClaimed   int
	ParentsFinalized int
	ChildrenSent     int
	TimedOutTenants  int
	FailureCounts    map[string]int
	PassFailure      string
}

// SummarizeCloudTenantDelivery reduces a pass report and error to safe
// operational metadata. Underlying errors are intentionally discarded.
func SummarizeCloudTenantDelivery(report CloudTenantDeliveryPassReport, err error) CloudTenantDeliverySummary {
	summary := CloudTenantDeliverySummary{
		Pages: report.Pages, Accounts: report.Accounts, Tenants: len(report.Tenants),
		FailureCounts: make(map[string]int),
	}
	for _, tenant := range report.Tenants {
		summary.ParentsClaimed += tenant.ParentsClaimed
		summary.ParentsFinalized += tenant.ParentsFinalized
		summary.ChildrenSent += tenant.ChildrenSent
		if tenant.TimedOut {
			summary.TimedOutTenants++
		}
		if tenant.Failure != "" {
			summary.FailureCounts[cloudDeliveryFailureClassification(tenant.Failure)]++
		}
	}
	if err != nil {
		summary.PassFailure = classifyCloudDeliveryPassError(err)
	}
	return summary
}

func cloudDeliveryFailureClassification(failure string) string {
	switch failure {
	case "open", "claim", "materialize", "tokens", "channel", "retry", "finalize", "timeout", "release", "parent":
		return failure
	default:
		return "tenant_failure"
	}
}

func classifyCloudDeliveryPassError(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, ErrCloudTenantDeliveryInvalidPage):
		return "invalid_page"
	case errors.Is(err, ErrCloudTenantDeliveryInvalidOptions):
		return "invalid_options"
	default:
		return "pass_failure"
	}
}

type CloudTenantDeliveryPassRunner func(context.Context) (CloudTenantDeliveryPassReport, error)
type CloudTenantDeliverySummarySink func(CloudTenantDeliverySummary)

const (
	// CloudTenantDeliveryCadence is intentionally fixed for the first enabled
	// deployment. Configuration cannot turn this into a tight polling loop.
	CloudTenantDeliveryCadence         = time.Minute
	CloudTenantDeliveryShutdownTimeout = 5 * time.Second
)

// CloudTenantDeliveryWorker runs complete tenant delivery passes at a fixed
// cadence. At most one pass may be active; a tick observed during an active
// pass is discarded rather than queued.
type CloudTenantDeliveryWorker struct {
	cancel context.CancelFunc
	done   chan struct{}
	active atomic.Bool
	wg     sync.WaitGroup
}

// StartCloudTenantDeliveryWorker starts the fixed one-minute cloud delivery
// loop. The pass function must honor its context so shutdown can release
// tenant-cache leases promptly.
func StartCloudTenantDeliveryWorker(ctx context.Context, run func(context.Context) error) *CloudTenantDeliveryWorker {
	return startCloudTenantDeliveryWorker(ctx, run, CloudTenantDeliveryCadence, nil, nil)
}

// StartCloudTenantDeliveryWorkerWithSummary is the operationally observable
// variant used by startup wiring. The sink receives no raw pass/provider error.
func StartCloudTenantDeliveryWorkerWithSummary(ctx context.Context, run CloudTenantDeliveryPassRunner, sink CloudTenantDeliverySummarySink) *CloudTenantDeliveryWorker {
	if run == nil {
		return nil
	}
	return startCloudTenantDeliveryWorker(ctx, nil, CloudTenantDeliveryCadence, run, sink)
}

func startCloudTenantDeliveryWorker(ctx context.Context, run func(context.Context) error, cadence time.Duration, summary CloudTenantDeliveryPassRunner, sink CloudTenantDeliverySummarySink) *CloudTenantDeliveryWorker {
	if ctx == nil || (run == nil && summary == nil) || cadence <= 0 {
		return nil
	}
	workerCtx, cancel := context.WithCancel(ctx)
	w := &CloudTenantDeliveryWorker{cancel: cancel, done: make(chan struct{})}
	go w.loop(workerCtx, run, cadence, summary, sink)
	return w
}

func (w *CloudTenantDeliveryWorker) loop(ctx context.Context, run func(context.Context) error, cadence time.Duration, summary CloudTenantDeliveryPassRunner, sink CloudTenantDeliverySummarySink) {
	ticker := time.NewTicker(cadence)
	defer ticker.Stop()
	defer close(w.done)

	// Start an enabled worker promptly; subsequent passes follow the fixed
	// cadence. This does not change the one-minute tick interval.
	w.launch(ctx, run, summary, sink)
	for {
		select {
		case <-ticker.C:
			w.launch(ctx, run, summary, sink)
		case <-ctx.Done():
			w.wg.Wait()
			return
		}
	}
}

func (w *CloudTenantDeliveryWorker) launch(ctx context.Context, run func(context.Context) error, summary CloudTenantDeliveryPassRunner, sink CloudTenantDeliverySummarySink) {
	if ctx.Err() != nil || !w.active.CompareAndSwap(false, true) {
		return
	}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		defer w.active.Store(false)
		if summary != nil {
			report, err := summary(ctx)
			if sink != nil {
				sink(SummarizeCloudTenantDelivery(report, err))
			}
			return
		}
		_ = run(ctx)
	}()
}

// Stop cancels the worker and waits for the active pass up to ctx's deadline.
// A caller should use a bounded shutdown context; an uncooperative pass must
// not hold process shutdown indefinitely.
func (w *CloudTenantDeliveryWorker) Stop(ctx context.Context) error {
	if w == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	w.cancel()
	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
