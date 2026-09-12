//go:build cgo

package migration

import _ "github.com/tursodatabase/go-libsql" // Register remote libSQL for the legacy schema preflight.
