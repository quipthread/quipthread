//go:build !cloud

package handlers

import (
	"github.com/go-chi/chi/v5"

	cloudpkg "github.com/quipthread/quipthread/cloud"
	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
)

// RegisterInvitationRoutes is a no-op in non-cloud builds.
func RegisterInvitationRoutes(_ chi.Router, _ *config.Config, _ db.Store, _ cloudpkg.Store) {}
