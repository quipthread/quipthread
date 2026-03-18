package middleware

import (
	"strings"
	"testing"

	"github.com/quipthread/quipthread/config"
)

func newSpamChecker(maxLinks int) *SpamChecker {
	return NewSpamChecker(&config.Config{SpamMaxLinks: maxLinks})
}

func TestSpamChecker(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		maxLinks int
		wantSpam bool
		wantWhy  string
	}{
		// Dangerous HTML
		{
			name:     "script tag",
			content:  `<script>alert(1)</script>`,
			maxLinks: 3,
			wantSpam: true,
			wantWhy:  "dangerous content",
		},
		{
			name:     "javascript: href",
			content:  `<a href="javascript:void(0)">click</a>`,
			maxLinks: 3,
			wantSpam: true,
			wantWhy:  "dangerous content",
		},
		{
			name:     "data:text/html URI",
			content:  `<img src="data:text/html,<h1>x</h1>">`,
			maxLinks: 3,
			wantSpam: true,
			wantWhy:  "dangerous content",
		},
		// Content length
		{
			name:     "empty content",
			content:  "",
			maxLinks: 3,
			wantSpam: true,
			wantWhy:  "content too short",
		},
		{
			name:     "single char",
			content:  "x",
			maxLinks: 3,
			wantSpam: true,
			wantWhy:  "content too short",
		},
		{
			name:     "two chars is ok",
			content:  "hi",
			maxLinks: 3,
			wantSpam: false,
		},
		{
			name:     "exactly 10000 bytes",
			content:  strings.Repeat("ab", 5_000),
			maxLinks: 3,
			wantSpam: false,
		},
		{
			name:     "10001 bytes",
			content:  strings.Repeat("a", 10_001),
			maxLinks: 3,
			wantSpam: true,
			wantWhy:  "content too long",
		},
		// Link density
		{
			name:     "exactly maxLinks",
			content:  "https://a.com https://b.com https://c.com",
			maxLinks: 3,
			wantSpam: false,
		},
		{
			name:     "one over maxLinks",
			content:  "https://a.com https://b.com https://c.com https://d.com",
			maxLinks: 3,
			wantSpam: true,
			wantWhy:  "too many links",
		},
		{
			name:     "maxLinks=0 disables link check",
			content:  "https://a.com https://b.com https://c.com https://d.com https://e.com",
			maxLinks: 0,
			wantSpam: false,
		},
		// Repeated characters
		{
			name:     "nine identical chars — ok",
			content:  "aaaaaaaaa",
			maxLinks: 3,
			wantSpam: false,
		},
		{
			name:     "ten identical chars — spam",
			content:  "aaaaaaaaaa",
			maxLinks: 3,
			wantSpam: true,
			wantWhy:  "repeated characters",
		},
		// Clean comment
		{
			name:     "clean comment",
			content:  "<p>This is a perfectly fine comment.</p>",
			maxLinks: 3,
			wantSpam: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checker := newSpamChecker(tc.maxLinks)
			isSpam, why := checker.IsSpam(tc.content)
			if isSpam != tc.wantSpam {
				t.Errorf("IsSpam = %v, want %v (reason: %q)", isSpam, tc.wantSpam, why)
			}
			if tc.wantSpam && tc.wantWhy != "" && why != tc.wantWhy {
				t.Errorf("reason = %q, want %q", why, tc.wantWhy)
			}
		})
	}
}
