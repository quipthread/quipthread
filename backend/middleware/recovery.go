package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recovery is a panic recovery middleware that logs the stack trace via slog
// before returning a 500 to the client. It replaces chi's built-in Recoverer
// so that panics are visible in the log stream rather than silently swallowed.
//
// http.ErrAbortHandler is re-panicked to preserve its special semantics
// (aborts the response without logging a spurious error).
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			err := recover()
			if err == nil {
				return
			}
			// http.ErrAbortHandler is a sentinel used to abort a handler mid-stream
			// without writing an error response. Re-panic to preserve that contract.
			if err == http.ErrAbortHandler { //nolint:errorlint // recover() returns any; http.ErrAbortHandler is a sentinel never wrapped
				panic(err)
			}
			slog.ErrorContext(r.Context(), "panic recovered",
				"error", err,
				"method", r.Method,
				"path", r.URL.Path,
				"stack", string(debug.Stack()),
			)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}()
		next.ServeHTTP(w, r)
	})
}
