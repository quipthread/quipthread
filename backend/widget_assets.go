package main

import (
	"io/fs"
	"net/http"
	"path"
	"regexp"

	"github.com/go-chi/chi/v5"
)

// widgetChunkName matches the module file names the embed build writes to
// embed/ (for example widget-4OYq4DyW.js); files are only ever read from the
// widget build itself.
var widgetChunkName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*\.js$`)

// registerWidgetRoutes serves the comment widget that sites embed:
//
//   - /embed.js is the loader sites include. Its name never changes, so it is
//     cached briefly and a deploy reaches readers within minutes.
//   - /embed/<name>-<hash>.js are the modules it loads. Their names change with
//     their content, so they are cached for good. Host pages load them as
//     cross-origin module scripts, which requires CORS.
func registerWidgetRoutes(r chi.Router, assets fs.FS) {
	r.Get("/embed.js", func(w http.ResponseWriter, r *http.Request) {
		serveWidgetFile(w, r, assets, "embed.js", "public, max-age=300")
	})
	r.Get("/embed/{file}", func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "file")
		if !widgetChunkName.MatchString(name) {
			http.NotFound(w, r)
			return
		}
		serveWidgetFile(w, r, assets, path.Join("embed", name), "public, max-age=31536000, immutable")
	})
}

func serveWidgetFile(w http.ResponseWriter, r *http.Request, assets fs.FS, name, cacheControl string) {
	data, err := fs.ReadFile(assets, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/javascript; charset=utf-8")
	h.Set("Cache-Control", cacheControl)
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Cross-Origin-Resource-Policy", "cross-origin")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(data) //nolint:gosec // G705: our own embedded widget build, served as JavaScript
}
