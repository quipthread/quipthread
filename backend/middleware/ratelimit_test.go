package middleware

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"
)

// ---- MemoryRateLimiter ------------------------------------------------------

func TestMemoryRateLimiter_AllowsUpToLimit(t *testing.T) {
	rl := NewMemoryRateLimiter(3, time.Minute)
	for i := range 3 {
		if !rl.Allow(context.Background(), "ip1") {
			t.Errorf("request %d: expected allowed", i+1)
		}
	}
}

func TestMemoryRateLimiter_BlocksAtLimit(t *testing.T) {
	rl := NewMemoryRateLimiter(3, time.Minute)
	for range 3 {
		rl.Allow(context.Background(), "ip1")
	}
	if rl.Allow(context.Background(), "ip1") {
		t.Error("4th request: expected blocked, got allowed")
	}
}

func TestMemoryRateLimiter_IndependentKeys(t *testing.T) {
	rl := NewMemoryRateLimiter(1, time.Minute)
	rl.Allow(context.Background(), "ip1")
	if !rl.Allow(context.Background(), "ip2") {
		t.Error("ip2: expected allowed (independent key), got blocked")
	}
}

func TestMemoryRateLimiter_WindowExpiry(t *testing.T) {
	rl := NewMemoryRateLimiter(2, 50*time.Millisecond)
	rl.Allow(context.Background(), "ip1")
	rl.Allow(context.Background(), "ip1")

	if rl.Allow(context.Background(), "ip1") {
		t.Error("at limit before expiry: expected blocked, got allowed")
	}

	time.Sleep(60 * time.Millisecond)

	if !rl.Allow(context.Background(), "ip1") {
		t.Error("after window expiry: expected allowed, got blocked")
	}
}

// ---- ParseWindow ------------------------------------------------------------

func TestParseWindow_Valid(t *testing.T) {
	cases := []struct {
		input string
		count int
		dur   time.Duration
	}{
		{"5/10m", 5, 10 * time.Minute},
		{"10/1h", 10, time.Hour},
		{"1/30s", 1, 30 * time.Second},
	}
	for _, tc := range cases {
		count, dur, err := ParseWindow(tc.input)
		if err != nil {
			t.Errorf("ParseWindow(%q): unexpected error: %v", tc.input, err)
			continue
		}
		if count != tc.count {
			t.Errorf("ParseWindow(%q) count = %d, want %d", tc.input, count, tc.count)
		}
		if dur != tc.dur {
			t.Errorf("ParseWindow(%q) dur = %v, want %v", tc.input, dur, tc.dur)
		}
	}
}

func TestParseWindow_Invalid(t *testing.T) {
	cases := []string{
		"",
		"5",
		"0/1m",
		"-1/1m",
		"abc/1m",
		"5/invalid",
	}
	for _, s := range cases {
		if _, _, err := ParseWindow(s); err == nil {
			t.Errorf("ParseWindow(%q): expected error, got nil", s)
		}
	}
}

// ---- RealIP / RemoteAddrIP --------------------------------------------------

func TestRealIP_XForwardedFor(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	if got := RealIP(r); got != "1.2.3.4" {
		t.Errorf("RealIP(X-Forwarded-For): got %q, want 1.2.3.4", got)
	}
}

func TestRealIP_XRealIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Real-IP", "9.10.11.12")
	if got := RealIP(r); got != "9.10.11.12" {
		t.Errorf("RealIP(X-Real-IP): got %q, want 9.10.11.12", got)
	}
}

func TestRealIP_FallsBackToRemoteAddr(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.168.1.1:1234"
	if got := RealIP(r); got != "192.168.1.1" {
		t.Errorf("RealIP(RemoteAddr fallback): got %q, want 192.168.1.1", got)
	}
}

func TestRemoteAddrIP_IgnoresProxyHeaders(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Forwarded-For", "1.2.3.4") // must be ignored
	r.RemoteAddr = "10.0.0.1:4321"
	if got := RemoteAddrIP(r); got != "10.0.0.1" {
		t.Errorf("RemoteAddrIP: got %q, want 10.0.0.1", got)
	}
}
