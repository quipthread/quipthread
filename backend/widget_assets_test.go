package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/go-chi/chi/v5"
)

func TestWidgetRoutes(t *testing.T) {
	r := chi.NewRouter()
	registerWidgetRoutes(r, fstest.MapFS{
		"embed.js":                     {Data: []byte("loader")},
		"embed/widget-4OYq4DyW.js":     {Data: []byte("widget")},
		"embed/jsxRuntime.module-x.js": {Data: []byte("runtime")},
		"secret.txt":                   {Data: []byte("not served")},
	})

	for _, tc := range []struct {
		path, body, cache string
		status            int
	}{
		{"/embed.js", "loader", "public, max-age=300", http.StatusOK},
		{"/embed/widget-4OYq4DyW.js", "widget", "public, max-age=31536000, immutable", http.StatusOK},
		{"/embed/jsxRuntime.module-x.js", "runtime", "public, max-age=31536000, immutable", http.StatusOK},
		{"/embed/missing-12345678.js", "", "", http.StatusNotFound},
		{"/embed/secret.txt", "", "", http.StatusNotFound},
		{"/embed/..%2fsecret.txt", "", "", http.StatusNotFound},
		{"/embed/.hidden.js", "", "", http.StatusNotFound},
	} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != tc.status {
			t.Errorf("%s: status %d, want %d", tc.path, rec.Code, tc.status)
			continue
		}
		if tc.status != http.StatusOK {
			continue
		}
		h := rec.Header()
		if rec.Body.String() != tc.body || h.Get("Cache-Control") != tc.cache {
			t.Errorf("%s: body %q cache %q", tc.path, rec.Body.String(), h.Get("Cache-Control"))
		}
		// Host pages import the modules cross-origin.
		if h.Get("Access-Control-Allow-Origin") != "*" || h.Get("Content-Type") != "text/javascript; charset=utf-8" {
			t.Errorf("%s: headers %v", tc.path, h)
		}
	}
}
