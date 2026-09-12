package middleware

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/models"
)

// listCountStore counts ListBlockedTerms calls and can inject refresh errors,
// so cache-scope behavior and fail-open semantics are observable.
type listCountStore struct {
	db.Store
	lists atomic.Int32
	err   atomic.Pointer[error]
}

func (s *listCountStore) ListBlockedTerms() ([]*models.BlockedTerm, error) {
	s.lists.Add(1)
	if e := s.err.Load(); e != nil {
		return nil, *e
	}
	return s.Store.ListBlockedTerms()
}

// ageScope force-expires one scope's cached entries so the next check must
// refresh from the DB (in-package test seam; the TTL is otherwise fixed).
func ageScope(scope string) {
	globalTermsCache.mu.Lock()
	defer globalTermsCache.mu.Unlock()
	if s, ok := globalTermsCache.scopes[scope]; ok {
		s.expires = time.Now().Add(-time.Second)
	}
}

func newBlocklistStore(t *testing.T) db.Store {
	t.Helper()
	s, err := db.NewSQLiteStoreForTest(":memory:")
	if err != nil {
		t.Fatalf("open in-memory store: %v", err)
	}
	t.Cleanup(func() { s.Close() }) //nolint:errcheck,gosec // deferred store cleanup in test
	return s
}

func TestContainsBlockedTerm_NoTerms(t *testing.T) {
	store := newBlocklistStore(t)
	InvalidateBlockedTermsCache()

	checker := NewBlockedTermsChecker(store)
	if isBlocked, _ := checker.ContainsBlockedTerm("hello world"); isBlocked {
		t.Error("ContainsBlockedTerm with empty list: expected false, got true")
	}
}

func TestContainsBlockedTerm_ExactMatch(t *testing.T) {
	store := newBlocklistStore(t)
	InvalidateBlockedTermsCache()

	if _, err := store.AddBlockedTerm("badword", false); err != nil {
		t.Fatalf("AddBlockedTerm: %v", err)
	}
	InvalidateBlockedTermsCache()

	checker := NewBlockedTermsChecker(store)
	isBlocked, matched := checker.ContainsBlockedTerm("this contains badword in it")
	if !isBlocked {
		t.Error("ContainsBlockedTerm: expected match for 'badword', got false")
	}
	if matched != "badword" {
		t.Errorf("ContainsBlockedTerm matched term: got %q, want badword", matched)
	}
}

func TestContainsBlockedTerm_CaseInsensitive(t *testing.T) {
	store := newBlocklistStore(t)
	InvalidateBlockedTermsCache()

	if _, err := store.AddBlockedTerm("spam", false); err != nil {
		t.Fatalf("AddBlockedTerm: %v", err)
	}
	InvalidateBlockedTermsCache()

	checker := NewBlockedTermsChecker(store)
	if isBlocked, _ := checker.ContainsBlockedTerm("SPAM alert"); !isBlocked {
		t.Error("ContainsBlockedTerm: expected case-insensitive match for SPAM, got false")
	}
}

func TestContainsBlockedTerm_NoMatch(t *testing.T) {
	store := newBlocklistStore(t)
	InvalidateBlockedTermsCache()

	if _, err := store.AddBlockedTerm("restricted", false); err != nil {
		t.Fatalf("AddBlockedTerm: %v", err)
	}
	InvalidateBlockedTermsCache()

	checker := NewBlockedTermsChecker(store)
	if isBlocked, _ := checker.ContainsBlockedTerm("perfectly fine comment"); isBlocked {
		t.Error("ContainsBlockedTerm: expected no match for clean comment, got true")
	}
}

func TestContainsBlockedTerm_RegexMatch(t *testing.T) {
	store := newBlocklistStore(t)
	InvalidateBlockedTermsCache()

	if _, err := store.AddBlockedTerm(`\bclick here\b`, true); err != nil {
		t.Fatalf("AddBlockedTerm regex: %v", err)
	}
	InvalidateBlockedTermsCache()

	checker := NewBlockedTermsChecker(store)
	if isBlocked, _ := checker.ContainsBlockedTerm("please click here now"); !isBlocked {
		t.Error("ContainsBlockedTerm regex: expected match for 'click here', got false")
	}
	if isBlocked, _ := checker.ContainsBlockedTerm("do not click elsewhere"); isBlocked {
		t.Error("ContainsBlockedTerm regex: expected no match for 'click elsewhere', got true")
	}
}

func TestContainsBlockedTerm_AfterDelete(t *testing.T) {
	store := newBlocklistStore(t)
	InvalidateBlockedTermsCache()

	term, err := store.AddBlockedTerm("temporary", false)
	if err != nil {
		t.Fatalf("AddBlockedTerm: %v", err)
	}
	InvalidateBlockedTermsCache()

	checker := NewBlockedTermsChecker(store)
	if isBlocked, _ := checker.ContainsBlockedTerm("temporary term"); !isBlocked {
		t.Error("ContainsBlockedTerm: expected match before delete, got false")
	}

	if err := store.DeleteBlockedTerm(term.ID); err != nil {
		t.Fatalf("DeleteBlockedTerm: %v", err)
	}
	InvalidateBlockedTermsCache()

	if isBlocked, _ := checker.ContainsBlockedTerm("temporary term"); isBlocked {
		t.Error("ContainsBlockedTerm: expected no match after delete, got true")
	}
}

// ---- Request-scoped (tenant) blocklist ---------------------------------------

func TestCheck_TenantScopesAreIsolated(t *testing.T) {
	storeA := newBlocklistStore(t)
	storeB := newBlocklistStore(t)
	InvalidateBlockedTermsCache()

	if _, err := storeA.AddBlockedTerm("badword-a", false); err != nil {
		t.Fatalf("AddBlockedTerm A: %v", err)
	}
	if _, err := storeB.AddBlockedTerm("badword-b", false); err != nil {
		t.Fatalf("AddBlockedTerm B: %v", err)
	}
	InvalidateBlockedTermsCache()

	checker := NewBlockedTermsChecker(storeB) // constructor store points at B; must not matter

	if blocked, matched := checker.Check("acc-a", storeA, "contains badword-a"); !blocked || matched != "badword-a" {
		t.Errorf("scope acc-a: blocked=%v matched=%q, want true/badword-a", blocked, matched)
	}
	if blocked, _ := checker.Check("acc-b", storeB, "contains badword-b"); !blocked {
		t.Error("scope acc-b: expected own term to block")
	}

	// No cross-scope leakage in either direction.
	if blocked, _ := checker.Check("acc-b", storeB, "contains badword-a"); blocked {
		t.Error("tenant B matched tenant A's cached term")
	}
	if blocked, _ := checker.Check("acc-a", storeA, "contains badword-b"); blocked {
		t.Error("tenant A matched tenant B's cached term")
	}
}

func TestCheck_ScopedCacheRefreshAndInvalidation(t *testing.T) {
	counted := &listCountStore{Store: newBlocklistStore(t)}
	InvalidateBlockedTermsCache()

	if _, err := counted.AddBlockedTerm("first", false); err != nil {
		t.Fatalf("AddBlockedTerm: %v", err)
	}
	checker := NewBlockedTermsChecker(counted)

	// Warm scope acc-a; the second check must come from cache.
	if blocked, _ := checker.Check("acc-a", counted, "first post"); !blocked {
		t.Fatal("warm check did not match")
	}
	before := counted.lists.Load()
	if _, _ = checker.Check("acc-a", counted, "second try"); counted.lists.Load() != before {
		t.Error("warm scope re-fetched terms from the DB")
	}

	// A different scope never inherits the warm cache, even for the same
	// store: it must perform its own fetch (and then matches, since the
	// underlying data is shared).
	lists := counted.lists.Load()
	checker.Check("acc-b", counted, "unrelated") //nolint:errcheck // warming acc-b; result irrelevant
	if counted.lists.Load() != lists+1 {
		t.Error("cold scope did not perform its own fetch")
	}

	// Scoped invalidation refreshes only the named scope.
	if _, err := counted.AddBlockedTerm("second", false); err != nil {
		t.Fatalf("AddBlockedTerm second: %v", err)
	}
	InvalidateBlockedTermsCacheFor("acc-a")
	lists = counted.lists.Load()
	if blocked, _ := checker.Check("acc-a", counted, "now with second"); !blocked {
		t.Error("invalidated scope did not see the newly added term")
	}
	if got := counted.lists.Load(); got != lists+1 {
		t.Errorf("invalidated scope fetches = %d, want exactly one refetch", got-lists)
	}
	if blocked, _ := checker.Check("acc-b", counted, "now with second"); blocked {
		t.Error("unrelated scope saw a term added after its cache was warmed")
	}
}

func TestCheck_FailOpenOnlyForFailingTenant(t *testing.T) {
	storeA := &listCountStore{Store: newBlocklistStore(t)}
	storeB := &listCountStore{Store: newBlocklistStore(t)}
	InvalidateBlockedTermsCache()

	if _, err := storeA.AddBlockedTerm("blocked", false); err != nil {
		t.Fatalf("AddBlockedTerm: %v", err)
	}
	checker := NewBlockedTermsChecker(storeB)

	if blocked, _ := checker.Check("acc-a", storeA, "a blocked comment"); !blocked {
		t.Fatal("expected pre-failure match")
	}

	// Expire tenant A's cached entries so the next check must refresh, then
	// make that refresh fail: fail-open — the comment is allowed.
	dbErr := errors.New("db unavailable")
	storeA.err.Store(&dbErr)
	ageScope("acc-a")
	if blocked, matched := checker.Check("acc-a", storeA, "a blocked comment"); blocked || matched != "" {
		t.Errorf("fail-open: blocked=%v matched=%q, want false/\"\"", blocked, matched)
	}

	// Tenant B is unaffected by A's failing store.
	if blocked, _ := checker.Check("acc-b", storeB, "anything at all"); blocked {
		t.Error("healthy tenant was blocked by another tenant's refresh error")
	}
}

func TestInvalidateBlockedTermsCache_FlushesAllScopes(t *testing.T) {
	counted := &listCountStore{Store: newBlocklistStore(t)}
	InvalidateBlockedTermsCache()

	if _, err := counted.AddBlockedTerm("term", false); err != nil {
		t.Fatalf("AddBlockedTerm: %v", err)
	}
	checker := NewBlockedTermsChecker(counted)

	// Warm both scopes.
	for _, scope := range []string{"acc-a", "acc-b"} {
		checker.Check(scope, counted, "x") //nolint:errcheck // warming the cache; result irrelevant
	}
	warm := counted.lists.Load()
	InvalidateBlockedTermsCache()
	if _, _ = checker.Check("acc-a", counted, "x"); counted.lists.Load() != warm+1 {
		t.Error("full flush did not evict acc-a's cache")
	}
}
