package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RateLimiter is the interface that rate limit backends must implement.
// Allow returns true if the request should proceed. Implement this interface
// to swap in a Redis or other distributed backend without changing call sites.
type RateLimiter interface {
	Allow(ctx context.Context, key string) bool
}

// RateLimit returns a chi-compatible middleware that rejects requests exceeding
// the rate limit for the client IP. ipFn determines how the client IP is
// extracted — pass RealIP when behind a trusted reverse proxy, or
// RemoteAddrIP when the backend is directly exposed.
func RateLimit(rl RateLimiter, ipFn func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !rl.Allow(r.Context(), ipFn(r)) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"rate limit exceeded"}`)) //nolint:errcheck // error response; connection may already be broken
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// MemoryRateLimiter is a per-key sliding-window rate limiter backed by an
// in-memory map. It is safe for concurrent use. For multi-instance deployments
// swap this out for a Redis-backed implementation of RateLimiter.
type MemoryRateLimiter struct {
	mu      sync.Mutex
	windows map[string][]time.Time
	count   int
	period  time.Duration
}

// NewMemoryRateLimiter creates a limiter that allows at most count requests
// within period for each unique key.
func NewMemoryRateLimiter(count int, period time.Duration) *MemoryRateLimiter {
	return &MemoryRateLimiter{
		windows: make(map[string][]time.Time),
		count:   count,
		period:  period,
	}
}

func (rl *MemoryRateLimiter) Allow(_ context.Context, key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.period)

	// Rebuild the window, keeping only timestamps within the period.
	prev := rl.windows[key]
	valid := prev[:0]
	for _, t := range prev {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.count {
		rl.windows[key] = valid
		return false
	}

	if len(valid) == 0 {
		// Window cleared — start a fresh slice so the old backing array is freed.
		rl.windows[key] = []time.Time{now}
	} else {
		rl.windows[key] = append(valid, now)
	}
	return true
}

// ParseWindow parses a rate limit string of the form "count/duration",
// e.g. "5/10m" or "10/1h". Duration uses Go's time.ParseDuration format.
func ParseWindow(s string) (int, time.Duration, error) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid rate limit format %q: want count/duration (e.g. 5/10m)", s)
	}
	count, err := strconv.Atoi(parts[0])
	if err != nil || count < 1 {
		return 0, 0, fmt.Errorf("invalid count in rate limit %q", s)
	}
	dur, err := time.ParseDuration(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid duration in rate limit %q: %w", s, err)
	}
	return count, dur, nil
}

// RealIP extracts the client IP from X-Forwarded-For, X-Real-IP, or RemoteAddr.
// Only use this when the backend is behind a trusted reverse proxy (TRUST_PROXY=true);
// otherwise clients can spoof the header and bypass rate limits.
func RealIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ip
}

// RemoteAddrIP extracts the client IP solely from the TCP connection address,
// ignoring all proxy headers. Safe to use when the backend is directly exposed.
func RemoteAddrIP(r *http.Request) string {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ip
}
