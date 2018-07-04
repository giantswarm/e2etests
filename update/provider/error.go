package provider

import "github.com/giantswarm/microerror"

var invalidConfigError = microerror.New("invalid config")

// IsInvalidConfig asserts invalidConfigError.
func IsInvalidConfig(err error) bool {
	return microerror.Cause(err) == invalidConfigError
}

var versionNotFoundError = microerror.New("version not found")

// IsVersionNotFound asserts versionNotFoundError.
func IsVersionNotFound(err error) bool {
	return microerror.Cause(err) == versionNotFoundError
}
