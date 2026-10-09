package middleware

import (
	"net/http"
	"net/url"
	"strings"
)

// CORSWithBaseURL sets CORS headers for the embed widget and dashboard. It
// permits the dashboard's canonical application origin (baseURL) in addition
// to allowedOrigins, which must list exact, canonical origins (e.g.
// "https://yoursite.com"). An empty or malformed allowlist fails closed.
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
			if IsEmbedAPIRoute(r.URL.Path) {
				embedCORS(w, r, next)
				return
			}

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

// IsEmbedAPIRoute reports whether path is part of the embed widget's API.
// The widget runs on publishers' origins and authenticates with site-bound
// bearer tokens, never cookies, so these routes are served to any origin
// without credentials. Site binding and per-site origin checks on mutations
// are enforced by the routes themselves.
func IsEmbedAPIRoute(path string) bool {
	switch path {
	case "/api/config", "/api/comments", "/api/auth/embed/me", "/api/auth/sso":
		return true
	}
	return strings.HasPrefix(path, "/api/comments/")
}

func embedCORS(w http.ResponseWriter, r *http.Request, next http.Handler) {
	h := w.Header()
	if r.Header.Get("Origin") != "" {
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		h.Set("Access-Control-Max-Age", "86400")
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	next.ServeHTTP(w, r)
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
