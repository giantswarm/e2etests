package loadtest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/giantswarm/apprclient"
	"github.com/giantswarm/backoff"
	"github.com/giantswarm/e2e-harness/pkg/framework"
	"github.com/giantswarm/helmclient"
	"github.com/giantswarm/microerror"
	"github.com/giantswarm/micrologger"
	"github.com/spf13/afero"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/helm/pkg/helm"
)

type Config struct {
	GuestFramework *framework.Guest
	Logger         micrologger.Logger

	AuthToken    string
	ClusterID    string
	CommonDomain string
}

type LoadTest struct {
	guestFramework *framework.Guest
	logger         micrologger.Logger

	authToken    string
	clusterID    string
	commonDomain string
}

func New(config Config) (*LoadTest, error) {
	if config.GuestFramework == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.GuestFramework must not be empty", config)
	}
	if config.Logger == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.Logger must not be empty", config)
	}

	if config.AuthToken == "" {
		return nil, microerror.Maskf(invalidConfigError, "%T.AuthToken must not be empty", config)
	}
	if config.ClusterID == "" {
		return nil, microerror.Maskf(invalidConfigError, "%T.ClusterID must not be empty", config)
	}
	if config.CommonDomain == "" {
		return nil, microerror.Maskf(invalidConfigError, "%T.CommonDomain must not be empty", config)
	}

	s := &LoadTest{
		guestFramework: config.GuestFramework,
		logger:         config.Logger,

		authToken:    config.AuthToken,
		clusterID:    config.ClusterID,
		commonDomain: config.CommonDomain,
	}

	return s, nil
}

func (l *LoadTest) Test(ctx context.Context) error {
	var err error

	var loadTestEndpoint string
	{
		loadTestEndpoint = fmt.Sprintf("loadtest-app.%s.%s", l.clusterID, l.commonDomain)

		l.logger.Log("level", "debug", "message", fmt.Sprintf("loadtest-app endpoint is %#q", loadTestEndpoint))
	}
	{
		l.logger.LogCtx(ctx, "level", "debug", "message", "enabling HPA for Nginx Ingress Controller")

		/* TODO Update user values configmap and trigger chartconfig CR update.
		 */

		l.logger.LogCtx(ctx, "level", "debug", "message", "enabled HPA for Nginx Ingress Controller")
	}

	{
		l.logger.LogCtx(ctx, "level", "debug", "message", "installing loadtest-app")

		err = l.InstallTestApp(ctx, loadTestEndpoint)
		if err != nil {
			return microerror.Mask(err)
		}

		l.logger.LogCtx(ctx, "level", "debug", "message", "installed loadtest-app")
	}

	{
		l.logger.LogCtx(ctx, "level", "debug", "message", "waiting for loadtest-app to be ready")

		err = l.WaitForLoadTestApp(ctx)
		if err != nil {
			return microerror.Mask(err)
		}

		l.logger.LogCtx(ctx, "level", "debug", "message", "loadtest-app is ready")
	}

	{
		l.logger.LogCtx(ctx, "level", "debug", "message", "starting loadtest job")

		err = l.StartLoadTestJob(ctx, loadTestEndpoint)
		if err != nil {
			return microerror.Mask(err)
		}

		l.logger.LogCtx(ctx, "level", "debug", "message", "started loadtest job")
	}

	//var results []byte

	{
		l.logger.LogCtx(ctx, "level", "debug", "message", "waiting for loadtest job to complete")

		_, err = l.WaitForLoadTestJob(ctx)
		if err != nil {
			return microerror.Mask(err)
		}

		l.logger.LogCtx(ctx, "level", "debug", "message", "loadtest job is complete")
	}

	{
		l.logger.LogCtx(ctx, "level", "debug", "message", "checking loadtest results")

		/* TODO Check results JSON.
		err = l.CheckLoadTestResults(ctx, results)
		if err != nil {
			return microerror.Mask(err)
		}
		*/

		l.logger.LogCtx(ctx, "level", "debug", "message", "checked loadtest results")
	}

	return nil
}

func (l *LoadTest) CheckLoadTestResults(ctx context.Context, podName string) error {
	return nil
}

func (l *LoadTest) InstallTestApp(ctx context.Context, loadTestEndpoint string) error {
	var err error

	var jsonValues []byte
	{
		values := LoadTestApp{
			Ingress: LoadTestAppIngress{
				Hosts: []string{
					loadTestEndpoint,
				},
			},
		}

		jsonValues, err = json.Marshal(values)
		if err != nil {
			return microerror.Mask(err)
		}
	}

	{
		err = l.installChart(ctx, AppChartName, jsonValues)
		if err != nil {
			return microerror.Mask(err)
		}
	}

	return nil
}

func (l *LoadTest) StartLoadTestJob(ctx context.Context, loadTestEndpoint string) error {
	var err error

	var jsonValues []byte
	{
		values := LoadTestValues{
			Auth: LoadTestValuesAuth{
				Token: l.authToken,
			},
			Test: LoadTestValuesTest{
				Endpoint: loadTestEndpoint,
			},
		}

		jsonValues, err = json.Marshal(values)
		if err != nil {
			return microerror.Mask(err)
		}
	}

	{
		err = l.installChart(ctx, JobChartName, jsonValues)
		if err != nil {
			return microerror.Mask(err)
		}
	}

	return nil
}

func (l *LoadTest) WaitForLoadTestApp(ctx context.Context) error {
	l.logger.Log("level", "debug", "message", "waiting for loadtest-app deployment to be ready")

	o := func() error {
		lo := metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=loadtest-app",
		}
		l, err := l.guestFramework.K8sClient().AppsV1().Deployments(LoadTestNamespace).List(lo)
		if err != nil {
			return microerror.Mask(err)
		}
		if len(l.Items) != 1 {
			return microerror.Maskf(waitError, "want %d deployments found %d", 1, len(l.Items))
		}

		deploy := l.Items[0]
		if *deploy.Spec.Replicas == deploy.Status.ReadyReplicas {
			return microerror.Maskf(waitError, "want %d ready pods found %d", deploy.Spec.Replicas, deploy.Status.ReadyReplicas)
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

	l.logger.Log("level", "debug", "message", "waited for loadtest-app deployment to be ready")

	return nil
}

func (l *LoadTest) WaitForLoadTestJob(ctx context.Context) ([]byte, error) {
	var podCount = 1
	var podName = ""

	l.logger.Log("level", "debug", "message", "waiting for loadtest job")

	o := func() error {
		lo := metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=stormforger-cli",
		}
		l, err := l.guestFramework.K8sClient().CoreV1().Pods(LoadTestNamespace).List(lo)
		if err != nil {
			return microerror.Mask(err)
		}

		if len(l.Items) == podCount {
			pod := l.Items[0]

			if pod.Status.Phase == corev1.PodSucceeded {
				podName = pod.Name

				return nil
			} else {
				return microerror.Maskf(waitError, "want %#q pod found %#q", corev1.PodSucceeded, pod.Status.Phase)
			}

		} else {
			return microerror.Maskf(waitError, "want %d pods found %d", podCount, len(l.Items))
		}

		return nil
	}

	b := backoff.NewConstant(backoff.ShortMaxWait, backoff.ShortMaxInterval)
	n := func(err error, delay time.Duration) {
		l.logger.Log("level", "debug", "message", err.Error())
	}

	err := backoff.RetryNotify(o, b, n)
	if err != nil {
		return nil, microerror.Mask(err)
	}

	l.logger.Log("level", "debug", "message", "waited for loadtest job")

	/*
		req := l.guestFramework.K8sClient().CoreV1().Pods(TestNamespace).GetLogs(podName, &corev1.PodLogOptions{})

		readCloser, err := req.Stream()
			if err != nil {
				return nil, err
			}

			out := io.Writer

			defer readCloser.Close()

			_, err := io.Copy(out, readCloser)
			if err != nil {
				return nil, err
			}
	*/

	return nil, nil
}

func (l *LoadTest) installChart(ctx context.Context, chartName string, jsonValues []byte) error {
	var err error

	var apprClient *apprclient.Client
	{
		c := apprclient.Config{
			Fs:     afero.NewOsFs(),
			Logger: l.logger,

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
			Logger:    l.logger,
			K8sClient: l.guestFramework.K8sClient(),

			RestConfig: l.guestFramework.RestConfig(),
		}

		helmClient, err = helmclient.New(c)
		if err != nil {
			return microerror.Mask(err)
		}

		err = helmClient.EnsureTillerInstalled(ctx)
		if err != nil {
			return microerror.Mask(err)
		}
	}

	// Install the chart in the tenant cluster.
	{
		l.logger.Log("level", "debug", "message", fmt.Sprintf("installing %#q", chartName))

		tarballPath, err := apprClient.PullChartTarball(ctx, chartName, ChartChannel)
		if err != nil {
			return microerror.Mask(err)
		}

		err = helmClient.InstallReleaseFromTarball(ctx, tarballPath, ChartNamespace, helm.ValueOverrides(jsonValues))
		if err != nil {
			return microerror.Mask(err)
		}

		l.logger.Log("level", "debug", "message", fmt.Sprintf("installed %#q", chartName))
	}

	return nil
}
