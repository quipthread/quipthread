package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSFailsClosedWithUnsetAllowlist(t *testing.T) {
	h := CORS(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
	h := CORS([]string{"HTTPS://Publisher.Example:443/"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
