//go:build !cloud

package cloud

import (
	"context"
	"errors"
)

var errNotAvailable = errors.New("cloud provisioning not available in this build")

// ProvisionSQLite is unavailable in non-cloud builds.
func ProvisionSQLite(_, _ string) (string, error) { return "", errNotAvailable }

// ProvisionTurso is unavailable in non-cloud builds.
func ProvisionTurso(_ context.Context, _, _, _, _, _, _ string) (string, error) {
	return "", errNotAvailable
}
