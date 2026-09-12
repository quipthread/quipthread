package middleware

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/quipthread/quipthread/db"
)

func TestStoreCache_GetOrOpenContextCancelsSlowOpen(t *testing.T) {
	c := NewStoreCache()
	defer c.Close() //nolint:errcheck // test cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	started := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		_, err := c.GetOrOpenContext(ctx, "slow", "turso\x00slow", func(ctx context.Context) (db.Store, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		})
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("slow opener did not start")
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("GetOrOpenContext error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("GetOrOpenContext did not honor cancellation")
	}
}

func TestStoreCache_GetOrOpenContextClosesStoreReturnedAfterCancellation(t *testing.T) {
	c := NewStoreCache()
	defer c.Close() //nolint:errcheck // test cleanup
	ctx, cancel := context.WithCancel(context.Background())
	opened := &fakeStore{}
	started := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		_, err := c.GetOrOpenContext(ctx, "partial", "turso\x00partial", func(context.Context) (db.Store, error) {
			close(started)
			<-ctx.Done()
			return opened, nil
		})
		result <- err
	}()
	<-started
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("GetOrOpenContext error = %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for !opened.closed() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !opened.closed() {
		t.Fatal("partially opened store was not closed")
	}
}

// fakeStore is a cache-test double: only Close is meaningful, tracked with a
// close counter so leaks and double-closes are observable.
type fakeStore struct {
	db.Store
	closes atomic.Int32
}

func (f *fakeStore) Close() error {
	f.closes.Add(1)
	return nil
}

func (f *fakeStore) closed() bool { return f.closes.Load() > 0 }

// fpStore tags a fake store with the fingerprint it was opened for, so
// mixed-fingerprint races can assert every caller got its own target.
type fpStore struct {
	fakeStore
	fp string
}

func TestStorageFingerprintDoesNotContainTenantURL(t *testing.T) {
	url := "libsql://tenant.example.test?authToken=private-token"
	fingerprint := StorageFingerprint("turso", url)
	if strings.Contains(fingerprint, "tenant.example.test") || strings.Contains(fingerprint, "private-token") {
		t.Fatalf("storage fingerprint exposed tenant target: %q", fingerprint)
	}
}

// countingOpener hands out pre-built stores (or fails for the first `failures`
// calls) and records how many times it was invoked.
type countingOpener struct {
	mu       sync.Mutex
	stores   []*fakeStore
	failures int
	calls    int
}

func newCountingOpener(n, failures int) *countingOpener {
	o := &countingOpener{failures: failures}
	for i := 0; i < n; i++ {
		o.stores = append(o.stores, &fakeStore{})
	}
	return o
}

func (o *countingOpener) open() (db.Store, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.calls < o.failures {
		o.calls++
		return nil, errors.New("open failed")
	}
	if o.calls >= len(o.stores)+o.failures {
		o.calls++
		return nil, errors.New("opener exhausted")
	}
	s := o.stores[o.calls-o.failures]
	o.calls++
	return s, nil
}

func (o *countingOpener) callCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.calls
}

// newTestCache builds a cache driven by a manual clock for deterministic TTL
// and LRU behavior.
func newTestCache(capacity int, ttl time.Duration) (*StoreCache, *atomic.Int64) {
	var now atomic.Int64
	c := NewStoreCacheWithLimits(capacity, ttl)
	c.nowFn = func() time.Time { return time.Unix(now.Load(), 0) }
	return c, &now
}

const fp1 = "sqlite\x00/tenants/a.db"
const fp2 = "turso\x00libsql://a.turso.io"

func TestStoreCache_GetOrOpenCachesAndReuses(t *testing.T) {
	c, _ := newTestCache(4, time.Hour)
	defer c.Close() //nolint:errcheck // test cleanup
	o := newCountingOpener(2, 0)

	l1, err := c.GetOrOpen("acc", fp1, o.open)
	if err != nil || l1.Store() == nil {
		t.Fatalf("first GetOrOpen = (%v, %v)", l1, err)
	}
	l2, err := c.GetOrOpen("acc", fp1, o.open)
	if err != nil {
		t.Fatalf("second GetOrOpen: %v", err)
	}
	if l1.Store() != l2.Store() {
		t.Error("cache miss on identical fingerprint")
	}
	if got := o.callCount(); got != 1 {
		t.Errorf("opener called %d times, want 1", got)
	}
	l1.Release()
	l2.Release()
}

func TestStoreCache_SingleflightColdBurstOpensOnce(t *testing.T) {
	c, _ := newTestCache(8, time.Hour)
	defer c.Close() //nolint:errcheck // test cleanup

	var calls atomic.Int32
	release := make(chan struct{})
	var opened atomic.Int32
	opener := func() (db.Store, error) {
		calls.Add(1)
		<-release // hold the opener so the burst piles onto singleflight
		opened.Add(1)
		return &fakeStore{}, nil
	}

	const burst = 32
	started := make(chan struct{}, burst)
	var wg sync.WaitGroup
	results := make([]db.Store, burst)
	errs := make([]error, burst)
	for i := 0; i < burst; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			started <- struct{}{}
			lease, err := c.GetOrOpen("acc", fp1, opener)
			errs[i] = err
			if lease != nil {
				results[i] = lease.Store()
				defer lease.Release()
			}
		}(i)
	}
	// Every goroutine has invoked GetOrOpen; give the winner time to block
	// inside the opener and the rest to pile onto the singleflight wait.
	for i := 0; i < burst; i++ {
		<-started
	}
	for calls.Load() < 1 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
		if results[i] != results[0] {
			t.Fatalf("goroutine %d received a different store instance", i)
		}
	}
	if got := opened.Load(); got != 1 {
		t.Errorf("cold burst opened %d stores, want exactly 1", got)
	}
}

func TestStoreCache_FailedOpensNotCachedAndRetryable(t *testing.T) {
	c, _ := newTestCache(4, time.Hour)
	defer c.Close() //nolint:errcheck // test cleanup
	o := newCountingOpener(3, 2)

	for i := 0; i < 2; i++ {
		lease, err := c.GetOrOpen("acc", fp1, o.open)
		if err == nil || lease != nil {
			t.Fatalf("attempt %d: expected failure, got (%v, %v)", i+1, lease, err)
		}
	}
	lease, err := c.GetOrOpen("acc", fp1, o.open)
	if err != nil || lease == nil {
		t.Fatalf("retry after failures: (%v, %v), want success", lease, err)
	}
	s := lease.Store()
	lease.Release()
	if got := o.callCount(); got != 3 {
		t.Errorf("opener called %d times, want 3 (failures must not be cached)", got)
	}
	// The successful open is now cached.
	if again, _ := c.GetOrOpen("acc", fp1, o.open); again.Store() != s {
		t.Error("successful open was not cached")
	}
	// The only unused pre-built store must never have been touched.
	if unused := o.stores[1]; unused.closes.Load() != 0 {
		t.Error("store never returned by the opener was closed")
	}
}

func TestStoreCache_TTLEviction(t *testing.T) {
	c, now := newTestCache(4, 30*time.Second)
	defer c.Close() //nolint:errcheck // test cleanup
	o := newCountingOpener(2, 0)

	l1, err := c.GetOrOpen("acc", fp1, o.open)
	if err != nil {
		t.Fatalf("initial open: %v", err)
	}
	s1 := l1.Store()

	// Just inside the TTL window: still a hit (and it refreshes lastUsed).
	now.Add(29)
	if l, _ := c.GetOrOpen("acc", fp1, o.open); l.Store() != s1 {
		t.Error("store evicted before TTL expiry")
	} else {
		l.Release()
	}

	// Past the TTL measured from the refreshed lastUsed: lazy eviction,
	// reopen, old store closed.
	l1.Release()
	now.Add(31)
	l2, err := c.GetOrOpen("acc", fp1, o.open)
	if err != nil {
		t.Fatalf("reopen after expiry: %v", err)
	}
	defer l2.Release()
	if l2.Store() == s1 {
		t.Error("expired entry was reused")
	}
	if !s1.(*fakeStore).closed() {
		t.Error("expired store was not closed after release")
	}
	if got := o.callCount(); got != 2 {
		t.Errorf("opener called %d times, want 2", got)
	}
	l1.Release()
}

func TestStoreCache_LRUCapacityEviction(t *testing.T) {
	c, now := newTestCache(2, time.Hour)
	defer c.Close() //nolint:errcheck // test cleanup
	oA := newCountingOpener(1, 0)
	oB := newCountingOpener(1, 0)
	oC := newCountingOpener(1, 0)

	lA, _ := c.GetOrOpen("acc-a", fp1, oA.open)
	defer lA.Release()
	sA := lA.Store()
	lB, _ := c.GetOrOpen("acc-b", fp1, oB.open)
	sB := lB.Store()
	lB.Release() // acc-b is idle; it must be the LRU victim

	// Touch acc-a so acc-b becomes the LRU victim.
	now.Add(10)
	if again, _ := c.GetOrOpen("acc-a", fp1, oA.open); again.Store() != sA {
		t.Fatal("touching acc-a did not hit its cache entry")
	} else {
		again.Release()
	}

	lC, err := c.GetOrOpen("acc-c", fp1, oC.open)
	if err != nil {
		t.Fatalf("open acc-c: %v", err)
	}
	defer lC.Release()
	sC := lC.Store()

	if !sB.(*fakeStore).closed() {
		t.Error("LRU victim (acc-b) was not closed on capacity eviction")
	}
	if sA.(*fakeStore).closed() {
		t.Error("recently used store (acc-a) was evicted instead of the LRU victim")
	}
	// Survivors are still served from cache without reopening.
	if again, _ := c.GetOrOpen("acc-a", fp1, oA.open); again.Store() != sA {
		t.Error("acc-a did not survive capacity eviction")
	} else {
		again.Release()
	}
	if again, _ := c.GetOrOpen("acc-c", fp1, oC.open); again.Store() != sC {
		t.Error("acc-c did not survive capacity eviction")
	} else {
		again.Release()
	}
	if got := oA.callCount() + oB.callCount() + oC.callCount(); got != 3 {
		t.Errorf("total opens = %d, want 3", got)
	}
}

func TestStoreCache_FingerprintChangeReplacesStore(t *testing.T) {
	c, _ := newTestCache(4, time.Hour)
	defer c.Close() //nolint:errcheck // test cleanup
	o := newCountingOpener(2, 0)

	old, err := c.GetOrOpen("acc", fp1, o.open)
	if err != nil {
		t.Fatalf("initial open: %v", err)
	}
	oldStore := old.Store()
	old.Release() // no active borrower: replacement may close immediately

	fresh, err := c.GetOrOpen("acc", fp2, o.open)
	if err != nil {
		t.Fatalf("open after fingerprint change: %v", err)
	}
	defer fresh.Release()
	if fresh.Store() == oldStore {
		t.Fatal("cached store reused across a storage fingerprint change")
	}
	if !oldStore.(*fakeStore).closed() {
		t.Error("replaced store was not closed")
	}
	if fresh.Store().(*fakeStore).closed() {
		t.Error("replacement store was closed")
	}
	// The replacement is now the cached entry.
	if again, _ := c.GetOrOpen("acc", fp2, o.open); again.Store() != fresh.Store() {
		t.Error("replacement store was not retained in cache")
	} else {
		again.Release()
	}
	if got := o.callCount(); got != 2 {
		t.Errorf("opener called %d times, want 2", got)
	}
}

func TestStoreCache_EvictClosesAndIsIdempotent(t *testing.T) {
	c, _ := newTestCache(4, time.Hour)
	defer c.Close() //nolint:errcheck // test cleanup
	o := newCountingOpener(2, 0)

	lease, _ := c.GetOrOpen("acc", fp1, o.open)
	s := lease.Store()
	lease.Release()
	if !c.Evict("acc") {
		t.Error("Evict reported no entry despite a cached store")
	}
	if !s.(*fakeStore).closed() {
		t.Error("evicted store was not closed")
	}
	if c.Evict("acc") {
		t.Error("second Evict reported an entry that no longer exists")
	}
	if s.(*fakeStore).closes.Load() != 1 {
		t.Errorf("evicted store closed %d times, want 1", s.(*fakeStore).closes.Load())
	}

	// After eviction the next GetOrOpen opens a fresh store.
	fresh, err := c.GetOrOpen("acc", fp1, o.open)
	if err != nil || fresh.Store() == s {
		t.Fatalf("post-evict GetOrOpen = (%v, %v), want a newly opened store", fresh, err)
	}
	fresh.Release()
}

// ---- Lease semantics ---------------------------------------------------------

func TestStoreCache_LeasedEntrySurvivesCapacityUntilRelease(t *testing.T) {
	c, _ := newTestCache(1, time.Hour)
	defer c.Close() //nolint:errcheck // test cleanup
	oA := newCountingOpener(1, 0)
	oB := newCountingOpener(1, 0)

	leaseA, err := c.GetOrOpen("acc-a", fp1, oA.open)
	if err != nil {
		t.Fatalf("open acc-a: %v", err)
	}
	storeA := leaseA.Store().(*fakeStore)

	// Capacity 1: acquiring acc-b retires acc-a's entry — but acc-a is actively
	// borrowed, so its store must stay open.
	leaseB, err := c.GetOrOpen("acc-b", fp1, oB.open)
	if err != nil {
		t.Fatalf("open acc-b: %v", err)
	}
	if storeA.closed() {
		t.Fatal("actively borrowed store was closed on capacity eviction")
	}

	leaseA.Release()
	if !storeA.closed() {
		t.Error("retired store was not closed after final release")
	}
	if storeA.closes.Load() != 1 {
		t.Errorf("retired store closed %d times, want exactly 1", storeA.closes.Load())
	}

	// acc-b was never retired; releasing it must not close it.
	leaseB.Release()
	if leaseB.Store().(*fakeStore).closed() {
		t.Error("live entry store was closed by mere release")
	}
}

func TestStoreCache_LeasedEntrySurvivesTTLUntilRelease(t *testing.T) {
	c, now := newTestCache(4, 10*time.Second)
	defer c.Close() //nolint:errcheck // test cleanup
	o := newCountingOpener(2, 0)

	lease, err := c.GetOrOpen("acc", fp1, o.open)
	if err != nil {
		t.Fatalf("initial open: %v", err)
	}
	store := lease.Store().(*fakeStore)

	// Expire the entry while the lease is active.
	now.Add(11)
	next, err := c.GetOrOpen("acc", fp1, o.open)
	if err != nil {
		t.Fatalf("reopen after expiry: %v", err)
	}
	defer next.Release()
	if next.Store() == lease.Store() {
		t.Fatal("expired entry was reused")
	}
	if store.closed() {
		t.Fatal("actively borrowed store was closed on TTL expiry")
	}

	lease.Release()
	if !store.closed() || store.closes.Load() != 1 {
		t.Errorf("post-release closes = %d, want exactly 1", store.closes.Load())
	}
}

func TestStoreCache_LeasedEntrySurvivesEvictUntilRelease(t *testing.T) {
	c, _ := newTestCache(4, time.Hour)
	defer c.Close() //nolint:errcheck // test cleanup
	o := newCountingOpener(1, 0)

	lease, err := c.GetOrOpen("acc", fp1, o.open)
	if err != nil {
		t.Fatalf("initial open: %v", err)
	}
	store := lease.Store().(*fakeStore)

	if !c.Evict("acc") {
		t.Fatal("Evict reported no entry despite a cached store")
	}
	if store.closed() {
		t.Fatal("actively borrowed store was closed by Evict")
	}

	lease.Release()
	if !store.closed() || store.closes.Load() != 1 {
		t.Errorf("post-release closes = %d, want exactly 1", store.closes.Load())
	}
	lease.Release() // idempotence
	if store.closes.Load() != 1 {
		t.Errorf("double Release closed store %d times, want 1", store.closes.Load())
	}
}

func TestStoreCache_CloseRespectsLeases(t *testing.T) {
	c, _ := newTestCache(8, time.Hour)
	o := newCountingOpener(2, 0)

	l1, err := c.GetOrOpen("a", fp1, o.open)
	if err != nil {
		t.Fatalf("open a: %v", err)
	}
	l2, err := c.GetOrOpen("b", fp1, o.open)
	if err != nil {
		t.Fatalf("open b: %v", err)
	}
	s1, s2 := l1.Store().(*fakeStore), l2.Store().(*fakeStore)

	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if s1.closed() || s2.closed() {
		t.Fatal("Close closed an actively borrowed store")
	}

	l1.Release()
	if !s1.closed() || s1.closes.Load() != 1 {
		t.Errorf("borrowed store closed %d times after release, want 1", s1.closes.Load())
	}
	l2.Release()
	if !s2.closed() || s2.closes.Load() != 1 {
		t.Errorf("borrowed store closed %d times after release, want 1", s2.closes.Load())
	}

	// Terminal state preserved after leased shutdown.
	if _, err := c.GetOrOpen("c", fp1, o.open); !errors.Is(err, ErrStoreCacheClosed) {
		t.Errorf("GetOrOpen after Close = %v, want ErrStoreCacheClosed", err)
	}
}

func TestStoreCache_CloseDuringInFlightOpenClosesOpenerResult(t *testing.T) {
	c, _ := newTestCache(4, time.Hour)

	// The opener closes the cache before returning its store, simulating a
	// shutdown racing an in-flight open (probe passed, then Close): the result
	// cannot be retained and must be closed by the cache instead of leaking.
	var produced *fakeStore
	opener := func() (db.Store, error) {
		produced = &fakeStore{}
		if err := c.Close(); err != nil {
			return nil, err
		}
		return produced, nil
	}
	lease, err := c.GetOrOpen("acc", fp1, opener)
	if !errors.Is(err, ErrStoreCacheClosed) {
		t.Fatalf("GetOrOpen = (%v, %v), want ErrStoreCacheClosed", lease, err)
	}
	if lease != nil && lease.Store() != nil {
		t.Errorf("unretained store leaked out of GetOrOpen: %v", lease.Store())
	}
	if produced == nil || produced.closes.Load() != 1 {
		t.Errorf("unretained opener result closed %d times, want 1", produced.closes.Load())
	}
	if err := c.Close(); err != nil {
		t.Errorf("Close after in-flight race = %v, want nil", err)
	}
}

func TestStoreCache_ConcurrentMixedFingerprintsNeverWrongStore(t *testing.T) {
	for round := 0; round < 20; round++ {
		c, now := newTestCache(4, time.Hour)
		openers := map[string]func() (db.Store, error){
			fp1: func() (db.Store, error) { return &fpStore{fp: fp1}, nil },
			fp2: func() (db.Store, error) { return &fpStore{fp: fp2}, nil },
		}

		const workers = 16
		start := make(chan struct{})
		var wg sync.WaitGroup
		errCh := make(chan error, workers)
		for w := 0; w < workers; w++ {
			fp := fp1
			if w%2 == 1 {
				fp = fp2
			}
			wg.Add(1)
			go func(fp string) {
				defer wg.Done()
				<-start
				lease, err := c.GetOrOpen("acc", fp, openers[fp])
				if err != nil {
					errCh <- fmt.Errorf("GetOrOpen(%q): %w", fp, err)
					return
				}
				defer lease.Release()
				got, ok := lease.Store().(*fpStore)
				if !ok {
					errCh <- fmt.Errorf("GetOrOpen(%q): unexpected store type %T", fp, lease.Store())
					return
				}
				if got.fp != fp {
					errCh <- fmt.Errorf("fingerprint confusion: requested %q got %q", fp, got.fp)
				}
				now.Add(1)
			}(fp)
		}
		close(start)
		wg.Wait()
		close(errCh)
		for err := range errCh {
			t.Error(err)
		}
		if err := c.Close(); err != nil {
			t.Fatalf("round %d Close: %v", round, err)
		}
	}
}

func TestStoreCache_ConcurrentMixedTrafficNoLeak(t *testing.T) {
	c, now := newTestCache(4, 5*time.Second)
	o := newCountingOpener(1024, 0)

	const workers = 16
	const iters = 20
	accounts := []string{"a", "b", "c", "d", "e"}
	fps := []string{fp1, fp2}

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				id := accounts[(w+i)%len(accounts)]
				fp := fps[(w/2+i)%len(fps)]
				lease, err := c.GetOrOpen(id, fp, o.open)
				if err != nil {
					t.Errorf("GetOrOpen(%s): %v", id, err)
					return
				}
				if i%7 == 0 {
					c.Evict(id)
				}
				now.Add(1)
				lease.Release()
			}
		}(w)
	}
	wg.Wait()

	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Every store handed out by the opener must have been closed exactly once:
	// no leaked connections, no double closes.
	produced := o.callCount() // opener never fails in this test
	totalCloses := int32(0)
	for _, s := range o.stores[:produced] {
		switch n := s.closes.Load(); n {
		case 0:
			t.Error("opener-produced store was never closed (leak)")
		case 1:
			totalCloses += n
		default:
			t.Errorf("store closed %d times, want at most 1", n)
		}
	}
	t.Logf("opened=%d closed_exactly_once=%d", produced, totalCloses)
}
