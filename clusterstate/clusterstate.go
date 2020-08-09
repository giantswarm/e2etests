package clusterstate

import (
	"context"
	"fmt"
	"time"

	"github.com/giantswarm/apprclient/v2"
	"github.com/giantswarm/backoff"
	"github.com/giantswarm/helmclient/v2/pkg/helmclient"
	"github.com/giantswarm/k8sclient/v4/pkg/k8sclient"
	"github.com/giantswarm/microerror"
	"github.com/giantswarm/micrologger"
	"github.com/spf13/afero"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/giantswarm/e2etests/v2/clusterstate/provider"
)

type Config struct {
	K8sClient       k8sclient.Interface
	LegacyFramework LegacyFramework
	Logger          micrologger.Logger
	Provider        provider.Interface
}

type ClusterState struct {
	k8sClient       k8sclient.Interface
	legacyFramework LegacyFramework
	logger          micrologger.Logger
	provider        provider.Interface
}

func New(config Config) (*ClusterState, error) {
	if config.K8sClient == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.K8sClient must not be empty", config)
	}
	if config.LegacyFramework == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.LegacyFramework must not be empty", config)
	}
	if config.Logger == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.Logger must not be empty", config)
	}
	if config.Provider == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.Provider must not be empty", config)
	}

	s := &ClusterState{
		k8sClient:       config.K8sClient,
		legacyFramework: config.LegacyFramework,
		logger:          config.Logger,
		provider:        config.Provider,
	}

	return s, nil
}

func (c *ClusterState) Test(ctx context.Context) error {
	var err error

	{
		c.logger.LogCtx(ctx, "level", "debug", "message", "installing e2e-app")

		err = c.InstallTestApp(ctx)
		if err != nil {
			return microerror.Mask(err)
		}

		c.logger.LogCtx(ctx, "level", "debug", "message", "installed e2e-app")
	}

	{
		c.logger.LogCtx(ctx, "level", "debug", "message", "checking test app is installed")

		err = c.CheckTestAppIsInstalled(ctx)
		if err != nil {
			return microerror.Mask(err)
		}

		c.logger.LogCtx(ctx, "level", "debug", "message", "test app is installed")
	}

	{
		c.logger.LogCtx(ctx, "level", "debug", "message", "rebooting master")

		err = c.provider.RebootMaster()
		if err != nil {
			return microerror.Mask(err)
		}

		c.logger.LogCtx(ctx, "level", "debug", "message", "rebooted master")
	}

	{
		c.logger.LogCtx(ctx, "level", "debug", "message", "waiting api to go down")

		err = c.legacyFramework.WaitForAPIDown()
		if err != nil {
			return microerror.Mask(err)
		}

		c.logger.LogCtx(ctx, "level", "debug", "message", "api is down")
	}

	{
		c.logger.LogCtx(ctx, "level", "debug", "message", "waiting for guest cluster")

		err = c.legacyFramework.WaitForGuestReady(ctx)
		if err != nil {
			return microerror.Mask(err)
		}

		c.logger.LogCtx(ctx, "level", "debug", "message", "guest cluster ready")
	}

	{
		c.logger.LogCtx(ctx, "level", "debug", "message", "checking test app is installed")

		err = c.CheckTestAppIsInstalled(ctx)
		if err != nil {
			return microerror.Mask(err)
		}

		c.logger.LogCtx(ctx, "level", "debug", "message", "test app is installed")
	}

	{
		c.logger.LogCtx(ctx, "level", "debug", "message", "replacing master node")

		err = c.provider.ReplaceMaster()
		if err != nil {
			return microerror.Mask(err)
		}

		c.logger.LogCtx(ctx, "level", "debug", "message", "master node replaced")
	}

	{
		c.logger.LogCtx(ctx, "level", "debug", "message", "waiting api to go down")

		err = c.legacyFramework.WaitForAPIDown()
		if err != nil {
			return microerror.Mask(err)
		}

		c.logger.LogCtx(ctx, "level", "debug", "message", "api is down")
	}

	{
		c.logger.LogCtx(ctx, "level", "debug", "message", "waiting for guest cluster")

		err = c.legacyFramework.WaitForGuestReady(ctx)
		if err != nil {
			return microerror.Mask(err)
		}

		c.logger.LogCtx(ctx, "level", "debug", "message", "guest cluster ready")
	}

	{
		c.logger.LogCtx(ctx, "level", "debug", "message", "checking test app is installed")

		err = c.CheckTestAppIsInstalled(ctx)
		if err != nil {
			return microerror.Mask(err)
		}

		c.logger.LogCtx(ctx, "level", "debug", "message", "test app is installed")
	}

	return nil
}

func (c *ClusterState) InstallTestApp(ctx context.Context) error {
	var err error

	var apprClient *apprclient.Client
	{
		c := apprclient.Config{
			Fs:     afero.NewOsFs(),
			Logger: c.logger,

			Address:      CNRAddress,
			Organization: CNROrganization,
		}

		apprClient, err = apprclient.New(c)
		if err != nil {
			return microerror.Mask(err)
		}
	}

	var helmClient *helmclient.Client
	{
		c := helmclient.Config{
			Logger:    c.logger,
			K8sClient: c.k8sClient,
		}

		helmClient, err = helmclient.New(c)
		if err != nil {
			return microerror.Mask(err)
		}
	}

	// Install the e2e app chart in the guest cluster.
	{
		c.logger.Log("level", "debug", "message", "installing e2e-app for testing")

		tarballPath, err := apprClient.PullChartTarball(ctx, ChartName, ChartChannel)
		if err != nil {
			return microerror.Mask(err)
		}

		opts := helmclient.InstallOptions{
			ReleaseName: ChartName,
			Wait:        true,
		}
		err = helmClient.InstallReleaseFromTarball(ctx, tarballPath, ChartNamespace, nil, opts)
		if err != nil {
			return microerror.Mask(err)
		}
	}

	return nil
}

func (c *ClusterState) CheckTestAppIsInstalled(ctx context.Context) error {
	var podCount = 2

	c.logger.Log("level", "debug", "message", fmt.Sprintf("waiting for %d pods of the e2e-app to be up", podCount))

	o := func() error {
		lo := metav1.ListOptions{
			LabelSelector: "app=e2e-app",
		}
		l, err := c.k8sClient.K8sClient().CoreV1().Pods(ChartNamespace).List(ctx, lo)
		if err != nil {
			return microerror.Mask(err)
		}
		if len(l.Items) != podCount {
			return microerror.Maskf(waitError, "want %d pods found %d", podCount, len(l.Items))
		}

		return nil
	}

	b := backoff.NewConstant(backoff.ShortMaxWait, backoff.ShortMaxInterval)
	n := func(err error, delay time.Duration) {
		c.logger.Log("level", "debug", "message", err.Error())
	}

	err := backoff.RetryNotify(o, b, n)
	if err != nil {
		return microerror.Mask(err)
	}

	c.logger.Log("level", "debug", "message", fmt.Sprintf("found %d pods of the e2e-app", podCount))

	return nil
}
