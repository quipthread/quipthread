package importer

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseNative_Basic(t *testing.T) {
	f, err := os.Open("../testdata/native-basic.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	result, err := ParseNative(f)
	if err != nil {
		t.Fatalf("ParseNative: %v", err)
	}

	if len(result.Comments) != 3 {
		t.Fatalf("expected 3 comments, got %d", len(result.Comments))
	}
	// alice and bob — 2 unique users
	if len(result.Users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(result.Users))
	}

	byID := make(map[string]struct {
		pageID, pageURL, pageTitle, parentID, content, status string
		createdAt                                             time.Time
	})
	for _, c := range result.Comments {
		byID[c.ID] = struct {
			pageID, pageURL, pageTitle, parentID, content, status string
			createdAt                                             time.Time
		}{c.PageID, c.PageURL, c.PageTitle, c.ParentID, c.Content, c.Status, c.CreatedAt}
	}

	c1, ok := byID["c1"]
	if !ok {
		t.Fatal("missing comment c1")
	}
	if c1.pageID != "/hello-world" {
		t.Errorf("c1 pageID: got %q, want /hello-world", c1.pageID)
	}
	if c1.pageTitle != "Hello World" {
		t.Errorf("c1 pageTitle: got %q", c1.pageTitle)
	}
	if c1.content != "<p>Hello from Alice.</p>" {
		t.Errorf("c1 content: got %q", c1.content)
	}
	if c1.status != "approved" {
		t.Errorf("c1 status: got %q", c1.status)
	}
	want := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	if !c1.createdAt.Equal(want) {
		t.Errorf("c1 createdAt: got %v, want %v", c1.createdAt, want)
	}

	c2, ok := byID["c2"]
	if !ok {
		t.Fatal("missing comment c2")
	}
	if c2.parentID != "c1" {
		t.Errorf("c2 parentID: got %q, want c1", c2.parentID)
	}

	c3, ok := byID["c3"]
	if !ok {
		t.Fatal("missing comment c3")
	}
	if c3.status != "pending" {
		t.Errorf("c3 status: got %q, want pending", c3.status)
	}
}

func TestParseNative_WrongVersion(t *testing.T) {
	input := strings.NewReader(`{"version":2,"comments":[]}`)
	_, err := ParseNative(input)
	if err == nil {
		t.Fatal("expected error for version 2, got nil")
	}
}

func TestParseNative_Idempotent(t *testing.T) {
	f, err := os.Open("../testdata/native-basic.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	r1, err := ParseNative(f)
	if err != nil {
		t.Fatal(err)
	}

	f2, err := os.Open("../testdata/native-basic.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()
	r2, err := ParseNative(f2)
	if err != nil {
		t.Fatal(err)
	}

	if len(r1.Comments) != len(r2.Comments) {
		t.Fatalf("idempotent: comment count mismatch %d vs %d", len(r1.Comments), len(r2.Comments))
	}
	for i, c := range r1.Comments {
		if c.ID != r2.Comments[i].ID {
			t.Errorf("idempotent: comment[%d] ID mismatch %q vs %q", i, c.ID, r2.Comments[i].ID)
		}
	}
}
