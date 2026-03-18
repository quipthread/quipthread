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
