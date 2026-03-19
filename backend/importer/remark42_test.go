package importer

import (
	"os"
	"testing"
)

func TestParseRemark42_Basic(t *testing.T) {
	f, err := os.Open("../testdata/remark42-basic.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck // deferred close in test

	result, err := ParseRemark42(f)
	if err != nil {
		t.Fatalf("ParseRemark42: %v", err)
	}

	if len(result.Comments) != 4 {
		t.Fatalf("expected 4 comments, got %d", len(result.Comments))
	}
	// alice and bob — 2 unique users
	if len(result.Users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(result.Users))
	}

	byID := make(map[string]struct {
		pageID, pageURL, parentID string
	})
	for _, c := range result.Comments {
		byID[c.ID] = struct {
			pageID, pageURL, parentID string
		}{c.PageID, c.PageURL, c.ParentID}
	}

	r1 := byID["remark42:r1"]
	if r1.pageID != "/post" {
		t.Errorf("r1 pageID: got %q, want /post", r1.pageID)
	}
	if r1.pageURL != "https://example.com/post" {
		t.Errorf("r1 pageURL: got %q", r1.pageURL)
	}
	if r1.parentID != "" {
		t.Errorf("r1 parentID: expected empty, got %q", r1.parentID)
	}

	r3 := byID["remark42:r3"]
	if r3.parentID != "remark42:r1" {
		t.Errorf("r3 parentID: got %q, want remark42:r1", r3.parentID)
	}

	r4 := byID["remark42:r4"]
	if r4.pageID != "/other-page" {
		t.Errorf("r4 pageID: got %q, want /other-page", r4.pageID)
	}
}

func TestParseRemark42_Edge(t *testing.T) {
	f, err := os.Open("../testdata/remark42-edge.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck // deferred close in test

	result, err := ParseRemark42(f)
	if err != nil {
		t.Fatalf("ParseRemark42: %v", err)
	}

	// deleted (e1) excluded; e2 and e3 included
	if len(result.Comments) != 2 {
		t.Fatalf("expected 2 comments (e2, e3), got %d", len(result.Comments))
	}

	byID := make(map[string]struct{ pageID, userID string })
	for _, c := range result.Comments {
		if c.ID == "remark42:e1" {
			t.Error("deleted comment e1 should be excluded")
		}
		byID[c.ID] = struct{ pageID, userID string }{c.PageID, c.UserID}
	}

	// missing locator URL → pageIDFromURL("") = "/"
	e2 := byID["remark42:e2"]
	if e2.pageID != "/" {
		t.Errorf("e2 pageID: got %q, want / (empty locator)", e2.pageID)
	}

	// missing user ID → falls back to user name as dedup key
	e3 := byID["remark42:e3"]
	wantUserID := syntheticUserID("remark42", "Eve Green")
	if e3.userID != wantUserID {
		t.Errorf("e3 userID: got %q, want %q", e3.userID, wantUserID)
	}
}
