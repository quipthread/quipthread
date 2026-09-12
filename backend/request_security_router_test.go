package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/quipthread/quipthread/middleware"
	"github.com/quipthread/quipthread/session"
)

func TestRequestSecurityRouterSeparatesDashboardAndPublisherOrigins(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.CORSWithBaseURL("https://app.example", []string{"https://publisher.example"}))
	r.Use(middleware.LimitRequestBody)
	r.Use(middleware.EnforceDashboardOrigin("https://app.example"))
	r.Post("/api/admin/test", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	r.Group(func(g chi.Router) {
		g.Use(middleware.EnforceEmbedOrigin([]string{"https://publisher.example"}))
		g.Post("/api/comments/test", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
	})

	request := func(method, path, origin, cookie string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Origin", origin)
		req.AddCookie(&http.Cookie{Name: cookie, Value: "session"})
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}

	if rr := request(http.MethodPost, "/api/admin/test", "https://app.example", session.DashboardCookieName); rr.Code != http.StatusNoContent {
		t.Fatalf("dashboard POST status = %d, want 204", rr.Code)
	}
	if rr := request(http.MethodPost, "/api/comments/test", "https://publisher.example", session.EmbedCookieName); rr.Code != http.StatusNoContent {
		t.Fatalf("publisher embed mutation status = %d, want 204", rr.Code)
	}
	if rr := request(http.MethodPost, "/api/comments/test", "https://app.example", session.EmbedCookieName); rr.Code != http.StatusForbidden {
		t.Fatalf("dashboard origin on embed mutation status = %d, want 403", rr.Code)
	}
	if rr := request(http.MethodPost, "/api/comments/test", "https://evil.example", session.EmbedCookieName); rr.Code != http.StatusForbidden {
		t.Fatalf("evil embed mutation status = %d, want 403", rr.Code)
	}
}
