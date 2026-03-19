package main

import (
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// newDevProxy returns an http.Handler that reverse-proxies all requests to
// rawURL (the Astro dev server). WebSocket upgrades (Vite HMR) are handled
// via a raw TCP tunnel so the bidirectional stream is preserved.
func newDevProxy(rawURL string) http.Handler {
	target, err := url.Parse(rawURL)
	if err != nil {
		panic("invalid DEV_DASHBOARD_URL: " + err.Error())
	}
	rp := &httputil.ReverseProxy{
		// Use Rewrite (Go 1.20+) instead of the deprecated Director.
		// SetURL rewrites scheme/host/path; then we fix Host and strip cookies.
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target) //nolint:gosec // G704: dev-only proxy; target is DEV_DASHBOARD_URL (developer config), not user input
			pr.Out.Host = target.Host
			// Vite rejects requests whose Host header doesn't match the dev server host
			// (DNS rebinding protection) — SetURL alone doesn't fix the Host header.
			// Astro dev server doesn't need auth cookies, and Node.js's HTTP parser
			// will throw "Header overflow" if the JWT cookie pushes headers past ~8KB.
			pr.Out.Header.Del("Cookie")
		},
		// Disable keep-alives to prevent "unsolicited response on idle connection"
		// warnings caused by the Astro/Vite dev server closing connections abruptly.
		Transport: &http.Transport{DisableKeepAlives: true},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			proxyWebSocket(target, w, r)
			return
		}
		rp.ServeHTTP(w, r)
	})
}

func proxyWebSocket(target *url.URL, w http.ResponseWriter, r *http.Request) {
	dst, err := net.Dial("tcp", target.Host) //nolint:gosec,noctx // G704: dev-only proxy; target is DEV_DASHBOARD_URL (developer config), not user input
	if err != nil {
		http.Error(w, "websocket proxy error", http.StatusBadGateway)
		return
	}
	defer dst.Close() //nolint:errcheck // deferred close; connection cleanup on exit

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}
	src, _, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer src.Close() //nolint:errcheck // deferred close; connection cleanup on exit

	if err := r.Write(dst); err != nil {
		return
	}

	done := make(chan struct{}, 2)
	cp := func(a, b net.Conn) {
		io.Copy(a, b) //nolint:errcheck,gosec // dev proxy; bidirectional copy errors are not actionable
		done <- struct{}{}
	}
	go cp(dst, src)
	go cp(src, dst)
	<-done
}
