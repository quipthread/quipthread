package middleware

import (
	"regexp"
	"strings"

	"github.com/quipthread/quipthread/config"
)

var (
	tagRe  = regexp.MustCompile(`<[^>]*>`)
	hrefRe = regexp.MustCompile(`(?i)href\s*=|https?://`)

	dangerousPatterns = []string{
		"<script", "<iframe", "<object", "<embed", "<form",
		"javascript:", "data:text/html",
	}
)

// SpamChecker applies heuristic rules to comment content.
// It has no external dependencies and runs synchronously in the request path.
type SpamChecker struct {
	maxLinks int
}

// NewSpamChecker constructs a SpamChecker from the application config.
func NewSpamChecker(cfg *config.Config) *SpamChecker {
	return &SpamChecker{maxLinks: cfg.SpamMaxLinks}
}

// IsSpam returns true and a reason string if the content trips a spam rule.
func (s *SpamChecker) IsSpam(content string) (bool, string) {
	lower := strings.ToLower(content)

	// Block dangerous HTML that Tiptap should never produce but could be injected.
	for _, p := range dangerousPatterns {
		if strings.Contains(lower, p) {
			return true, "dangerous content"
		}
	}

	plain := strings.TrimSpace(tagRe.ReplaceAllString(content, ""))

	// Minimum meaningful length.
	if len(plain) < 2 {
		return true, "content too short"
	}

	// Maximum length guard (10 KB of plain text is generous).
	if len(content) > 10_000 {
		return true, "content too long"
	}

	// Link density check.
	if s.maxLinks > 0 {
		linkCount := len(hrefRe.FindAllString(content, -1))
		if linkCount > s.maxLinks {
			return true, "too many links"
		}
	}

	// Repeated-character run (≥10 identical bytes in a row, e.g. "aaaaaaaaaa").
	if hasRepeatedRun(plain, 10) {
		return true, "repeated characters"
	}

	return false, ""
}

// hasRepeatedRun returns true if s contains a run of n or more identical bytes.
func hasRepeatedRun(s string, n int) bool {
	run := 1
	for i := 1; i < len(s); i++ {
		if s[i] == s[i-1] {
			run++
			if run >= n {
				return true
			}
		} else {
			run = 1
		}
	}
	return false
}
