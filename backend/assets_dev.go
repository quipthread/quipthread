//go:build !production

package main

import (
	"io/fs"
	"os"
)

func dashboardSubFS() fs.FS {
	return os.DirFS("../apps/dashboard/dist")
}

// widgetSubFS reads the embed build from disk (run the embed build first).
func widgetSubFS() fs.FS {
	return os.DirFS("../embed/dist")
}

// productionBuild reports whether this binary was built with -tags production.
const productionBuild = false
