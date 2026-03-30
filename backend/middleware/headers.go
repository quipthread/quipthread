package middleware

import "net/http"

// SecurityHeaders sets security-related response headers on every request.
// Browsers only apply CSP directives to HTML documents, so including it on
// API (JSON) and static asset responses is harmless — those content types are
// unaffected. The same middleware handles all routes for simplicity.
func SecurityHeaders(next http.Handler) http.Handler {
	// 'unsafe-inline' is required for script-src and style-src because Astro
	// emits inline <script> hydration blocks and inline <style> tags in the
	// built dashboard pages, and the approve page contains an inline script.
	// Migrating to nonces would require Astro build pipeline changes — deferred.
	//
	// fonts.googleapis.com / fonts.gstatic.com: Google Fonts loaded by the
	// dashboard auth pages (DM Sans, Lora) and landing layout.
	//
	// img-src https: permits user avatar images from OAuth providers (GitHub
	// Avatars CDN, Google User Content) without hard-coding specific hostnames
	// that may change. data: permits base64 inline images.
	//
	// frame-ancestors 'self': allows the dashboard to iframe its own embed
	// preview (/embed-preview) while blocking third-party framing.
	const csp = "default-src 'self'; " +
		"script-src 'self' 'unsafe-inline'; " +
		"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
		"font-src 'self' https://fonts.gstatic.com; " +
		"img-src 'self' data: https:; " +
		"connect-src 'self'; " +
		"frame-ancestors 'self'; " +
		"base-uri 'self'; " +
		"form-action 'self'"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}
