package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCORSFailsClosedWithUnsetAllowlist(t *testing.T) {
	h := CORSWithBaseURL("", nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "https://publisher.example")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("disallowed origin received CORS permission")
	}
	if rr.Header().Get("Vary") != "Origin" {
		t.Fatalf("Vary = %q, want Origin", rr.Header().Get("Vary"))
	}
}

func TestCORSMatchesCanonicalExactOrigin(t *testing.T) {
	h := CORSWithBaseURL("", []string{"HTTPS://Publisher.Example:443/"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "https://publisher.example")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "https://publisher.example" {
		t.Fatal("canonical exact origin was not reflected")
	}
}

func TestCORSEmbedRoutesAreNonCredentialedForAnyOrigin(t *testing.T) {
	h := CORSWithBaseURL("https://app.example", []string{"https://publisher.example"})(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	for _, path := range []string{"/api/config", "/api/comments", "/api/comments/c1/vote", "/api/auth/embed/me", "/api/auth/sso"} {
		for _, method := range []string{http.MethodOptions, http.MethodGet} {
			r := httptest.NewRequest(method, path+"?siteId=s1", nil)
			r.Header.Set("Origin", "https://any-customer.example")
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, r)
			if rr.Code == http.StatusForbidden {
				t.Fatalf("%s %s: forbidden for a publisher origin", method, path)
			}
			if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
				t.Fatalf("%s %s: Allow-Origin = %q, want *", method, path, got)
			}
			if got := rr.Header().Get("Access-Control-Allow-Credentials"); got != "" {
				t.Fatalf("%s %s: Allow-Credentials = %q, want none", method, path, got)
			}
			if !strings.Contains(rr.Header().Get("Access-Control-Allow-Headers"), "Authorization") {
				t.Fatalf("%s %s: Authorization header not allowed", method, path)
			}
		}
	}
}

func TestCORSNonEmbedRoutesStayStrict(t *testing.T) {
	h := CORSWithBaseURL("https://app.example", []string{"https://publisher.example"})(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	for _, path := range []string{"/api/admin/comments", "/api/billing/status", "/api/auth/dashboard/me", "/api/commentsx"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Header.Set("Origin", "https://any-customer.example")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, r)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("%s: status = %d, want 403 for an unlisted origin", path, rr.Code)
		}
	}
}
