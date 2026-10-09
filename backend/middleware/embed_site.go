package middleware

import (
	"errors"
	"net/http"

	"github.com/quipthread/quipthread/cloud"
	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/models"
)

var (
	// ErrEmbedSiteNotFound means the site is unknown or not active.
	ErrEmbedSiteNotFound = errors.New("embed site not found")
	// ErrEmbedSiteUnavailable means the site's tenant could not be reached.
	ErrEmbedSiteUnavailable = errors.New("embed site unavailable")
)

// EmbedSite is a site resolved for an embed authentication flow: the store
// that owns it, the site row, and the owning account (empty when self-hosted).
// Callers must Release it when finished with Store.
type EmbedSite struct {
	Store     db.Store
	Site      *models.Site
	AccountID string
	lease     *StoreLease
}

// Release returns the tenant store lease. Safe to call on a nil EmbedSite.
func (s *EmbedSite) Release() {
	if s != nil {
		s.lease.Release()
	}
}

// EmbedSites resolves embed sites outside the request middleware chain, for
// flows such as OAuth callbacks and SSO exchanges whose site arrives in OAuth
// state or a JSON body. In cloud mode the site registry selects the tenant and
// there is no fallback to the global store; self-hosted reads the single store.
type EmbedSites struct {
	store      db.Store
	cloudStore cloud.Store
	cache      *StoreCache
	cfg        *config.Config
}

func NewEmbedSites(store db.Store, cloudStore cloud.Store, cache *StoreCache, cfg *config.Config) *EmbedSites {
	return &EmbedSites{store: store, cloudStore: cloudStore, cache: cache, cfg: cfg}
}

func (e *EmbedSites) cloud() bool {
	return e.cfg.CloudMode
}

// Resolve looks up siteID and returns its owning store.
func (e *EmbedSites) Resolve(siteID string) (*EmbedSite, error) {
	if siteID == "" {
		return nil, ErrEmbedSiteNotFound
	}
	if e.cloud() {
		if e.cloudStore == nil || e.cache == nil {
			return nil, ErrEmbedSiteUnavailable
		}
		pt, site, lease, failure := resolvePublicSite(e.cloudStore, e.cache, e.cfg, siteID)
		if failure != nil {
			if failure.status == http.StatusNotFound {
				return nil, ErrEmbedSiteNotFound
			}
			return nil, ErrEmbedSiteUnavailable
		}
		return &EmbedSite{Store: pt.Store, Site: site, AccountID: pt.AccountID, lease: lease}, nil
	}

	site, err := e.store.GetSite(siteID)
	if err != nil {
		return nil, ErrEmbedSiteUnavailable
	}
	if site == nil {
		return nil, ErrEmbedSiteNotFound
	}
	return &EmbedSite{Store: e.store, Site: site}, nil
}

// OriginAllowed reports whether origin may host the embed for site: it must
// exactly match the site's configured domain. Self-hosted deployments also
// accept the operator's ALLOWED_ORIGINS, since sites there predate domain
// validation and the allowlist is the established publisher boundary.
func (e *EmbedSites) OriginAllowed(site *models.Site, origin string) bool {
	u, ok := parseOriginHeader(origin)
	if !ok {
		return false
	}
	if d, ok := normalizeSiteDomain(site.Domain); ok && originMatchesDomain(u, d) {
		return true
	}
	if e.cloud() {
		return false
	}
	canonical, ok := canonicalOrigin(origin)
	if !ok {
		return false
	}
	for _, allowed := range e.cfg.AllowedOrigins {
		if a, ok := canonicalOrigin(allowed); ok && a == canonical {
			return true
		}
	}
	return false
}
