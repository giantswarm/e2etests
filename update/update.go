package update

import (
	"context"
	"fmt"
	"time"

	"github.com/giantswarm/backoff"
	"github.com/giantswarm/microerror"
	"github.com/giantswarm/micrologger"

	"github.com/giantswarm/e2etests/update/provider"
)

type Config struct {
	Logger   micrologger.Logger
	Provider provider.Interface

	MaxWait time.Duration
}

type Update struct {
	logger   micrologger.Logger
	provider provider.Interface

	maxWait time.Duration
}

func New(config Config) (*Update, error) {
	if config.Logger == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.Logger must not be empty", config)
	}
	if config.Provider == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.Provider must not be empty", config)
	}

	if config.MaxWait == 0 {
		config.MaxWait = 60 * time.Minute
	}

	u := &Update{
		logger:   config.Logger,
		provider: config.Provider,

		maxWait: config.MaxWait,
	}

	return u, nil
}

func (u *Update) Test(ctx context.Context) error {
	var err error

	// TODO the check for the created status condition is a hack for now because
	// e2e-hareness does not yet consider the CR status. This has to be fixed and
	// then we can remove the first check here. For now we go with this to not be
	// blocked.
	//
	//     https://github.com/giantswarm/giantswarm/issues/3937
	//
	{
		u.logger.LogCtx(ctx, "level", "debug", "message", "waiting for the guest cluster to be created")

		o := func() error {
			isCreated, err := u.provider.IsCreated()
			if err != nil {
				return microerror.Mask(err)
			}
			if isCreated {
				return backoff.Permanent(microerror.Mask(alreadyCreatedError))
			}

			return microerror.Mask(notCreatedError)
		}
		b := backoff.NewConstant(u.maxWait, 5*time.Minute)
		n := backoff.NewNotifier(u.logger, ctx)

		err := backoff.RetryNotify(o, b, n)
		if IsAlreadyCreated(err) {
			// fall through
		} else if err != nil {
			return microerror.Mask(err)
		}

		u.logger.LogCtx(ctx, "level", "debug", "message", "waited for the guest cluster to be created")
	}

	var currentVersion string
	{
		u.logger.LogCtx(ctx, "level", "debug", "message", "looking for the current version bundle version")

		currentVersion, err = u.provider.CurrentVersion()
		if provider.IsVersionNotFound(err) {
			u.logger.LogCtx(ctx, "level", "debug", "message", "did not find the current version bundle version")
			u.logger.LogCtx(ctx, "level", "debug", "message", "canceling e2e test for current version")
			return nil
		} else if err != nil {
			return microerror.Mask(err)
		}

		u.logger.LogCtx(ctx, "level", "debug", "message", fmt.Sprintf("found the current version bundle version '%s'", currentVersion))
	}

	var nextVersion string
	{
		u.logger.LogCtx(ctx, "level", "debug", "message", "looking for the next version bundle version")

		nextVersion, err = u.provider.NextVersion()
		if provider.IsVersionNotFound(err) {
			u.logger.LogCtx(ctx, "level", "debug", "message", "did not find the next version bundle version")
			u.logger.LogCtx(ctx, "level", "debug", "message", "canceling e2e test for current version")
			return nil
		} else if err != nil {
			return microerror.Mask(err)
		}

		u.logger.LogCtx(ctx, "level", "debug", "message", fmt.Sprintf("found the next version bundle version '%s'", nextVersion))
	}

	{
		u.logger.LogCtx(ctx, "level", "debug", "message", "updating the guest cluster with the new version bundle version")

		err := u.provider.UpdateVersion(nextVersion)
		if err != nil {
			return microerror.Mask(err)
		}

		u.logger.LogCtx(ctx, "level", "debug", "message", "updated the guest cluster with the new version bundle version")
	}

	{
		u.logger.LogCtx(ctx, "level", "debug", "message", "waiting for the guest cluster to be updated")

		o := func() error {
			isUpdated, err := u.provider.IsUpdated()
			if err != nil {
				return microerror.Mask(err)
			}
			if isUpdated {
				return backoff.Permanent(microerror.Mask(alreadyUpdatedError))
			}

			return microerror.Mask(notUpdatedError)
		}
		b := backoff.NewConstant(u.maxWait, 5*time.Minute)
		n := backoff.NewNotifier(u.logger, ctx)

		err := backoff.RetryNotify(o, b, n)
		if IsAlreadyUpdated(err) {
			// fall through
		} else if err != nil {
			return microerror.Mask(err)
		}

		u.logger.LogCtx(ctx, "level", "debug", "message", "waited for the guest cluster to be updated")
	}

	return nil
}
