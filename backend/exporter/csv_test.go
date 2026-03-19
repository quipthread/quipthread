package exporter

import (
	"bytes"
	"encoding/csv"
	"io"
	"testing"
	"time"

	"github.com/quipthread/quipthread/models"
)

func TestWriteCSV_Empty(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteCSV(&buf, nil); err != nil {
		t.Fatalf("WriteCSV(nil): %v", err)
	}

	r := csv.NewReader(&buf)
	header, err := r.Read()
	if err != nil {
		t.Fatalf("read header: %v", err)
	}
	if len(header) != 11 {
		t.Errorf("header column count: got %d, want 11", len(header))
	}
	if header[0] != "id" {
		t.Errorf("header[0]: got %q, want id", header[0])
	}
	if _, err := r.Read(); err != io.EOF {
		t.Errorf("expected EOF after header for empty input, got %v", err)
	}
}

func TestWriteCSV_HTMLStripped(t *testing.T) {
	ts := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	comments := []*models.Comment{
		{
			ID:         "c1",
			SiteID:     "s1",
			PageID:     "/page",
			AuthorName: "Alice",
			Status:     "approved",
			Content:    "<p>Hello <strong>world</strong>.</p>",
			CreatedAt:  ts,
			UpdatedAt:  ts,
		},
	}

	var buf bytes.Buffer
	if err := WriteCSV(&buf, comments); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}

	r := csv.NewReader(&buf)
	r.Read() //nolint:errcheck,gosec // skip header row
	row, err := r.Read()
	if err != nil {
		t.Fatalf("read row: %v", err)
	}

	want := "Hello world."
	if row[8] != want {
		t.Errorf("WriteCSV content: got %q, want %q", row[8], want)
	}
}

func TestWriteCSV_TimestampFormat(t *testing.T) {
	ts := time.Date(2024, 3, 17, 14, 30, 0, 0, time.UTC)
	comments := []*models.Comment{
		{ID: "c1", SiteID: "s1", Status: "approved", Content: "x", CreatedAt: ts, UpdatedAt: ts},
	}

	var buf bytes.Buffer
	if err := WriteCSV(&buf, comments); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}

	r := csv.NewReader(&buf)
	r.Read() //nolint:errcheck,gosec // skip header row
	row, _ := r.Read()

	want := "2024-03-17T14:30:00Z"
	if row[9] != want {
		t.Errorf("WriteCSV created_at: got %q, want %q", row[9], want)
	}
	if row[10] != want {
		t.Errorf("WriteCSV updated_at: got %q, want %q", row[10], want)
	}
}

func TestWriteCSV_SpecialCharsQuoted(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	comments := []*models.Comment{
		{
			ID:         "c1",
			SiteID:     "s1",
			AuthorName: "Alice, Jr.",
			Content:    `He said "hello"`,
			Status:     "approved",
			CreatedAt:  ts,
			UpdatedAt:  ts,
		},
	}

	var buf bytes.Buffer
	if err := WriteCSV(&buf, comments); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}

	r := csv.NewReader(&buf)
	r.Read() //nolint:errcheck,gosec // skip header row
	row, err := r.Read()
	if err != nil {
		t.Fatalf("read row: %v", err)
	}

	if row[6] != "Alice, Jr." {
		t.Errorf("WriteCSV author_name: got %q, want %q", row[6], "Alice, Jr.")
	}
	if row[8] != `He said "hello"` {
		t.Errorf("WriteCSV content: got %q, want %q", row[8], `He said "hello"`)
	}
}
