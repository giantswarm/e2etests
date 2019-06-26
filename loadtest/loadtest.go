package loadtest

import (
	"context"
	"fmt"
	"time"

	"github.com/giantswarm/backoff"
	"github.com/giantswarm/helmclient"
	"github.com/giantswarm/microerror"
	"github.com/giantswarm/micrologger"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/giantswarm/e2etests/loadtest/provider"
)

type Config struct {
	HelmClient helmclient.Interface
	K8sClient  kubernetes.Interface
	Logger     micrologger.Logger
	Provider   provider.Interface
}

type LoadTest struct {
	helmClient helmclient.Interface
	k8sClient  kubernetes.Interface
	logger     micrologger.Logger
	provider   provider.Interface
}

func New(config Config) (*LoadTest, error) {
	if config.HelmClient == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.HelmClient must not be empty", config)
	}
	if config.K8sClient == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.K8sClient must not be empty", config)
	}
	if config.Logger == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.Logger must not be empty", config)
	}
	if config.Provider == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.Provider must not be empty", config)
	}

	s := &LoadTest{
		helmClient: config.HelmClient,
		k8sClient:  config.K8sClient,
		logger:     config.Logger,
		provider:   config.Provider,
	}

	return s, nil
}

func (l *LoadTest) Test(ctx context.Context) error {
	var err error

	{
		l.logger.LogCtx(ctx, "level", "debug", "message", "installing loadtest-app")

		err = l.InstallTestApp(ctx)
		if err != nil {
			return microerror.Mask(err)
		}

		l.logger.LogCtx(ctx, "level", "debug", "message", "installed loadtest-app")
	}

	{
		l.logger.LogCtx(ctx, "level", "debug", "message", "checking loadtest-app is installed")

		err = l.CheckTestAppIsInstalled(ctx)
		if err != nil {
			return microerror.Mask(err)
		}

		l.logger.LogCtx(ctx, "level", "debug", "message", "loadtest-app is installed")
	}

	return nil
}

func (l *LoadTest) InstallTestApp(ctx context.Context) error {
	// Install the loadtest-app chart in the tenant cluster.
	{
		l.logger.Log("level", "debug", "message", "installing loadtest-app for testing")

		tarballURL := "https://giantswarm.github.com/control-plane-test-catalog/loadtest-app-0.1.0.tgz"

		tarballPath, err := l.helmClient.PullChartTarball(ctx, tarballURL)
		if err != nil {
			return microerror.Mask(err)
		}

		err = l.helmClient.InstallReleaseFromTarball(ctx, tarballPath, metav1.NamespaceDefault)
		if err != nil {
			return microerror.Mask(err)
		}
	}

	return nil
}

func (l *LoadTest) CheckTestAppIsInstalled(ctx context.Context) error {
	var podCount = 2

	l.logger.Log("level", "debug", "message", fmt.Sprintf("waiting for %d pods of the loadtest-app to be up", podCount))

	o := func() error {
		lo := metav1.ListOptions{
			LabelSelector: "app=loadtest-app",
		}
		pl, err := l.k8sClient.CoreV1().Pods(metav1.NamespaceDefault).List(lo)
		if err != nil {
			return microerror.Mask(err)
		}
		if len(pl.Items) != podCount {
			return microerror.Maskf(waitError, "want %d pods found %d", podCount, len(pl.Items))
		}

		return nil
	}

	b := backoff.NewConstant(backoff.ShortMaxWait, backoff.ShortMaxInterval)
	n := func(err error, delay time.Duration) {
		l.logger.Log("level", "debug", "message", err.Error())
	}

	err := backoff.RetryNotify(o, b, n)
	if err != nil {
		return microerror.Mask(err)
	}

	l.logger.Log("level", "debug", "message", fmt.Sprintf("found %d pods of the loadtest-app", podCount))

	return nil
}
