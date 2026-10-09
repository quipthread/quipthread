//go:build production

package main

import (
	"embed"
	"io/fs"
)

//go:embed all:static/dashboard
var dashboardEmbedFS embed.FS

//go:embed all:static/widget
var widgetEmbedFS embed.FS

func dashboardSubFS() fs.FS {
	sub, err := fs.Sub(dashboardEmbedFS, "static/dashboard")
	if err != nil {
		panic("embedded dashboard assets not found: " + err.Error())
	}
	return sub
}

// widgetSubFS holds the embed build: embed.js and the modules under embed/.
func widgetSubFS() fs.FS {
	sub, err := fs.Sub(widgetEmbedFS, "static/widget")
	if err != nil {
		panic("embedded widget assets not found: " + err.Error())
	}
	return sub
}

// productionBuild reports whether this binary was built with -tags production.
const productionBuild = true
