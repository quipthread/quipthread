//go:build cgo

package db

import _ "github.com/tursodatabase/go-libsql" // Register the native driver for sql.Open("libsql", ...).
