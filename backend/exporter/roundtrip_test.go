package exporter

import (
	"bytes"
	"testing"
	"time"

	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/importer"
	"github.com/quipthread/quipthread/models"
)

func newTestStore(t *testing.T) db.Store {
	t.Helper()
	s, err := db.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("open in-memory store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNativeRoundTrip(t *testing.T) {
	store := newTestStore(t)

	if err := store.CreateSite(&models.Site{ID: "site1", OwnerID: "owner", Domain: "example.com"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertUser(&models.User{ID: "u1", DisplayName: "Alice Smith", AvatarURL: "https://example.com/alice.jpg", Role: "commenter"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertUser(&models.User{ID: "u2", DisplayName: "Bob Jones", Role: "commenter"}); err != nil {
		t.Fatal(err)
	}

	base := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	comments := []*models.Comment{
		{ID: "c1", SiteID: "site1", PageID: "/hello", PageURL: "https://example.com/hello", PageTitle: "Hello", UserID: "u1", Content: "<p>Hello!</p>", Status: "approved", CreatedAt: base},
		{ID: "c2", SiteID: "site1", PageID: "/hello", PageURL: "https://example.com/hello", PageTitle: "Hello", ParentID: "c1", UserID: "u2", Content: "<p>Reply.</p>", Status: "approved", CreatedAt: base.Add(time.Hour)},
		{ID: "c3", SiteID: "site1", PageID: "/other", PageURL: "https://example.com/other", PageTitle: "Other", UserID: "u1", Content: "<p>Other page.</p>", Status: "pending", CreatedAt: base.Add(2 * time.Hour)},
		{ID: "c4", SiteID: "site1", PageID: "/hello", UserID: "u1", Content: "<p>Old.</p>", Status: "approved", CreatedAt: base.Add(-24 * time.Hour)},
		{ID: "c5", SiteID: "site1", PageID: "/hello", UserID: "u1", Content: "<p>Far future.</p>", Status: "approved", CreatedAt: base.Add(48 * time.Hour)},
	}
	for _, c := range comments {
		if err := store.CreateComment(c); err != nil {
			t.Fatalf("create comment %s: %v", c.ID, err)
		}
	}

	site, _ := store.GetSite("site1")

	t.Run("status filter excludes pending", func(t *testing.T) {
		exported, err := store.ExportComments("site1", db.ExportFilter{Status: "approved"})
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range exported {
			if c.Status != "approved" {
				t.Errorf("status filter: got comment %s with status %q", c.ID, c.Status)
			}
		}
		ids := commentIDs(exported)
		if ids["c3"] {
			t.Error("pending comment c3 should be excluded by status filter")
		}
	})

	t.Run("status=all includes pending", func(t *testing.T) {
		exported, err := store.ExportComments("site1", db.ExportFilter{Status: "all"})
		if err != nil {
			t.Fatal(err)
		}
		ids := commentIDs(exported)
		if !ids["c3"] {
			t.Error("status=all should include pending comment c3")
		}
	})

	t.Run("pageId filter", func(t *testing.T) {
		exported, err := store.ExportComments("site1", db.ExportFilter{Status: "all", PageID: "/other"})
		if err != nil {
			t.Fatal(err)
		}
		if len(exported) != 1 || exported[0].ID != "c3" {
			t.Errorf("pageId filter: expected [c3], got %v", commentIDs(exported))
		}
	})

	t.Run("from/to date range", func(t *testing.T) {
		from := base.Add(-12 * time.Hour)
		to := base.Add(24 * time.Hour)
		exported, err := store.ExportComments("site1", db.ExportFilter{Status: "all", From: &from, To: &to})
		if err != nil {
			t.Fatal(err)
		}
		ids := commentIDs(exported)
		if ids["c4"] {
			t.Error("c4 predates 'from' and should be excluded")
		}
		if ids["c5"] {
			t.Error("c5 is after 'to' and should be excluded")
		}
		if !ids["c1"] || !ids["c2"] || !ids["c3"] {
			t.Error("c1, c2, c3 should be within the date range")
		}
	})

	t.Run("author name populated from users join", func(t *testing.T) {
		exported, err := store.ExportComments("site1", db.ExportFilter{Status: "approved"})
		if err != nil {
			t.Fatal(err)
		}
		byID := make(map[string]*models.Comment)
		for _, c := range exported {
			byID[c.ID] = c
		}
		if byID["c1"].AuthorName != "Alice Smith" {
			t.Errorf("c1 AuthorName: got %q, want Alice Smith", byID["c1"].AuthorName)
		}
		if byID["c2"].AuthorName != "Bob Jones" {
			t.Errorf("c2 AuthorName: got %q, want Bob Jones", byID["c2"].AuthorName)
		}
	})

	t.Run("write/parse round-trip", func(t *testing.T) {
		exported, err := store.ExportComments("site1", db.ExportFilter{Status: "all"})
		if err != nil {
			t.Fatal(err)
		}

		var buf bytes.Buffer
		if err := WriteNative(&buf, exported, site); err != nil {
			t.Fatalf("WriteNative: %v", err)
		}

		parsed, err := importer.ParseNative(&buf)
		if err != nil {
			t.Fatalf("ParseNative: %v", err)
		}

		if len(parsed.Comments) != len(exported) {
			t.Fatalf("round-trip: exported %d comments, parsed %d", len(exported), len(parsed.Comments))
		}

		byID := make(map[string]*models.Comment)
		for _, c := range exported {
			byID[c.ID] = c
		}
		for _, pc := range parsed.Comments {
			orig, ok := byID[pc.ID]
			if !ok {
				t.Errorf("round-trip: unexpected comment ID %q", pc.ID)
				continue
			}
			if pc.Content != orig.Content {
				t.Errorf("round-trip %s: content mismatch", pc.ID)
			}
			if pc.PageID != orig.PageID {
				t.Errorf("round-trip %s: pageID mismatch", pc.ID)
			}
			if pc.ParentID != orig.ParentID {
				t.Errorf("round-trip %s: parentID mismatch %q vs %q", pc.ID, pc.ParentID, orig.ParentID)
			}
			if !pc.CreatedAt.Equal(orig.CreatedAt) {
				t.Errorf("round-trip %s: createdAt mismatch %v vs %v", pc.ID, pc.CreatedAt, orig.CreatedAt)
			}
		}
	})
}

func commentIDs(comments []*models.Comment) map[string]bool {
	m := make(map[string]bool, len(comments))
	for _, c := range comments {
		m[c.ID] = true
	}
	return m
}
