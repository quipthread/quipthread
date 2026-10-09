//go:build !cloud

package handlers

import (
	"github.com/go-chi/chi/v5"

	cloudpkg "github.com/quipthread/quipthread/cloud"
	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/middleware"
)

// RegisterAccountDeletionRoutes is a no-op in non-cloud builds: self-hosted
// installs have no hosted account to delete.
func RegisterAccountDeletionRoutes(_ chi.Router, _ *config.Config, _ cloudpkg.Store, _ *middleware.StoreCache) {
}
