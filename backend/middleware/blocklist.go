package middleware

import (
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/quipthread/quipthread/db"
)

type termEntry struct {
	plain string         // lowercase; used for substring match when rx is nil
	rx    *regexp.Regexp // non-nil when is_regex=true
}

type termsCache struct {
	mu      sync.Mutex
	entries []termEntry
	expires time.Time
}

func (c *termsCache) get(store db.Store) ([]termEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries != nil && time.Now().Before(c.expires) {
		return c.entries, nil
	}
	records, err := store.ListBlockedTerms()
	if err != nil {
		return nil, err
	}
	entries := make([]termEntry, 0, len(records))
	for _, r := range records {
		if r.IsRegex {
			rx, err := regexp.Compile(r.Term)
			if err != nil {
				// Skip malformed patterns — shouldn't happen since we validate on insert.
				continue
			}
			entries = append(entries, termEntry{rx: rx})
		} else {
			entries = append(entries, termEntry{plain: strings.ToLower(r.Term)})
		}
	}
	c.entries = entries
	c.expires = time.Now().Add(60 * time.Second)
	return entries, nil
}

func (c *termsCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = nil
}

var globalTermsCache = &termsCache{}

// InvalidateBlockedTermsCache resets the cached blocklist so the next comment
// check re-fetches from the DB. Call after any blocked_terms mutation.
func InvalidateBlockedTermsCache() {
	globalTermsCache.invalidate()
}

// BlockedTermsChecker checks comment content against the global blocklist.
type BlockedTermsChecker struct {
	store db.Store
}

func NewBlockedTermsChecker(store db.Store) *BlockedTermsChecker {
	return &BlockedTermsChecker{store: store}
}

// ContainsBlockedTerm returns true if the content contains any blocked term.
// Plain terms are matched case-insensitively as substrings; regex terms are
// matched against the original content using the compiled pattern.
// On cache refresh errors the check fails open — comments are allowed through
// rather than blocking all submissions due to a transient DB failure.
func (c *BlockedTermsChecker) ContainsBlockedTerm(content string) (bool, string) {
	entries, err := globalTermsCache.get(c.store)
	if err != nil || len(entries) == 0 {
		return false, ""
	}
	lower := strings.ToLower(content)
	for _, e := range entries {
		if e.rx != nil {
			if e.rx.MatchString(content) {
				return true, e.rx.String()
			}
		} else if strings.Contains(lower, e.plain) {
			return true, e.plain
		}
	}
	return false, ""
}
