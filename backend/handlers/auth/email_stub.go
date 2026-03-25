//go:build !cloud

package auth

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/quipthread/quipthread/middleware"
)

// RegisterCloudAuthRoutes is a no-op in non-cloud builds.
func RegisterCloudAuthRoutes(_ chi.Router, _ *Handler, _ middleware.RateLimiter, _ func(*http.Request) string) {
}

func (h *Handler) cloudEmailRegister(_ http.ResponseWriter, _ *http.Request) bool { return false }
func (h *Handler) cloudEmailLogin(_ http.ResponseWriter, _ *http.Request) bool    { return false }
func (h *Handler) cloudForgot(_ http.ResponseWriter, _ *http.Request) bool        { return false }
func (h *Handler) cloudEmailResend(_ http.ResponseWriter, _ *http.Request) bool   { return false }
