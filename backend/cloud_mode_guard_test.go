package main

import (
	"errors"
	"testing"
)

// Startup guard: CLOUD_MODE=true must fail closed whenever a cloud
// control-plane store cannot be supplied. Running cloud mode without the store
// would skip site-registry dual writes, so it must never start silently.
func TestValidateCloudStoreFailsClosed(t *testing.T) {
	t.Run("propagates open error", func(t *testing.T) {
		openErr := errors.New("open failed")
		cs, err := validateCloudStore(nil, openErr)
		if !errors.Is(err, openErr) {
			t.Fatalf("err = %v, want %v", err, openErr)
		}
		if cs != nil {
			t.Error("store returned alongside error must be nil")
		}
	})

	t.Run("rejects nil store without error", func(t *testing.T) {
		cs, err := validateCloudStore(nil, nil)
		if err == nil {
			t.Fatal("validateCloudStore(nil, nil) succeeded, want error")
		}
		if cs != nil {
			t.Error("store returned alongside error must be nil")
		}
	})
}
