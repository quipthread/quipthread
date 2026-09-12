//go:build !cloud

package auth

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// RegisterSSORoutes is a no-op in non-cloud builds.
func RegisterSSORoutes(_ chi.Router, _ *Handler, _ ...func(http.Handler) http.Handler) {}
