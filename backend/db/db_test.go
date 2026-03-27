package db

import (
	"fmt"
	"testing"
	"time"

	"github.com/quipthread/quipthread/models"
)

func newTestStore(t *testing.T) Store {
	t.Helper()
	s, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("open in-memory store: %v", err)
	}
	t.Cleanup(func() { s.Close() }) //nolint:errcheck,gosec // deferred store cleanup in test
	return s
}

// ---- CountSites -------------------------------------------------------------

func TestCountSites(t *testing.T) {
	store := newTestStore(t)

	n, err := store.CountSites()
	if err != nil {
		t.Fatalf("CountSites (empty): %v", err)
	}
	if n != 0 {
		t.Fatalf("CountSites (empty): got %d, want 0", n)
	}

	for i := range 3 {
		s := &models.Site{ID: fmt.Sprintf("s%d", i), OwnerID: "o", Domain: "x.com"}
		if err := store.CreateSite(s); err != nil {
			t.Fatalf("CreateSite %d: %v", i, err)
		}
	}

	n, err = store.CountSites()
	if err != nil {
		t.Fatalf("CountSites after inserts: %v", err)
	}
	if n != 3 {
		t.Errorf("CountSites: got %d, want 3", n)
	}
}

// ---- CountCommentsThisMonth -------------------------------------------------

func TestCountCommentsThisMonth(t *testing.T) {
	store := newTestStore(t)

	if err := store.CreateSite(&models.Site{ID: "s1", OwnerID: "o", Domain: "x.com"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertUser(&models.User{ID: "u1", DisplayName: "T", Role: "commenter"}); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	lastMonth := now.AddDate(0, -1, 0)

	comments := []*models.Comment{
		{ID: "cur1", SiteID: "s1", PageID: "/", UserID: "u1", Content: "x", Status: "approved", CreatedAt: now},
		{ID: "cur2", SiteID: "s1", PageID: "/", UserID: "u1", Content: "x", Status: "approved", CreatedAt: now},
		{ID: "old1", SiteID: "s1", PageID: "/", UserID: "u1", Content: "x", Status: "approved", CreatedAt: lastMonth},
	}
	for _, c := range comments {
		if err := store.CreateComment(c); err != nil {
			t.Fatalf("CreateComment %s: %v", c.ID, err)
		}
	}

	n, err := store.CountCommentsThisMonth()
	if err != nil {
		t.Fatalf("CountCommentsThisMonth: %v", err)
	}
	if n != 2 {
		t.Errorf("CountCommentsThisMonth: got %d, want 2 (old comment excluded)", n)
	}
}

// ---- Blocked terms ----------------------------------------------------------

func TestBlockedTerms_CRUD(t *testing.T) {
	store := newTestStore(t)

	terms, err := store.ListBlockedTerms()
	if err != nil {
		t.Fatalf("ListBlockedTerms (empty): %v", err)
	}
	if len(terms) != 0 {
		t.Fatalf("ListBlockedTerms (empty): got %d, want 0", len(terms))
	}

	added, err := store.AddBlockedTerm("spam", false)
	if err != nil {
		t.Fatalf("AddBlockedTerm: %v", err)
	}
	if added.ID == "" {
		t.Error("AddBlockedTerm: expected non-empty ID")
	}
	if added.Term != "spam" {
		t.Errorf("AddBlockedTerm term: got %q, want spam", added.Term)
	}

	terms, err = store.ListBlockedTerms()
	if err != nil {
		t.Fatalf("ListBlockedTerms after add: %v", err)
	}
	if len(terms) != 1 {
		t.Fatalf("ListBlockedTerms after add: got %d, want 1", len(terms))
	}

	// Duplicate insert is idempotent — same ID returned.
	dup, err := store.AddBlockedTerm("spam", false)
	if err != nil {
		t.Fatalf("AddBlockedTerm duplicate: %v", err)
	}
	if dup.ID != added.ID {
		t.Errorf("AddBlockedTerm duplicate: got ID %q, want %q", dup.ID, added.ID)
	}

	if err := store.DeleteBlockedTerm(added.ID); err != nil {
		t.Fatalf("DeleteBlockedTerm: %v", err)
	}

	terms, err = store.ListBlockedTerms()
	if err != nil {
		t.Fatalf("ListBlockedTerms after delete: %v", err)
	}
	if len(terms) != 0 {
		t.Errorf("ListBlockedTerms after delete: got %d, want 0", len(terms))
	}
}

func TestBulkAddBlockedTerms(t *testing.T) {
	store := newTestStore(t)

	if _, err := store.AddBlockedTerm("existing", false); err != nil {
		t.Fatalf("AddBlockedTerm seed: %v", err)
	}

	added, err := store.BulkAddBlockedTerms([]string{"new1", "new2", "existing"})
	if err != nil {
		t.Fatalf("BulkAddBlockedTerms: %v", err)
	}
	if added != 2 {
		t.Errorf("BulkAddBlockedTerms added: got %d, want 2 (duplicate skipped)", added)
	}

	terms, err := store.ListBlockedTerms()
	if err != nil {
		t.Fatalf("ListBlockedTerms after bulk add: %v", err)
	}
	if len(terms) != 3 {
		t.Errorf("ListBlockedTerms after bulk add: got %d, want 3", len(terms))
	}
}
