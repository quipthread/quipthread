package notifications

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSummarizeCloudTenantDeliveryIsSanitized(t *testing.T) {
	report := CloudTenantDeliveryPassReport{
		Pages: 1, Accounts: 1,
		Tenants: []CloudTenantDeliveryResult{{AccountID: "tenant-a", ChildrenSent: 2, Failure: "provider https://private.example/api recipient@example.test"}},
	}
	summary := SummarizeCloudTenantDelivery(report, errors.New("provider https://private.example/api recipient@example.test"))
	if summary.PassFailure != "pass_failure" || summary.FailureCounts["tenant_failure"] != 1 || summary.ChildrenSent != 2 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if strings.Contains(summary.PassFailure, "private") || strings.Contains(summary.PassFailure, "recipient") {
		t.Fatalf("summary exposed underlying error: %+v", summary)
	}
}

func TestCloudTenantDeliveryWorkerUsesOneMinuteCadence(t *testing.T) {
	if CloudTenantDeliveryCadence != time.Minute {
		t.Fatalf("cloud delivery cadence = %s, want 1m", CloudTenantDeliveryCadence)
	}
}

func TestCloudTenantDeliveryWorkerSkipsOverlappingTicksAndCancelsSafely(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 1)
	var active atomic.Int32
	var maxActive atomic.Int32
	run := func(ctx context.Context) error {
		current := active.Add(1)
		for {
			previous := maxActive.Load()
			if current <= previous || maxActive.CompareAndSwap(previous, current) {
				break
			}
		}
		started <- struct{}{}
		<-ctx.Done()
		active.Add(-1)
		return ctx.Err()
	}
	worker := startCloudTenantDeliveryWorker(ctx, run, 5*time.Millisecond, nil, nil)
	if worker == nil {
		t.Fatal("worker was not created")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	time.Sleep(30 * time.Millisecond)
	if got := maxActive.Load(); got != 1 {
		t.Fatalf("maximum concurrent passes = %d, want 1", got)
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	if err := worker.Stop(shutdownCtx); err != nil {
		t.Fatalf("worker stop: %v", err)
	}
}
