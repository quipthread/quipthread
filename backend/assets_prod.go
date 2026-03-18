//go:build production

package main

import (
	"embed"
	"io/fs"
)

//go:embed static/dashboard
var dashboardEmbedFS embed.FS

//go:embed static/embed.js
var embedJSBytes []byte

func dashboardSubFS() fs.FS {
	sub, err := fs.Sub(dashboardEmbedFS, "static/dashboard")
	if err != nil {
		panic("embedded dashboard assets not found: " + err.Error())
	}
	return sub
}
