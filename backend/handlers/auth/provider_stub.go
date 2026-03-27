//go:build !cloud

package auth

import (
	"net/http"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
)

type cloudExtras struct{} //nolint:unused // used by embedded field in provider.go; populated in cloud builds

// NewHandler constructs a Handler.
func NewHandler(store db.Store, cfg *config.Config) *Handler {
	return newHandler(store, cfg)
}

// NewHandlerWithCloud constructs a Handler, ignoring the unused cloud store argument.
func NewHandlerWithCloud(store db.Store, cfg *config.Config, _ any) *Handler {
	return NewHandler(store, cfg)
}

// cloudHandleLinkCallback is a no-op stub; returns false to use self-hosted logic.
func (h *Handler) cloudHandleLinkCallback(_ http.ResponseWriter, _ *http.Request, _ *UserInfo, _ string) bool {
	return false
}

// cloudUpsertAndIssueToken is a no-op stub that returns false so the caller
// falls through to the self-hosted login path. Reached only when CLOUD_MODE=true
// is set at runtime but the binary was built without the cloud tag.
func (h *Handler) cloudUpsertAndIssueToken(_ http.ResponseWriter, _ *http.Request, _ *UserInfo) bool {
	return false
}
