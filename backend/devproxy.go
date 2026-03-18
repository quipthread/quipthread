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
	rp := httputil.NewSingleHostReverseProxy(target)
	// Disable keep-alives to prevent "unsolicited response on idle connection"
	// warnings caused by the Astro/Vite dev server closing connections abruptly.
	rp.Transport = &http.Transport{DisableKeepAlives: true}
	// Vite rejects requests whose Host header doesn't match the dev server host
	// (DNS rebinding protection). Override the director to rewrite Host.
	base := rp.Director
	rp.Director = func(req *http.Request) {
		base(req)
		req.Host = target.Host
		// Astro dev server doesn't need auth cookies, and Node.js's HTTP parser
		// will throw "Header overflow" if the JWT cookie pushes headers past ~8KB.
		req.Header.Del("Cookie")
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
	dst, err := net.Dial("tcp", target.Host)
	if err != nil {
		http.Error(w, "websocket proxy error", http.StatusBadGateway)
		return
	}
	defer dst.Close()

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}
	src, _, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer src.Close()

	if err := r.Write(dst); err != nil {
		return
	}

	done := make(chan struct{}, 2)
	cp := func(a, b net.Conn) {
		io.Copy(a, b)
		done <- struct{}{}
	}
	go cp(dst, src)
	go cp(src, dst)
	<-done
}
