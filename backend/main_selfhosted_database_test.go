//go:build !cloud

package main

import (
	"errors"
	"testing"

	"github.com/quipthread/quipthread/config"
)

func TestOpenStoreRejectsRemoteSelfHostedDatabaseBeforeDriverOpen(t *testing.T) {
	for _, databaseURL := range []string{"libsql://tenant.example.test", "https://tenant.example.test"} {
		store, err := openStore(&config.Config{DatabaseURL: databaseURL})
		if store != nil || !errors.Is(err, config.ErrSelfHostedRemoteDatabase) {
			t.Fatalf("openStore(%q) = store=%v err=%v", databaseURL, store, err)
		}
	}
	store, err := openStore(&config.Config{CloudMode: true, DatabaseURL: "libsql://tenant.example.test"})
	if store != nil || !errors.Is(err, config.ErrSelfHostedRemoteDatabase) {
		t.Fatalf("non-cloud build accepted remote cloud-mode database: store=%v err=%v", store, err)
	}
}
