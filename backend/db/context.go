package db

import "context"

type contextKey struct{}

func WithStore(ctx context.Context, s Store) context.Context {
	return context.WithValue(ctx, contextKey{}, s)
}

func StoreFromContext(ctx context.Context) (Store, bool) {
	s, ok := ctx.Value(contextKey{}).(Store)
	return s, ok
}

type publicTenantKey struct{}

// PublicTenant is the explicit result of public-site tenant resolution: the
// tenant store for the account that owns the requested site, plus the site and
// account IDs the resolution was based on. Its presence — not a generic
// StoreFromContext hit — is the only proof that a request was resolved via the
// public site resolver.
type PublicTenant struct {
	Store     Store
	SiteID    string
	AccountID string
}

func WithPublicTenant(ctx context.Context, pt PublicTenant) context.Context {
	return context.WithValue(ctx, publicTenantKey{}, pt)
}

func PublicTenantFromContext(ctx context.Context) (PublicTenant, bool) {
	pt, ok := ctx.Value(publicTenantKey{}).(PublicTenant)
	return pt, ok
}
