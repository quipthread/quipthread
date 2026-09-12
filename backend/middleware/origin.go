package middleware

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/quipthread/quipthread/session"
)

// EnforceDashboardOrigin protects unsafe requests carrying the dashboard
// session cookie. Origin is preferred; Referer is accepted only as a fallback.
func EnforceDashboardOrigin(baseURL string) func(http.Handler) http.Handler {
	expected, valid := canonicalOrigin(baseURL)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isUnsafeMethod(r.Method) || !hasCookie(r, session.DashboardCookieName) {
				next.ServeHTTP(w, r)
				return
			}
			if !valid || requestOrigin(r) != expected {
				writeOriginForbidden(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// EnforceExactOrigin protects an unsafe endpoint that must be called from the
// configured application origin regardless of whether it carries a cookie.
// This is used for public authentication exchanges such as SSO.
func EnforceExactOrigin(baseURL string) func(http.Handler) http.Handler {
	expected, valid := canonicalOrigin(baseURL)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isUnsafeMethod(r.Method) || valid && requestOrigin(r) == expected {
				next.ServeHTTP(w, r)
				return
			}
			writeOriginForbidden(w)
		})
	}
}

// EnforceEmbedOrigin protects self-hosted embed mutations using the explicit
// publisher allowlist. Cloud routes use site-bound origin enforcement instead.
func EnforceEmbedOrigin(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	valid := len(allowedOrigins) > 0
	for _, raw := range allowedOrigins {
		origin, ok := canonicalOrigin(raw)
		if !ok {
			valid = false
			continue
		}
		allowed[origin] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isUnsafeMethod(r.Method) || !hasCookie(r, session.EmbedCookieName) {
				next.ServeHTTP(w, r)
				return
			}
			origin := requestOrigin(r)
			if !valid {
				writeOriginForbidden(w)
				return
			}
			if _, ok := allowed[origin]; !ok {
				writeOriginForbidden(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isUnsafeMethod(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete
}

func hasCookie(r *http.Request, name string) bool {
	_, err := r.Cookie(name)
	return err == nil
}

func requestOrigin(r *http.Request) string {
	if raw := r.Header.Get("Origin"); raw != "" {
		origin, _ := canonicalOrigin(raw)
		return origin
	}
	raw := strings.TrimSpace(r.Header.Get("Referer"))
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil {
		return ""
	}
	origin, _ := canonicalOrigin(u.Scheme + "://" + u.Host)
	return origin
}

func writeOriginForbidden(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`{"error":"csrf_origin_forbidden"}`))
}
