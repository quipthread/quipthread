package middleware

import (
	"strings"
	"sync"
	"time"

	"github.com/quipthread/quipthread/db"
)

type termsCache struct {
	mu      sync.Mutex
	terms   []string
	expires time.Time
}

func (c *termsCache) get(store db.Store) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.terms != nil && time.Now().Before(c.expires) {
		return c.terms, nil
	}
	records, err := store.ListBlockedTerms()
	if err != nil {
		return nil, err
	}
	terms := make([]string, len(records))
	for i, r := range records {
		terms[i] = strings.ToLower(r.Term)
	}
	c.terms = terms
	c.expires = time.Now().Add(60 * time.Second)
	return terms, nil
}

func (c *termsCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.terms = nil
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

// ContainsBlockedTerm returns true if the content contains any blocked term
// (case-insensitive whole-word or substring match). On cache refresh errors the
// check fails open — comments are allowed through rather than blocking all submissions
// due to a transient DB failure.
func (c *BlockedTermsChecker) ContainsBlockedTerm(content string) (bool, string) {
	terms, err := globalTermsCache.get(c.store)
	if err != nil || len(terms) == 0 {
		return false, ""
	}
	lower := strings.ToLower(content)
	for _, t := range terms {
		if strings.Contains(lower, t) {
			return true, t
		}
	}
	return false, ""
}
