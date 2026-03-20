package middleware

import (
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/quipthread/quipthread/cloud"
	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/session"
)

// StoreCache holds open tenant store connections keyed by account ID.
type StoreCache struct {
	mu     sync.Mutex
	stores map[string]db.Store
}

func NewStoreCache() *StoreCache {
	return &StoreCache{stores: make(map[string]db.Store)}
}

func (c *StoreCache) Get(accountID string) (db.Store, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.stores[accountID]
	return s, ok
}

func (c *StoreCache) Set(accountID string, s db.Store) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stores[accountID] = s
}

func (c *StoreCache) Evict(accountID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.stores, accountID)
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

			// Static assets and public pages that don't need a tenant store —
			// skip resolution so a logged-in user's session cookie doesn't
			// block delivery, and so unauthenticated access never returns 500
			// from a failed tenant lookup.
			if strings.HasPrefix(r.URL.Path, "/_astro/") ||
				r.URL.Path == "/favicon.svg" ||
				r.URL.Path == "/embed.js" ||
				r.URL.Path == "/embed-preview" {
				next.ServeHTTP(w, r)
				return
			}

			// Claims may already be in context (set by RequireAuth/RequireAdmin),
			// or need to be parsed directly from the cookie — InjectTenantStore
			// runs as a global middleware before the auth group middleware.
			claims, _ := r.Context().Value(session.UserKey).(*session.Claims)
			if claims == nil {
				if cookie, err := r.Cookie(session.CookieName); err == nil {
					claims, _ = session.Parse(cfg.JWTSecret, cookie.Value)
				}
			}
			if claims == nil || claims.AccountID == "" {
				next.ServeHTTP(w, r)
				return
			}

			if s, ok := cache.Get(claims.AccountID); ok {
				next.ServeHTTP(w, r.WithContext(db.WithStore(r.Context(), s)))
				return
			}

			acc, err := cloudStore.GetAccountByID(claims.AccountID)
			if err != nil {
				// Do not fall through to the global store — that would expose
				// other tenants' data to an authenticated but unresolvable account.
				http.Error(w, `{"error":"tenant resolution failed"}`, http.StatusInternalServerError)
				return
			}

			var s db.Store
			if acc.DBType == "turso" {
				dsn := acc.DBURL
				if cfg.TursoAuthToken != "" && !strings.Contains(dsn, "authToken=") {
					sep := "?"
					if strings.Contains(dsn, "?") {
						sep = "&"
					}
					dsn = dsn + sep + "authToken=" + url.QueryEscape(cfg.TursoAuthToken)
				}
				s, err = db.NewLibSQLStore(dsn)
			} else {
				s, err = db.NewSQLiteStore(acc.DBURL)
			}
			if err != nil {
				// Store open failure — same principle, never fall through to global.
				http.Error(w, `{"error":"tenant store unavailable"}`, http.StatusInternalServerError)
				return
			}

			cache.Set(claims.AccountID, s)
			next.ServeHTTP(w, r.WithContext(db.WithStore(r.Context(), s)))
		})
	}
}
