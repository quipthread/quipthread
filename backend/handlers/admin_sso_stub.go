//go:build !cloud

package handlers

import "github.com/go-chi/chi/v5"

// RegisterSSOAdminRoutes is a no-op in non-cloud builds.
func RegisterSSOAdminRoutes(_ chi.Router, _ *AdminHandler) {}
