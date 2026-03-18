package importer

import (
	"os"
	"testing"
	"time"
)

func TestParseDisqus_Basic(t *testing.T) {
	f, err := os.Open("../testdata/disqus-basic.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	result, err := ParseDisqus(f)
	if err != nil {
		t.Fatalf("ParseDisqus: %v", err)
	}

	if len(result.Comments) != 3 {
		t.Fatalf("expected 3 comments, got %d", len(result.Comments))
	}
	if len(result.Users) != 2 {
		t.Fatalf("expected 2 users (alice deduped), got %d", len(result.Users))
	}

	byID := make(map[string]*struct {
		id, pageID, pageURL, pageTitle, parentID, userID string
		createdAt                                        time.Time
	})
	for _, c := range result.Comments {
		byID[c.ID] = &struct {
			id, pageID, pageURL, pageTitle, parentID, userID string
			createdAt                                        time.Time
		}{c.ID, c.PageID, c.PageURL, c.PageTitle, c.ParentID, c.UserID, c.CreatedAt}
	}

	c2001, ok := byID["disqus:2001"]
	if !ok {
		t.Fatal("missing comment disqus:2001")
	}
	if c2001.pageID != "/hello-world" {
		t.Errorf("2001 pageID: got %q, want %q", c2001.pageID, "/hello-world")
	}
	if c2001.pageURL != "https://example.com/hello-world" {
		t.Errorf("2001 pageURL: got %q, want %q", c2001.pageURL, "https://example.com/hello-world")
	}
	if c2001.pageTitle != "Hello World" {
		t.Errorf("2001 pageTitle: got %q", c2001.pageTitle)
	}
	if c2001.parentID != "" {
		t.Errorf("2001 parentID: expected empty, got %q", c2001.parentID)
	}
	want2001 := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	if !c2001.createdAt.Equal(want2001) {
		t.Errorf("2001 createdAt: got %v, want %v", c2001.createdAt, want2001)
	}

	c2002, ok := byID["disqus:2002"]
	if !ok {
		t.Fatal("missing comment disqus:2002")
	}
	if c2002.parentID != "disqus:2001" {
		t.Errorf("2002 parentID: got %q, want %q", c2002.parentID, "disqus:2001")
	}

	c2003, ok := byID["disqus:2003"]
	if !ok {
		t.Fatal("missing comment disqus:2003")
	}
	if c2003.pageID != "/another-post" {
		t.Errorf("2003 pageID: got %q, want %q", c2003.pageID, "/another-post")
	}

	// alice and bob must map to the same user IDs across comments
	if c2001.userID != c2003.userID {
		t.Errorf("alice's userID inconsistent: %q vs %q", c2001.userID, c2003.userID)
	}
	if c2001.userID == c2002.userID {
		t.Error("alice and bob should have different user IDs")
	}
}

func TestParseDisqus_Edge(t *testing.T) {
	f, err := os.Open("../testdata/disqus-edge.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	result, err := ParseDisqus(f)
	if err != nil {
		t.Fatalf("ParseDisqus: %v", err)
	}

	// deleted (4002) and spam (4003) must be excluded
	if len(result.Comments) != 3 {
		t.Fatalf("expected 3 comments (4001, 4004, 4005), got %d", len(result.Comments))
	}

	byID := make(map[string]bool)
	for _, c := range result.Comments {
		byID[c.ID] = true
	}
	if byID["disqus:4002"] {
		t.Error("deleted comment 4002 should be excluded")
	}
	if byID["disqus:4003"] {
		t.Error("spam comment 4003 should be excluded")
	}

	// orphaned post: thread 9999 has no <thread> element — page fields empty/fallback
	var orphan *struct{ pageID, pageURL, pageTitle string }
	for _, c := range result.Comments {
		if c.ID == "disqus:4004" {
			orphan = &struct{ pageID, pageURL, pageTitle string }{c.PageID, c.PageURL, c.PageTitle}
		}
	}
	if orphan == nil {
		t.Fatal("missing orphaned comment disqus:4004")
	}
	if orphan.pageID != "/" {
		t.Errorf("orphaned pageID: got %q, want %q", orphan.pageID, "/")
	}
	if orphan.pageURL != "" {
		t.Errorf("orphaned pageURL: got %q, want empty", orphan.pageURL)
	}

	// anonymous post: no name/username falls back to "anonymous" key
	var anon *struct{ userID string }
	for _, c := range result.Comments {
		if c.ID == "disqus:4005" {
			anon = &struct{ userID string }{c.UserID}
		}
	}
	if anon == nil {
		t.Fatal("missing anonymous comment disqus:4005")
	}
	if anon.userID != "disqus-user:anonymous" {
		t.Errorf("anonymous userID: got %q, want %q", anon.userID, "disqus-user:anonymous")
	}
}
