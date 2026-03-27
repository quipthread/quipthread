package middleware

import (
	"testing"

	"github.com/quipthread/quipthread/db"
)

func newBlocklistStore(t *testing.T) db.Store {
	t.Helper()
	s, err := db.NewSQLiteStore(":memory:")
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
