package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/quipthread/quipthread/session"
)

func TestEnforceDashboardOriginUsesRefererOnlyAsFallback(t *testing.T) {
	h := EnforceDashboardOrigin("https://app.example")(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	for _, tc := range []struct {
		name     string
		origin   string
		referer  string
		wantCode int
	}{
		{"same origin", "https://app.example", "", http.StatusNoContent},
		{"same origin referer", "", "https://app.example/dashboard", http.StatusNoContent},
		{"cross origin", "https://evil.example", "https://app.example/dashboard", http.StatusForbidden},
		{"missing origin", "", "", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/admin/comments", nil)
			r.AddCookie(&http.Cookie{Name: session.DashboardCookieName, Value: "session"})
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if tc.referer != "" {
				r.Header.Set("Referer", tc.referer)
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, r)
			if rr.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d", rr.Code, tc.wantCode)
			}
		})
	}
}

func TestEnforceEmbedOriginRequiresConfiguredPublisher(t *testing.T) {
	h := EnforceEmbedOrigin([]string{"https://publisher.example"})(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	for _, origin := range []string{"https://publisher.example", "https://evil.example", ""} {
		r := httptest.NewRequest(http.MethodPost, "/api/comments", nil)
		r.AddCookie(&http.Cookie{Name: session.EmbedCookieName, Value: "session"})
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, r)
		want := http.StatusNoContent
		if origin != "https://publisher.example" {
			want = http.StatusForbidden
		}
		if rr.Code != want {
			t.Fatalf("origin %q: status = %d, want %d", origin, rr.Code, want)
		}
	}
}
