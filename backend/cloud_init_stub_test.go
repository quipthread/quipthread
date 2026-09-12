//go:build !cloud

package main

import (
	"strings"
	"testing"

	"github.com/quipthread/quipthread/config"
)

// Startup guard: a non-cloud build cannot honor CLOUD_MODE=true. openCloudStore
// must fail so main() exits instead of silently running cloud mode without
// site-registry dual writes.
func TestOpenCloudStoreFailsInNonCloudBuild(t *testing.T) {
	cs, err := openCloudStore(&config.Config{CloudMode: true})
	if err == nil {
		t.Fatal("openCloudStore succeeded in non-cloud build, want error")
	}
	if cs != nil {
		t.Error("openCloudStore returned a non-nil store in non-cloud build")
	}
	if !strings.Contains(err.Error(), "cloud build") {
		t.Errorf("error %q does not mention the cloud build requirement", err)
	}
}

func TestValidateCloudStoreFailsClosedOnNilStore(t *testing.T) {
	if _, err := validateCloudStore(nil, nil); err == nil {
		t.Fatal("validateCloudStore(nil, nil) succeeded, want error")
	}
}
