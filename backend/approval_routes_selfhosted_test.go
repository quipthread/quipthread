//go:build !cloud

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/models"
)

// selfhostedApprovalStore backs the self-hosted registration test: it embeds
// db.Store and implements just enough to observe that the real, store-backed
// approval handlers remain constructed and invoked exactly as before.
type selfhostedApprovalStore struct {
	db.Store

	getApprovalTokenCalls atomic.Int32
}

func (s *selfhostedApprovalStore) GetApprovalToken(token string) (*models.ApprovalToken, error) {
	s.getApprovalTokenCalls.Add(1)
	return nil, nil
}

// TestRegisterApprovalRoutes_SelfHostedUnchanged proves the containment gate
// leaves the original wiring intact when CloudMode is false: the store-backed
// handlers are constructed and consulted by both routes and keep their legacy
// status codes for an unknown token.
func TestRegisterApprovalRoutes_SelfHostedUnchanged(t *testing.T) {
	st := &selfhostedApprovalStore{}
	r := chi.NewRouter()
	registerApprovalRoutes(r, &config.Config{CloudMode: false}, st, nil, nil)

	t.Run("GET page consults store and keeps legacy 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/approve/some-token", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
		if got := st.getApprovalTokenCalls.Load(); got != 1 {
			t.Fatalf("GetApprovalToken calls = %d, want 1", got)
		}
	})

	t.Run("POST action consults store and keeps legacy 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/approve/some-token", strings.NewReader(`{"action":"approve"}`))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
		if !strings.Contains(rec.Body.String(), `"error":"token not found or already used"`) {
			t.Fatalf("body = %q, want legacy JSON token-not-found error", rec.Body.String())
		}
	})
}
