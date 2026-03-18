package importer

import (
	"os"
	"strings"
	"testing"
)

func TestParseWordPress_Basic(t *testing.T) {
	f, err := os.Open("../testdata/wordpress-basic.wxr")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	result, err := ParseWordPress(f)
	if err != nil {
		t.Fatalf("ParseWordPress: %v", err)
	}

	if len(result.Comments) != 3 {
		t.Fatalf("expected 3 comments, got %d", len(result.Comments))
	}
	// alice (email-keyed) and bob — 2 unique users
	if len(result.Users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(result.Users))
	}

	byID := make(map[string]struct {
		status, parentID, pageID string
	})
	for _, c := range result.Comments {
		byID[c.ID] = struct {
			status, parentID, pageID string
		}{c.Status, c.ParentID, c.PageID}
	}

	c1 := byID["wordpress:1"]
	if c1.status != "approved" {
		t.Errorf("comment 1 status: got %q, want approved", c1.status)
	}
	if c1.pageID != "/hello-world" {
		t.Errorf("comment 1 pageID: got %q, want /hello-world", c1.pageID)
	}
	if c1.parentID != "" {
		t.Errorf("comment 1 parentID: expected empty (comment_parent=0), got %q", c1.parentID)
	}

	c2 := byID["wordpress:2"]
	if c2.status != "pending" {
		t.Errorf("comment 2 status: got %q, want pending", c2.status)
	}

	c3 := byID["wordpress:3"]
	if c3.parentID != "wordpress:1" {
		t.Errorf("comment 3 parentID: got %q, want wordpress:1", c3.parentID)
	}
}

func TestParseWordPress_Edge(t *testing.T) {
	f, err := os.Open("../testdata/wordpress-edge.wxr")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	result, err := ParseWordPress(f)
	if err != nil {
		t.Fatalf("ParseWordPress: %v", err)
	}

	// pingback (10), trackback (11), spam (12), trash (13) all excluded
	// remaining: 14, 15, 16
	if len(result.Comments) != 3 {
		t.Fatalf("expected 3 comments (14, 15, 16), got %d", len(result.Comments))
	}

	excluded := map[string]string{
		"wordpress:10": "pingback",
		"wordpress:11": "trackback",
		"wordpress:12": "spam",
		"wordpress:13": "trash",
	}
	byID := make(map[string]struct{ content, disqusAuthor string })
	for _, c := range result.Comments {
		if reason, bad := excluded[c.ID]; bad {
			t.Errorf("comment %s (%s) should be excluded", c.ID, reason)
		}
		byID[c.ID] = struct{ content, disqusAuthor string }{c.Content, c.DisqusAuthor}
	}

	// plain text → wrapped in <p>
	c14 := byID["wordpress:14"]
	if !strings.HasPrefix(c14.content, "<p>") {
		t.Errorf("plain text comment 14 should be wrapped in <p>, got %q", c14.content)
	}

	// HTML content → passed through unchanged
	c15 := byID["wordpress:15"]
	if !strings.HasPrefix(c15.content, "<p>Already") {
		t.Errorf("HTML comment 15 should be unchanged, got %q", c15.content)
	}

	// HTML entity in author name decoded by XML parser
	c16 := byID["wordpress:16"]
	if c16.disqusAuthor != "J&J Smith" {
		t.Errorf("comment 16 author: got %q, want %q", c16.disqusAuthor, "J&J Smith")
	}
}

func TestMaybeWrapHTML(t *testing.T) {
	cases := []struct {
		input, want string
	}{
		{"", ""},
		{"<p>Already HTML.</p>", "<p>Already HTML.</p>"},
		{"Plain text.", "<p>Plain text.</p>"},
		{"First para\n\nSecond para", "<p>First para</p><p>Second para</p>"},
		{"Has a <br> tag", "Has a <br> tag"},
	}
	for _, tc := range cases {
		got := maybeWrapHTML(tc.input)
		if got != tc.want {
			t.Errorf("maybeWrapHTML(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
