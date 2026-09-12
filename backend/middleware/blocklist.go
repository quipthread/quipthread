package middleware

import (
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/quipthread/quipthread/db"
)

// blockedTermsTTL bounds how long a scope's compiled blocklist may be served
// from cache before the next check re-fetches from the DB.
const blockedTermsTTL = 60 * time.Second

type termEntry struct {
	plain string         // lowercase; used for substring match when rx is nil
	rx    *regexp.Regexp // non-nil when is_regex=true
}

// termsScope holds the compiled entries for exactly one tenant scope.
type termsScope struct {
	entries []termEntry
	expires time.Time
}

// termsCache caches compiled blocklist terms keyed by tenant scope. The scope
// is a stable tenant identifier — the owning account ID in cloud mode, "" for
// the shared self-hosted store. Entries never cross scopes: every lookup and
// every invalidation is scoped, so one tenant's terms can never filter (or be
// flushed by) another tenant's traffic.
//
// Blocked terms are stored per tenant database (ListBlockedTerms takes no
// site argument), so account-level scoping is sufficient; there is no
// site-specific schema to key on.
type termsCache struct {
	mu     sync.Mutex
	scopes map[string]*termsScope
}

func (c *termsCache) get(scope string, store db.Store) ([]termEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.scopes == nil {
		c.scopes = make(map[string]*termsScope)
	}
	if s, ok := c.scopes[scope]; ok && time.Now().Before(s.expires) {
		return s.entries, nil
	}
	records, err := store.ListBlockedTerms()
	if err != nil {
		// Refresh failure: the stale scope (if any) is left untouched but is
		// past expiry, so the caller fails open until a later refresh
		// succeeds. Nothing from another scope is consulted.
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
	c.scopes[scope] = &termsScope{entries: entries, expires: time.Now().Add(blockedTermsTTL)}
	return entries, nil
}

func (c *termsCache) invalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.scopes = make(map[string]*termsScope)
}

func (c *termsCache) invalidate(scope string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.scopes, scope)
}

var globalTermsCache = &termsCache{}

// InvalidateBlockedTermsCache flushes every cached scope so the next comment
// check re-fetches from the DB. Call after any blocked_terms mutation. This is
// deliberately broad: mutation handlers may lack tenant context, and flushing
// other tenants' caches is merely a transient refetch, whereas failing to
// flush the mutated tenant would serve stale terms.
func InvalidateBlockedTermsCache() {
	globalTermsCache.invalidateAll()
}

// InvalidateBlockedTermsCacheFor flushes only the named tenant scope. Use it
// when the mutated tenant store is known (e.g. request-scoped admin paths) so
// unrelated tenants keep their warm caches.
func InvalidateBlockedTermsCacheFor(scope string) {
	globalTermsCache.invalidate(scope)
}

// BlockedTermsChecker checks comment content against a blocklist. The store
// held by the checker is only the legacy/self-hosted target used by
// ContainsBlockedTerm; request-scoped paths must call Check with the explicit
// tenant store and scope instead.
type BlockedTermsChecker struct {
	store db.Store
}

func NewBlockedTermsChecker(store db.Store) *BlockedTermsChecker {
	return &BlockedTermsChecker{store: store}
}

// ContainsBlockedTerm checks content against the constructor store's blocklist
// under the shared self-hosted scope. Cloud tenants must never use this path;
// use Check with the resolved tenant store and account scope.
func (c *BlockedTermsChecker) ContainsBlockedTerm(content string) (bool, string) {
	return c.Check("", c.store, content)
}

// Check returns true if the content contains any blocked term from store,
// caching the compiled list under scope. Plain terms are matched
// case-insensitively as substrings; regex terms are matched against the
// original content using the compiled pattern.
//
// On cache refresh errors the check fails open — comments are allowed through
// rather than blocking all submissions due to a transient DB failure — and
// only for the tenant whose refresh failed.
func (c *BlockedTermsChecker) Check(scope string, store db.Store, content string) (bool, string) {
	entries, err := globalTermsCache.get(scope, store)
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
