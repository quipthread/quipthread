package notifications

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/models"
)

// dispatchCountingStore embeds db.Store with no backing implementation: any
// method that is not explicitly overridden dereferences a nil interface and
// panics, so unexpected store access during the disabled-dispatcher test fails
// the run instead of silently reading data. The five overridden methods cover
// the full store surface reachable from runDispatch.
type dispatchCountingStore struct {
	db.Store

	listSites            atomic.Int32
	countPending         atomic.Int32
	listPending          atomic.Int32
	createApprovalToken  atomic.Int32
	updateLastNotifiedAt atomic.Int32
}

func (s *dispatchCountingStore) ListSites() ([]*models.Site, error) {
	s.listSites.Add(1)
	return []*models.Site{}, nil
}

func (s *dispatchCountingStore) CountPendingComments(siteID string, since time.Time) (int, error) {
	s.countPending.Add(1)
	return 0, nil
}

func (s *dispatchCountingStore) ListPendingComments(siteID string, since time.Time) ([]*models.Comment, error) {
	s.listPending.Add(1)
	return nil, nil
}

func (s *dispatchCountingStore) CreateApprovalToken(t *models.ApprovalToken) error {
	s.createApprovalToken.Add(1)
	return nil
}

func (s *dispatchCountingStore) UpdateSiteLastNotifiedAt(siteID string, t time.Time) error {
	s.updateLastNotifiedAt.Add(1)
	return nil
}

// TestStartDispatcher_CloudModeNeverTouchesStore proves that in managed cloud
// mode the dispatcher performs zero store reads or dispatches: StartDispatcher
// fails closed before its startup pass or first tick.
func TestStartDispatcher_CloudModeNeverTouchesStore(t *testing.T) {
	st := &dispatchCountingStore{}
	cfg := &config.Config{CloudMode: true}
	ctx := context.Background()

	done := make(chan struct{})
	go func() {
		defer close(done)
		StartDispatcher(ctx, st, NewMultiNotifier(), cfg)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("StartDispatcher did not return in cloud mode; dispatcher gate failed")
	}

	if got := st.listSites.Load(); got != 0 {
		t.Fatalf("ListSites calls = %d, want 0", got)
	}
	if got := st.countPending.Load(); got != 0 {
		t.Fatalf("CountPendingComments calls = %d, want 0", got)
	}
	if got := st.listPending.Load(); got != 0 {
		t.Fatalf("ListPendingComments calls = %d, want 0", got)
	}
	if got := st.createApprovalToken.Load(); got != 0 {
		t.Fatalf("CreateApprovalToken calls = %d, want 0", got)
	}
	if got := st.updateLastNotifiedAt.Load(); got != 0 {
		t.Fatalf("UpdateSiteLastNotifiedAt calls = %d, want 0", got)
	}
}

// TestStartDispatcher_SelfHostedStillRuns proves self-hosted behavior is
// unchanged: with CloudMode=false the dispatcher still performs its immediate
// startup dispatch pass (observable via one ListSites read through the
// store), which is exactly what it did before containment.
func TestStartDispatcher_SelfHostedStillRuns(t *testing.T) {
	st := &dispatchCountingStore{}
	cfg := &config.Config{CloudMode: false}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		StartDispatcher(ctx, st, NewMultiNotifier(), cfg)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("StartDispatcher did not return on cancelled context")
	}

	// One startup runDispatch pass, then ctx.Done stops the loop. No tenant
	// content beyond the (empty in this fixture) site list is consulted
	// because there are no sites to notify.
	if got := st.listSites.Load(); got != 1 {
		t.Fatalf("ListSites calls = %d, want 1", got)
	}
	if got := st.countPending.Load(); got != 0 {
		t.Fatalf("CountPendingComments calls = %d, want 0", got)
	}
}
