//go:build !cloud

package main

import (
	"github.com/quipthread/quipthread/cloud"
	"github.com/quipthread/quipthread/config"
)

func openCloudStore(_ *config.Config) (cloud.Store, error) {
	return nil, nil
}
