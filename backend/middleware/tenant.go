package middleware

import (
	"bytes"
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"golang.org/x/sync/singleflight"

	"github.com/quipthread/quipthread/cloud"
	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/session"
)

// Safe defaults for the bounded tenant store cache. Both are overridable via
// the validated TENANT_STORE_CACHE_CAPACITY and TENANT_STORE_CACHE_TTL config
// values (see config.Config).
const (
	DefaultStoreCacheCapacity = 128
	DefaultStoreCacheTTL      = 15 * time.Minute
)

// ErrStoreCacheClosed is returned by GetOrOpen once the cache has been closed
// during orderly shutdown. Callers must treat it as terminal: the cache will
// never hand out stores again.
var ErrStoreCacheClosed = errors.New("tenant store cache closed")

// StorageFingerprint identifies a tenant's storage location for cache
// validation without retaining the URL or credentials in the cache key.
func StorageFingerprint(dbType, dbURL string) string {
	digest := sha256.Sum256([]byte(dbType + "\x00" + dbURL))
	return hex.EncodeToString(digest[:])
}

// StoreLease is a request-scoped handle to a tenant store acquired from the
// cache. The cache guarantees the store stays open until Release: retirement
// (capacity eviction, TTL expiry, fingerprint replacement, Evict, Close)
// removes the entry from lookup immediately but defers the close until the
// final active lease releases. Release is idempotent.
type StoreLease struct {
	store   db.Store
	release func()
	once    sync.Once
}

// Store returns the leased tenant store.
func (l *StoreLease) Store() db.Store {
	if l == nil {
		return nil
	}
	return l.store
}

// Release gives up the caller's claim on the store. Idempotent and safe to
// call on a nil lease. After the final release of a retired entry, the cache
// closes its store exactly once, outside cache locks.
func (l *StoreLease) Release() {
	if l == nil || l.release == nil {
		return
	}
	l.once.Do(l.release)
}

// UnmanagedLease wraps a store not owned by the cache (e.g. a global fallback
// store). Its Release is a no-op.
func UnmanagedLease(s db.Store) *StoreLease {
	return &StoreLease{store: s}
}

// storeCacheEntry is a cache-owned tenant store with its validation metadata
// and borrower accounting. All fields are guarded by StoreCache.mu.
type storeCacheEntry struct {
	accountID   string
	store       db.Store
	fingerprint string
	lastUsed    time.Time
	refs        int // active leases borrowed from this entry
	retired     bool
	closed      bool
}

// StoreCache holds open tenant store connections keyed by account ID, bounded
// by capacity (deterministic LRU) and idle TTL (lazy expiration). Opens are
// deduplicated per account+fingerprint: a cold burst invokes the opener exactly
// once, and concurrent requests for different storage targets of the same
// account never receive each other's stores.
//
// The cache owns every store it retains. Retired entries leave the lookup
// immediately but their stores close only after the last active lease
// releases — an actively borrowed store is never closed. Every store is closed
// exactly once, outside cache locks. Once closed, the cache fails
// deterministically and closes any opener result it cannot retain.
type StoreCache struct {
	mu       sync.Mutex
	entries  map[string]*list.Element // accountID -> element holding *storeCacheEntry
	lru      *list.List               // front = most recently used
	capacity int
	ttl      time.Duration
	closed   bool
	group    singleflight.Group
	nowFn    func() time.Time // overridable for deterministic TTL tests
}

func NewStoreCache() *StoreCache {
	return NewStoreCacheWithLimits(DefaultStoreCacheCapacity, DefaultStoreCacheTTL)
}

// NewStoreCacheWithLimits overrides the safe defaults. Non-positive values
// fall back to the defaults, mirroring config validation.
func NewStoreCacheWithLimits(capacity int, ttl time.Duration) *StoreCache {
	if capacity < 1 {
		capacity = DefaultStoreCacheCapacity
	}
	if ttl <= 0 {
		ttl = DefaultStoreCacheTTL
	}
	return &StoreCache{
		entries:  make(map[string]*list.Element),
		lru:      list.New(),
		capacity: capacity,
		ttl:      ttl,
		nowFn:    time.Now,
	}
}

// GetOrOpen returns a lease on the cached tenant store for accountID when a
// live entry exists (not idle-expired, fingerprint still valid); otherwise it
// opens one via opener, deduplicating concurrent opens per account+fingerprint
// so a cold burst invokes the opener exactly once. Failed opens are never
// cached and are retryable. When the storage fingerprint differs from the
// cached entry, the old entry is retired (closed after its final release) and
// a fresh store is opened for the requested fingerprint.
//
// Callers must call Lease.Release when finished — middleware releases only
// after downstream handlers complete.
func (c *StoreCache) GetOrOpen(accountID, fingerprint string, opener func() (db.Store, error)) (*StoreLease, error) {
	return c.GetOrOpenContext(context.Background(), accountID, fingerprint, func(context.Context) (db.Store, error) {
		return opener()
	})
}

// GetOrOpenContext is the cancellation-aware tenant-store cache path. The
// opener receives the same context so callers and the cache can stop waiting
// at the tenant deadline or shutdown budget. Already-started native connects
// and SQL operations are non-interruptible, so direct callers may remain
// blocked until the opener returns. A store opened after cancellation is
// closed instead of being retained by the cache.
func (c *StoreCache) GetOrOpenContext(ctx context.Context, accountID, fingerprint string, opener func(context.Context) (db.Store, error)) (*StoreLease, error) {
	if ctx == nil {
		return nil, context.Canceled
	}
	if opener == nil {
		return nil, errors.New("tenant store opener is required")
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if lease, err := c.acquire(accountID, fingerprint); err != nil {
			return nil, err
		} else if lease != nil {
			return lease, nil
		}

		orphan, err := c.openOnceContext(ctx, accountID, fingerprint, opener)
		if err != nil {
			return nil, err
		}
		if orphan != nil {
			// The cache slot was taken by a concurrent insert before ours
			// landed. The opened store still matches the requested
			// fingerprint exactly, so hand it out as an uncached lease rather
			// than discarding work or returning a wrong-fingerprint store.
			return orphan, nil
		}
		// Population raced with us; loop and acquire from the cache.
	}
}

// acquire takes a referenced lease on the live entry for accountID, if one
// exists and is fresh with a matching fingerprint. Expired or mismatched
// entries are retired (closed when nothing borrows them). A nil lease with no
// error means cache miss.
func (c *StoreCache) acquire(accountID, fingerprint string) (*StoreLease, error) {
	var toClose []db.Store

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrStoreCacheClosed
	}
	el, ok := c.entries[accountID]
	if ok {
		entry := el.Value.(*storeCacheEntry)
		now := c.nowFn()
		if entry.fingerprint == fingerprint && now.Sub(entry.lastUsed) < c.ttl {
			entry.lastUsed = now
			entry.refs++
			c.lru.MoveToFront(el)
			c.mu.Unlock()
			return &StoreLease{store: entry.store, release: func() { c.releaseEntry(entry) }}, nil
		}

		// Expired or the account's storage moved — retire and reopen.
		c.lru.Remove(el)
		delete(c.entries, accountID)
		toClose = c.retireLocked(entry)
	}
	c.mu.Unlock()

	closeStores(toClose)
	return nil, nil
}

// peekLive reports whether a fresh, fingerprint-matching entry currently
// occupies the slot, without borrowing it. Used inside singleflight to detect
// concurrent population.
func (c *StoreCache) peekLive(accountID, fingerprint string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.entries[accountID]
	if !ok {
		return false
	}
	entry := el.Value.(*storeCacheEntry)
	return !c.closed && entry.fingerprint == fingerprint && c.nowFn().Sub(entry.lastUsed) < c.ttl
}

// openOnceContext deduplicates the open across callers requesting the same
// account+fingerprint. It returns an uncached orphan lease only when the cache
// slot was lost to a concurrent insert; otherwise the opened store is cached
// and callers acquire it from the cache.
func (c *StoreCache) openOnceContext(ctx context.Context, accountID, fingerprint string, opener func(context.Context) (db.Store, error)) (*StoreLease, error) {
	var orphan db.Store

	result := c.group.DoChan(accountID+"\x00"+fingerprint, func() (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Re-check after waiting on an in-flight open for the same target:
		// the winner may have populated a matching entry meanwhile.
		if c.peekLive(accountID, fingerprint) {
			return nil, nil
		}

		opened, err := opener(ctx)
		if err != nil {
			return nil, err // failed opens are never cached
		}
		if opened == nil {
			return nil, errors.New("tenant store opener returned nil store")
		}
		if err := ctx.Err(); err != nil {
			_ = opened.Close()
			return nil, err
		}
		retained, err := c.insert(accountID, fingerprint, opened)
		if err != nil {
			_ = opened.Close() //nolint:errcheck // best-effort release on shutdown race
			return nil, err
		}
		if !retained {
			orphan = opened // slot lost to a concurrent insert; caller owns it
		}
		return nil, nil
	})
	var err error
	select {
	case completed := <-result:
		err = completed.Err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	if orphan != nil {
		return &StoreLease{store: orphan, release: func() { _ = orphan.Close() }}, nil //nolint:errcheck // best-effort release
	}
	return nil, nil
}

// insert stores an opened entry in the cache slot and enforces capacity via
// deterministic LRU. It reports whether the store was retained; when another
// insert already claimed the slot, the caller keeps ownership of its store.
// Displaced entries are retired — closed only when nothing borrows them — and
// any immediate closes happen outside the cache lock.
func (c *StoreCache) insert(accountID, fingerprint string, s db.Store) (bool, error) {
	var toClose []db.Store

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return false, ErrStoreCacheClosed
	}
	if _, ok := c.entries[accountID]; ok {
		// Lost the race to a concurrent insert for this account. Keep the
		// existing entry; the caller's store remains valid as an orphan.
		c.mu.Unlock()
		return false, nil
	}
	c.entries[accountID] = c.lru.PushFront(&storeCacheEntry{
		accountID:   accountID,
		store:       s,
		fingerprint: fingerprint,
		lastUsed:    c.nowFn(),
	})
	for c.lru.Len() > c.capacity {
		oldest := c.lru.Back()
		evicted := oldest.Value.(*storeCacheEntry)
		c.lru.Remove(oldest)
		delete(c.entries, evicted.accountID)
		toClose = append(toClose, c.retireLocked(evicted)...)
	}
	c.mu.Unlock()

	closeStores(toClose)
	return true, nil
}

// retireLocked marks an entry retired. It returns the store to close when
// nothing borrows it (closing itself must happen outside cache locks);
// otherwise the final release closes it.
func (c *StoreCache) retireLocked(entry *storeCacheEntry) []db.Store {
	entry.retired = true
	if entry.refs == 0 && !entry.closed {
		entry.closed = true
		return []db.Store{entry.store}
	}
	return nil
}

func (c *StoreCache) releaseEntry(entry *storeCacheEntry) {
	var toClose []db.Store

	c.mu.Lock()
	if entry.refs > 0 {
		entry.refs--
	}
	if entry.retired && entry.refs == 0 && !entry.closed {
		entry.closed = true
		toClose = append(toClose, entry.store)
	}
	c.mu.Unlock()

	closeStores(toClose)
}

func closeStores(stores []db.Store) {
	for _, s := range stores {
		_ = s.Close() //nolint:errcheck // best-effort release; nothing actionable
	}
}

// Evict retires the cached entry for accountID, if any, reporting whether one
// existed. The entry leaves lookup immediately; its store closes right away
// only if nothing is currently borrowing it, otherwise after the final
// release. Safe to call on a closed cache (no-op).
func (c *StoreCache) Evict(accountID string) bool {
	var toClose []db.Store

	c.mu.Lock()
	el, ok := c.entries[accountID]
	if ok {
		entry := el.Value.(*storeCacheEntry)
		delete(c.entries, accountID)
		c.lru.Remove(el)
		toClose = c.retireLocked(entry)
	}
	c.mu.Unlock()

	closeStores(toClose)
	return ok
}

// Close retires every cached entry and puts the cache into a terminal state:
// subsequent GetOrOpen calls fail with ErrStoreCacheClosed, and any opener
// result that cannot be retained is closed. Stores without active leases close
// during Close; borrowed ones close after their final release. Idempotent.
func (c *StoreCache) Close() error {
	var toClose []db.Store

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	for el := c.lru.Front(); el != nil; el = el.Next() {
		entry := el.Value.(*storeCacheEntry)
		toClose = append(toClose, c.retireLocked(entry)...)
	}
	c.entries = make(map[string]*list.Element)
	c.lru.Init()
	c.mu.Unlock()

	var firstErr error
	for _, s := range toClose {
		if err := s.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// OpenTenantStore opens the tenant db.Store for the given account. Callers
// should go through StoreCache.GetOrOpen, which deduplicates and caches the
// result.
func OpenTenantStore(acc *cloud.Account, cfg *config.Config) (db.Store, error) {
	return OpenTenantStoreContext(context.Background(), acc, cfg)
}

// OpenTenantStoreContext opens a tenant store using the caller's context so
// callers can stop waiting before native work begins or while setup proceeds.
// Already-started native connects and SQL operations are non-interruptible, so
// direct callers may remain blocked until return; late-completing opens are
// closed by the cache/open path rather than retained.
func OpenTenantStoreContext(ctx context.Context, acc *cloud.Account, cfg *config.Config) (db.Store, error) {
	if ctx == nil {
		return nil, context.Canceled
	}
	if acc == nil || cfg == nil {
		return nil, errors.New("tenant store account and config are required")
	}
	if acc.DBType == "turso" {
		if acc.ProvisioningStatus != "" && acc.ProvisioningStatus != cloud.AccountReady {
			return nil, errors.New("tenant database is not ready")
		}
		if acc.DBURL == "" {
			return nil, errors.New("tenant database target is empty")
		}
		return db.NewLibSQLStoreWithAuthTokenContext(ctx, acc.DBURL, cfg.TursoAuthToken)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return db.NewSQLiteStore(acc.DBURL)
}

// openTenantStore is the resolver-facing open seam; overridable in tests so
// middleware paths can be exercised with close-counting fake stores.
var openTenantStore = OpenTenantStore

// isPublicEmbedRoute reports whether the request targets exactly one of the
// public embed endpoints whose tenant is resolved by PublicSiteResolver (or,
// when that feature is disabled, needs no tenant at all). Method and exact
// path only — prefixes and subresources are deliberately excluded so admin
// mutations on the same paths still get session-based tenant injection.
func isPublicEmbedRoute(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	return r.URL.Path == "/api/config" || r.URL.Path == "/api/comments"
}

// isPublicCommentPostRoute reports whether the request targets exactly
// POST /api/comments while the public site resolver feature is enabled. In
// that mode the tenant for this route comes from the body-aware resolver
// (site_id in the JSON body), so the global JWT-based injection must bypass
// it — a session cookie must never be able to select its tenant store.
func isPublicCommentPostRoute(cfg *config.Config, r *http.Request) bool {
	return cfg.PublicSiteResolverEnabled &&
		r.Method == http.MethodPost &&
		r.URL.Path == "/api/comments"
}

// isPublicCommentMutationRoute reports whether the request targets exactly one
// of the public comment mutation endpoints — DELETE /api/comments/{id},
// POST /api/comments/{id}/vote, POST /api/comments/{id}/flag — while the
// public site resolver feature is enabled. Their tenant comes from the
// required siteId query parameter via the route-local query resolver, so the
// global JWT-based injection must bypass them: a session cookie must never be
// able to select their tenant store. Matching is exact method + path segment
// shape; other methods, trailing slashes, unknown subresources, and nested
// paths stay subject to JWT injection.
func isPublicCommentMutationRoute(cfg *config.Config, r *http.Request) bool {
	if !cfg.PublicSiteResolverEnabled {
		return false
	}
	segs := strings.Split(r.URL.Path, "/")
	if len(segs) < 4 || segs[1] != "api" || segs[2] != "comments" || segs[3] == "" {
		return false
	}
	switch r.Method {
	case http.MethodDelete:
		return len(segs) == 4
	case http.MethodPost:
		return len(segs) == 5 && (segs[4] == "vote" || segs[4] == "flag")
	default:
		return false
	}
}

// InjectTenantStore resolves the tenant db.Store from the JWT AccountID claim and
// injects it into the request context. Only active when cfg.CloudMode is true.
func InjectTenantStore(cloudStore cloud.Store, cache *StoreCache, cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.CloudMode {
				next.ServeHTTP(w, r)
				return
			}

			// Public embed endpoints must never have their tenant determined
			// by cookies/JWT: they are resolved by the route-local public site
			// resolver from the site registry, or need no tenant store at all.
			// With the resolver feature enabled, POST /api/comments is equally
			// body-resolved and must bypass JWT tenant injection too, as must
			// the query-resolved comment mutation routes (delete/vote/flag).
			if isPublicEmbedRoute(r) || isPublicCommentPostRoute(cfg, r) || isPublicCommentMutationRoute(cfg, r) {
				next.ServeHTTP(w, r)
				return
			}

			// Static assets and public pages that don't need a tenant store —
			// skip resolution so a logged-in user's session cookie doesn't
			// block delivery, and so unauthenticated access never returns 500
			// from a failed tenant lookup. Approval links resolve their tenant
			// exclusively from the hashed central locator inside the approval
			// controller — a session cookie must never influence which store
			// they reach, so /approve is bypassed here entirely.
			if strings.HasPrefix(r.URL.Path, "/_astro/") ||
				strings.HasPrefix(r.URL.Path, "/auth/") ||
				strings.HasPrefix(r.URL.Path, "/approve/") ||
				r.URL.Path == "/favicon.svg" ||
				r.URL.Path == "/embed.js" ||
				r.URL.Path == "/embed-preview" ||
				r.URL.Path == "/login" ||
				r.URL.Path == "/signup" ||
				r.URL.Path == "/forgot-password" ||
				r.URL.Path == "/accept-invite" ||
				r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			// Claims may already be in context (set by RequireAuth/RequireAdmin),
			// or need to be parsed directly from the cookie — InjectTenantStore
			// runs as a global middleware before the auth group middleware.
			claims, _ := r.Context().Value(session.UserKey).(*session.Claims)
			if claims == nil {
				cookieName := session.DashboardCookieName
				if isEmbedAuthRoute(r) || isPublicEmbedRoute(r) || isPublicCommentPostRoute(cfg, r) || isPublicCommentMutationRoute(cfg, r) {
					cookieName = session.EmbedCookieName
				}
				if cookie, err := r.Cookie(cookieName); err == nil {
					audience := session.DashboardAudience
					if cookieName == session.EmbedCookieName {
						audience = session.EmbedAudience
					}
					claims, _ = session.ParseForAudience(cfg.JWTSecret, cookie.Value, audience)
				}
			}
			if claims == nil || claims.AccountID == "" {
				next.ServeHTTP(w, r)
				return
			}
			if claims.IsTeamMember {
				if err := cloud.CheckTeamMembership(cloudStore, claims.Sub, claims.AccountID); err != nil {
					writeError(w, http.StatusUnauthorized, "team membership is no longer active")
					return
				}
			}

			acc, err := cloudStore.GetAccountByID(claims.AccountID)
			if err != nil || acc == nil {
				// Do not fall through to the global store — that would expose
				// other tenants' data to an authenticated but unresolvable account.
				http.Error(w, `{"error":"tenant resolution failed"}`, http.StatusInternalServerError)
				return
			}

			lease, err := cache.GetOrOpen(claims.AccountID, StorageFingerprint(acc.DBType, acc.DBURL), func() (db.Store, error) {
				return openTenantStore(acc, cfg)
			})
			if err != nil {
				// Store open failure — same principle, never fall through to global.
				http.Error(w, `{"error":"tenant store unavailable"}`, http.StatusInternalServerError)
				return
			}
			// Hold the lease until downstream handling completes so retirement
			// can never close a store still serving a request.
			defer lease.Release()

			next.ServeHTTP(w, r.WithContext(db.WithStore(r.Context(), lease.Store())))
		})
	}
}

func isEmbedAuthRoute(r *http.Request) bool {
	return r.URL.Path == "/api/auth/embed/me" || r.URL.Path == "/auth/embed/logout"
}

// publicResolveFailure is a terminal, caller-safe resolution failure: an HTTP
// status plus the stable error code to emit. It never carries registry or
// topology details.
type publicResolveFailure struct {
	status int
	code   string
}

// resolvePublicTenant performs the shared tenant resolution used by both the
// GET query-param resolver and the POST body-aware resolver: active registry
// lookup → owning account → cached tenant store open → verified tenant site.
// On success it returns the explicit PublicTenant and the lease that must be
// held until downstream handling completes. Failures are stable JSON codes;
// there is no global-store fallback path.
func resolvePublicTenant(cloudStore cloud.Store, cache *StoreCache, cfg *config.Config, siteID string) (db.PublicTenant, *StoreLease, *publicResolveFailure) {
	entry, err := cloudStore.GetActiveSiteRegistration(siteID)
	if err != nil {
		return db.PublicTenant{}, nil, &publicResolveFailure{http.StatusServiceUnavailable, "service_unavailable"}
	}
	if entry == nil {
		// Unknown or not-active (reserved/inactive) registration —
		// indistinguishable to the caller on purpose.
		return db.PublicTenant{}, nil, &publicResolveFailure{http.StatusNotFound, "site_not_found"}
	}

	acc, err := cloudStore.GetAccountByID(entry.AccountID)
	if err != nil || acc == nil {
		return db.PublicTenant{}, nil, &publicResolveFailure{http.StatusServiceUnavailable, "service_unavailable"}
	}
	lease, err := cache.GetOrOpen(entry.AccountID, StorageFingerprint(acc.DBType, acc.DBURL), func() (db.Store, error) {
		return openTenantStore(acc, cfg)
	})
	if err != nil {
		return db.PublicTenant{}, nil, &publicResolveFailure{http.StatusServiceUnavailable, "service_unavailable"}
	}
	s := lease.Store()

	// Verify the site actually exists in the tenant data plane before
	// serving; a registry entry without tenant rows is a consistency
	// gap we must not paper over with global-store fallback.
	site, err := s.GetSite(siteID)
	if err != nil {
		lease.Release()
		return db.PublicTenant{}, nil, &publicResolveFailure{http.StatusServiceUnavailable, "service_unavailable"}
	}
	if site == nil {
		lease.Release()
		return db.PublicTenant{}, nil, &publicResolveFailure{http.StatusNotFound, "site_not_found"}
	}

	return db.PublicTenant{
		Store:     s,
		SiteID:    siteID,
		AccountID: entry.AccountID,
	}, lease, nil
}

// PublicSiteResolver resolves the tenant for public embed traffic (GET
// /api/config, GET /api/comments) from the site registry instead of session
// cookies. It is route-local middleware: install it only around those two
// exact GET routes, and only when CloudMode && PublicSiteResolverEnabled &&
// cloudStore != nil. The required siteId query parameter selects an active
// site registration; the owning account's tenant store is opened via the
// shared cache/open path and injected as an explicit public-tenant context.
// Every failure mode responds with stable JSON and never falls back to the
// master/global store.
func PublicSiteResolver(cloudStore cloud.Store, cache *StoreCache, cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.CloudMode || !cfg.PublicSiteResolverEnabled || cloudStore == nil {
				next.ServeHTTP(w, r)
				return
			}

			siteID := r.URL.Query().Get("siteId")
			if siteID == "" {
				writePublicError(w, r, http.StatusBadRequest, "site_id_required")
				return
			}

			pt, lease, failure := resolvePublicTenant(cloudStore, cache, cfg, siteID)
			if failure != nil {
				writePublicError(w, r, failure.status, failure.code)
				return
			}
			// Hold the lease until downstream handling completes so retirement
			// can never close a store still serving a request.
			defer lease.Release()

			ctx := db.WithPublicTenant(r.Context(), pt)
			ctx = db.WithStore(ctx, pt.Store)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// MaxPublicBodyBytes bounds the request body accepted by the body-aware POST
// resolver before anything is read. 1 MiB is far above any legitimate comment
// payload while capping buffering cost per in-flight request.
const MaxPublicBodyBytes = 1 << 20

// extractSiteID pulls only the top-level site_id field out of a JSON body.
// It decodes into a single-field probe (no re-encoding of the payload) and
// reports false when the body is not valid JSON at all.
func extractSiteID(raw []byte) (string, bool) {
	var probe struct {
		SiteID string `json:"site_id"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return "", false
	}
	return probe.SiteID, true
}

// PublicSiteResolverPOST is the body-aware counterpart of PublicSiteResolver
// for exactly POST /api/comments. It applies a hard ≤1 MiB body bound before
// reading, reads the raw bytes once, extracts only site_id without re-encoding
// the payload, restores byte-identical body content for downstream handlers,
// resolves the tenant through the same active-registry/account/cache/verified-
// site path as the GET resolver, and attaches an explicit PublicTenant context
// with the store lease held through downstream handling. Cookies and any
// pre-existing tenant context are ignored; every failure responds with stable
// JSON and there is no fallback to the global store. Install it only on the
// exact POST /api/comments route, only when CloudMode &&
// PublicSiteResolverEnabled && cloudStore != nil. When inactive it passes
// through untouched without consuming the body.
func PublicSiteResolverPOST(cloudStore cloud.Store, cache *StoreCache, cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.CloudMode || !cfg.PublicSiteResolverEnabled || cloudStore == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Bound before reading so oversized bodies fail fast with 413
			// instead of being buffered.
			r.Body = http.MaxBytesReader(w, r.Body, MaxPublicBodyBytes)
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				var maxErr *http.MaxBytesError
				if errors.As(err, &maxErr) {
					writePublicError(w, r, http.StatusRequestEntityTooLarge, "body_too_large")
					return
				}
				writePublicError(w, r, http.StatusBadRequest, "invalid_json")
				return
			}

			siteID, ok := extractSiteID(raw)
			if !ok {
				writePublicError(w, r, http.StatusBadRequest, "invalid_json")
				return
			}
			if siteID == "" {
				writePublicError(w, r, http.StatusBadRequest, "site_id_required")
				return
			}

			pt, lease, failure := resolvePublicTenant(cloudStore, cache, cfg, siteID)
			if failure != nil {
				writePublicError(w, r, failure.status, failure.code)
				return
			}
			// Hold the lease until downstream handling completes so retirement
			// can never close a store still serving a request.
			defer lease.Release()

			// Restore the identical bytes so the handler remains the
			// authoritative full decoder of the payload.
			r.Body = io.NopCloser(bytes.NewReader(raw))
			ctx := db.WithPublicTenant(r.Context(), pt)
			ctx = db.WithStore(ctx, pt.Store)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// writePublicError emits the stable JSON error shape used by handlers
// ({"error": code, "request_id": ...}) without leaking registry/topology
// details. 5xx codes are logged with the request ID for correlation.
func writePublicError(w http.ResponseWriter, r *http.Request, status int, code string) {
	reqID := chimiddleware.GetReqID(r.Context())
	if status >= 500 {
		slog.ErrorContext(r.Context(), "public tenant resolution failed",
			"request_id", reqID, "status", status, "code", code, "path", r.URL.Path)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": code, "request_id": reqID}) //nolint:errcheck,gosec // error response; connection may already be broken
}

// --- Public POST origin enforcement ------------------------------------------

// csrfOriginForbidden is the deterministic rejection code for the route-local
// CSRF/origin check on the cookie-authenticated public comment POST.
const csrfOriginForbidden = "csrf_origin_forbidden"

// normalizedDomain is a safely parsed stored site Domain. hasScheme records
// whether the stored value carried an explicit scheme; bare-host domains (the
// common form, e.g. "example.com") carry no scheme or port information in the
// existing model, which stores exactly what the admin entered after only
// non-empty validation.
type normalizedDomain struct {
	host      string // lowercase hostname, no port, no user info
	port      string // explicit port, "" when none was stored
	hasScheme bool
	scheme    string // set only when hasScheme
}

// normalizeSiteDomain parses a stored site Domain into its exact host/port
// identity. It deliberately rejects anything that could widen the match:
// wildcards, leading-dot prefixes, paths, query strings, fragments, user
// info, whitespace, and non-http(s) schemes. Bare hosts are validated by
// re-parsing them as URL authorities so malformed ports are caught.
func normalizeSiteDomain(domain string) (normalizedDomain, bool) {
	var nd normalizedDomain

	d := strings.TrimSpace(domain)
	if d == "" {
		return nd, false
	}

	if i := strings.Index(d, "://"); i >= 0 {
		u, err := url.Parse(d)
		if err != nil {
			return nd, false
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return nd, false
		}
		if u.User != nil || u.Host == "" {
			return nd, false
		}
		if u.Path != "" && u.Path != "/" {
			return nd, false
		}
		nd.hasScheme = true
		nd.scheme = u.Scheme
		nd.host = strings.ToLower(u.Hostname())
		nd.port = u.Port()
	} else {
		// Bare host[:port]. Reject every character that would change how a
		// URL authority is interpreted before delegating to the parser.
		if strings.ContainsAny(d, "/\\?#%@* \t\r\n") {
			return nd, false
		}
		u, err := url.Parse("http://" + d)
		if err != nil || u.Host == "" || u.Path != "" && u.Path != "/" {
			return nd, false
		}
		nd.host = strings.ToLower(u.Hostname())
		nd.port = u.Port()
	}

	if nd.host == "" || nd.host == "*" || strings.HasPrefix(nd.host, ".") {
		return nd, false
	}
	return nd, true
}

func defaultPort(scheme string) string {
	switch scheme {
	case "https":
		return "443"
	case "http":
		return "80"
	default:
		return ""
	}
}

// originMatchesDomain exact-matches a parsed Origin against the normalized
// stored domain: scheme (exact when the domain declares one; http/https only
// for bare hosts, since the model does not record their scheme), lowercase
// host, and effective port (explicit ports must match; absent ports compare
// against the scheme default).
func originMatchesDomain(origin *url.URL, d normalizedDomain) bool {
	if origin.User != nil || origin.Host == "" {
		return false
	}
	if d.hasScheme {
		if origin.Scheme != d.scheme {
			return false
		}
	} else if origin.Scheme != "http" && origin.Scheme != "https" {
		return false
	}
	if strings.ToLower(origin.Hostname()) != d.host {
		return false
	}

	op := origin.Port()
	if op == "" {
		op = defaultPort(origin.Scheme)
	}
	dp := d.port
	if dp == "" {
		if d.hasScheme {
			dp = defaultPort(d.scheme)
		} else {
			// Bare host with no stored port: the origin must be on its own
			// scheme's default port.
			dp = defaultPort(origin.Scheme)
		}
	}
	return op == dp
}

// parseOriginHeader validates the Origin header shape itself: present, not
// the string "null", an absolute http(s) URL with a host, and free of user
// info, query, fragment, and any path beyond "/" — matching what browsers
// actually emit.
func parseOriginHeader(raw string) (*url.URL, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "null") {
		return nil, false
	}
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() {
		return nil, false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, false
	}
	if u.User != nil || u.Host == "" {
		return nil, false
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return nil, false
	}
	if u.Path != "" && u.Path != "/" {
		return nil, false
	}
	return u, true
}

// isJSONContentType reports whether the Content-Type header designates
// application/json. Parameters such as charset are permitted; anything else,
// including a missing header, is not JSON.
func isJSONContentType(ct string) bool {
	mt, _, err := mime.ParseMediaType(ct)
	return err == nil && mt == "application/json"
}

// enforcePublicSiteOrigin is the shared site-bound CSRF/origin check used by
// both origin middlewares. The check is bound to the resolved verified site,
// never to cookies, JWTs, or global CORS configuration: the request must carry
// a non-empty, non-"null", well-formed Origin whose scheme, host, and port
// exactly match the resolved site's Domain. When requireJSON is set (the
// body-bearing POST route) Content-Type must also be application/json; the
// bodyless mutation routes skip that requirement. Any other combination is
// rejected deterministically with 403 csrf_origin_forbidden before downstream
// middleware or the handler run. When cloud mode or the resolver flag is off
// it passes through untouched, preserving self-hosted behavior.
func enforcePublicSiteOrigin(cfg *config.Config, requireJSON bool) func(http.Handler) http.Handler {
	active := cfg.CloudMode && cfg.PublicSiteResolverEnabled

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !active {
				next.ServeHTTP(w, r)
				return
			}

			pt, ok := db.PublicTenantFromContext(r.Context())
			if !ok || pt.Store == nil || pt.SiteID == "" {
				// The resolver chain was bypassed — fail closed rather than
				// authorize an unverifiable origin.
				writePublicError(w, r, http.StatusInternalServerError, "internal_error")
				return
			}
			site, err := pt.Store.GetSite(pt.SiteID)
			if err != nil || site == nil {
				writePublicError(w, r, http.StatusInternalServerError, "internal_error")
				return
			}

			forbidden := func() bool {
				if requireJSON && !isJSONContentType(r.Header.Get("Content-Type")) {
					return true
				}
				origin, ok := parseOriginHeader(r.Header.Get("Origin"))
				if !ok {
					return true
				}
				d, ok := normalizeSiteDomain(site.Domain)
				if !ok {
					// An unstorable/unverifiable domain can never match:
					// fail closed instead of widening the acceptance rule.
					return true
				}
				return !originMatchesDomain(origin, d)
			}()
			if forbidden {
				writePublicError(w, r, http.StatusForbidden, csrfOriginForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// EnforcePublicPostOrigin is route-local CSRF/origin validation for exactly
// POST /api/comments in enabled cloud public-resolver mode. Install it
// immediately after PublicSiteResolverPOST and before quota/handler. In
// addition to the exact-match Origin rule it requires Content-Type
// application/json.
func EnforcePublicPostOrigin(cfg *config.Config) func(http.Handler) http.Handler {
	return enforcePublicSiteOrigin(cfg, true)
}

// EnforcePublicMutationOrigin is the bodyless counterpart of
// EnforcePublicPostOrigin for exactly DELETE /api/comments/{id},
// POST /api/comments/{id}/vote, and POST /api/comments/{id}/flag in enabled
// cloud public-resolver mode. Install it immediately after the query-based
// PublicSiteResolver and before plan/handler. Because these requests carry no
// JSON body, only the site-bound exact-match Origin rule applies — no
// Content-Type requirement.
func EnforcePublicMutationOrigin(cfg *config.Config) func(http.Handler) http.Handler {
	return enforcePublicSiteOrigin(cfg, false)
}

// RequirePlanPublic is the request-scoped counterpart of RequirePlan for the
// query-resolved comment mutation routes. The subscription decision comes
// exclusively from the explicit PublicTenant context — never from JWT-selected
// storage or the constructor/global store — so plan checks can neither be
// driven by cookies nor poison other tenants' cached subscriptions. A missing
// context fails closed. When cloud mode or the resolver feature is off it
// passes through untouched.
func RequirePlanPublic(cfg *config.Config, minPlan string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.CloudMode || !cfg.PublicSiteResolverEnabled {
				next.ServeHTTP(w, r)
				return
			}
			pt, ok := db.PublicTenantFromContext(r.Context())
			if !ok || pt.Store == nil || pt.AccountID == "" {
				writeError(w, http.StatusInternalServerError, "internal_error")
				return
			}
			sub, err := GetCachedSubscription(pt.AccountID, pt.Store)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to load subscription")
				return
			}
			if PlanRank[sub.Plan] < PlanRank[minPlan] {
				writeError(w, http.StatusPaymentRequired, "plan_upgrade_required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
