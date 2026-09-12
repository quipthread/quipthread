//go:build !cloud

package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Self-hosted compatibility: in non-cloud builds the SSO admin routes are not
// registered at all, so requests must 404 rather than reach any handler.
func TestRegisterSSOAdminRoutesNoopInNonCloudBuild(t *testing.T) {
	r := chi.NewRouter()
	RegisterSSOAdminRoutes(r, nil) // handler must never be reached; nil is safe

	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		req := httptest.NewRequest(method, "/api/admin/sites/s1/sso", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s status = %d, want 404 (route must not exist)", method, rec.Code)
		}
	}
}
