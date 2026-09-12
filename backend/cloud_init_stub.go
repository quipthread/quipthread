//go:build !cloud

package main

import (
	"fmt"

	"github.com/quipthread/quipthread/cloud"
	"github.com/quipthread/quipthread/config"
)

const cloudRuntimeBuild = false

// openCloudStore fails closed in non-cloud builds: there is no cloud
// control-plane store, so CLOUD_MODE=true can never be honored. main() treats
// any error here as fatal rather than silently running cloud mode without
// site-registry dual writes.
func openCloudStore(_ *config.Config) (cloud.Store, error) {
	return nil, fmt.Errorf("CLOUD_MODE=true requires a cloud build; rebuild with -tags cloud or unset CLOUD_MODE")
}
