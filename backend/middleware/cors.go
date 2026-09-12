package middleware

import (
	"net/http"
	"net/url"
	"strings"
)

// CORS returns a middleware that sets appropriate CORS headers for the embed
// widget and dashboard. allowedOrigins must list exact, canonical origins
// (e.g. "https://yoursite.com"). An empty or malformed allowlist fails closed.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	return CORSWithBaseURL("", allowedOrigins)
}

// CORSWithBaseURL permits the dashboard's canonical application origin in
// addition to the publisher-only allowlist used by the embed widget.
func CORSWithBaseURL(baseURL string, allowedOrigins []string) func(http.Handler) http.Handler {
	allOrigins := make([]string, 0, len(allowedOrigins)+1)
	allOrigins = append(allOrigins, allowedOrigins...)
	if strings.TrimSpace(baseURL) != "" {
		allOrigins = append(allOrigins, baseURL)
	}

	originSet := make(map[string]struct{}, len(allOrigins))
	configurationInvalid := len(allOrigins) == 0
	for _, o := range allOrigins {
		canonical, ok := canonicalOrigin(o)
		if !ok {
			configurationInvalid = true
			continue
		}
		originSet[canonical] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			w.Header().Add("Vary", "Origin")

			allowed := false
			if !configurationInvalid {
				if canonical, ok := canonicalOrigin(origin); ok {
					_, allowed = originSet[canonical]
				}
			}

			if origin != "" && !allowed {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"cors_origin_forbidden"}`))
				return
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Max-Age", "86400")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// canonicalOrigin validates an origin and normalizes case and default ports.
// It intentionally rejects paths and other URL components so configured
// values cannot accidentally widen an origin comparison.
func canonicalOrigin(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "http" && u.Scheme != "https" || u.Host == "" || u.User != nil {
		return "", false
	}
	if u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", false
	}
	port := u.Port()
	if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		host = u.Hostname()
		if strings.Contains(host, ":") {
			host = "[" + strings.ToLower(host) + "]"
		}
		host += ":" + port
	}
	return strings.ToLower(u.Scheme) + "://" + host, true
}
