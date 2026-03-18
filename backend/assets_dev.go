//go:build !production

package main

import (
	"io/fs"
	"os"
)

// embedJSBytes is unused in dev mode; embed.js is served directly from disk.
var embedJSBytes []byte

func dashboardSubFS() fs.FS {
	return os.DirFS("../apps/dashboard/dist")
}
