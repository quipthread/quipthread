package middleware

import (
	"net/http"
)

const (
	// MaxNormalBodyBytes is the cap for ordinary JSON, auth, comment, account,
	// admin, and unknown requests. Import and Stripe routes are explicitly
	// selected below so their existing route-specific limits remain authoritative.
	MaxNormalBodyBytes       int64 = 1 << 20
	maxTextImportBodyBytes   int64 = 33 << 20
	maxSQLiteImportBodyBytes int64 = 129 << 20
)

// LimitRequestBody attaches a lazy bounded reader to ordinary requests.
// Authentication, rate limiting, and route selection therefore happen before
// any request body is read. Unknown or rejected routes can return without
// touching a slow/chunked body, while handlers that do read it receive the
// standard http.MaxBytesError after the 1 MiB cap.
func LimitRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit, limited := normalBodyLimit(r.URL.Path)
		if !limited {
			next.ServeHTTP(w, r)
			return
		}

		if r.Body == nil {
			r.Body = http.NoBody
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}

func normalBodyLimit(path string) (int64, bool) {
	switch path {
	case "/api/admin/import/disqus", "/api/admin/import/wordpress", "/api/admin/import/remark42", "/api/admin/import/native":
		return maxTextImportBodyBytes, false
	case "/api/admin/import/quipthread", "/api/admin/import/sqlite/inspect", "/api/admin/import/sqlite/run":
		return maxSQLiteImportBodyBytes, false
	case "/api/billing/webhook":
		// Stripe's handler owns its existing 64 KiB cap.
		return 0, false
	default:
		return MaxNormalBodyBytes, true
	}
}
