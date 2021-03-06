package loadtest

import "github.com/giantswarm/microerror"

var failedLoadTestError = &microerror.Error{
	Kind: "failedLoadTestError",
}

// IsFailedLoadTest asserts failedLoadTestError.
func IsFailedLoadTest(err error) bool {
	return microerror.Cause(err) == failedLoadTestError
}

var invalidConfigError = &microerror.Error{
	Kind: "invalidConfigError",
}

// IsInvalidConfig asserts invalidConfigError.
func IsInvalidConfig(err error) bool {
	return microerror.Cause(err) == invalidConfigError
}

var waitError = &microerror.Error{
	Kind: "waitError",
}

// IsWait asserts waitError.
func IsWait(err error) bool {
	return microerror.Cause(err) == waitError
}
