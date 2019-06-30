package loadtest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/giantswarm/apiextensions/pkg/apis/core/v1alpha1"
	"github.com/giantswarm/apprclient"
	"github.com/giantswarm/backoff"
	"github.com/giantswarm/e2e-harness/pkg/framework"
	"github.com/giantswarm/helmclient"
	"github.com/giantswarm/microerror"
	"github.com/giantswarm/micrologger"
	"github.com/spf13/afero"
	yaml "gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/helm/pkg/helm"
)

type Config struct {
	Clients        *Clients
	GuestFramework *framework.Guest
	Logger         micrologger.Logger

	AuthToken    string
	ClusterID    string
	CommonDomain string
}

type LoadTest struct {
	clients        *Clients
	guestFramework *framework.Guest
	logger         micrologger.Logger

	authToken    string
	clusterID    string
	commonDomain string
}

func New(config Config) (*LoadTest, error) {
	if config.Clients.ControlPlaneHelmClient == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.Clients.ControlPlaneHelmClient must not be empty", config)
	}
	if config.Clients.ControlPlaneK8sClient == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.Clients.ControlPlaneK8sClient must not be empty", config)
	}
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
		clients:        config.Clients,
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

		err = l.enableIngressControllerHPA(ctx)
		if err != nil {
			return microerror.Mask(err)
		}

		l.logger.LogCtx(ctx, "level", "debug", "message", "enabled HPA for Nginx Ingress Controller")
	}

	{
		l.logger.LogCtx(ctx, "level", "debug", "message", "installing loadtest app")

		err = l.installTestApp(ctx, loadTestEndpoint)
		if err != nil {
			return microerror.Mask(err)
		}

		l.logger.LogCtx(ctx, "level", "debug", "message", "installed loadtest app")
	}

	{
		l.logger.LogCtx(ctx, "level", "debug", "message", "waiting for loadtest app to be ready")

		err = l.waitForLoadTestApp(ctx)
		if err != nil {
			return microerror.Mask(err)
		}

		l.logger.LogCtx(ctx, "level", "debug", "message", "loadtest app is ready")
	}

	return nil
}

// enableIngressControllerHPA enables HPA via the user configmap and updates
// the chartconfig CR so chart-operator reconciles the config change.
func (l *LoadTest) enableIngressControllerHPA(ctx context.Context) error {
	var err error

	values := UserConfigMapValues{
		Data: UserConfigMapValuesData{
			AutoscalingEnabled: true,
		},
	}

	var data []byte

	data, err = yaml.Marshal(values)
	if err != nil {
		return microerror.Mask(err)
	}

	_, err = l.guestFramework.K8sClient().CoreV1().ConfigMaps(metav1.NamespaceSystem).Patch(UserConfigMapName, types.StrategicMergePatchType, data)
	if err != nil {
		return microerror.Mask(err)
	}

	var cr *v1alpha1.ChartConfig

	cr, err = l.guestFramework.G8sClient().CoreV1alpha1().ChartConfigs(CustomResourceNamespace).Get(CustomResourceName, metav1.GetOptions{})
	if err != nil {
		return microerror.Mask(err)
	}

	// Set dummy annotation to trigger an update event.
	annotations := cr.Annotations
	annotations["test"] = "test"
	cr.SetAnnotations(annotations)

	_, err = l.guestFramework.G8sClient().CoreV1alpha1().ChartConfigs(CustomResourceNamespace).Update(cr)
	if err != nil {
		return microerror.Mask(err)
	}

	return nil
}

// installLoadTestApp installs a chart that deploys the Stormforger test app
// in the tenant cluster as the test workload for the load test.
func (l *LoadTest) installTestApp(ctx context.Context, loadTestEndpoint string) error {
	var err error

	var jsonValues []byte
	{
		values := AppValues{
			Ingress: AppValuesIngress{
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

	var tenantHelmClient helmclient.Interface

	{
		c := helmclient.Config{
			Logger:    l.logger,
			K8sClient: l.guestFramework.K8sClient(),

			RestConfig: l.guestFramework.RestConfig(),
		}

		tenantHelmClient, err = helmclient.New(c)
		if err != nil {
			return microerror.Mask(err)
		}
	}

	{
		err = tenantHelmClient.EnsureTillerInstalled(ctx)
		if err != nil {
			return microerror.Mask(err)
		}
	}

	{
		err = l.installChart(ctx, tenantHelmClient, AppChartName, jsonValues)
		if err != nil {
			return microerror.Mask(err)
		}
	}

	return nil
}

// waitForLoadTestApp waits for all pods of the test app to be ready.
func (l *LoadTest) waitForLoadTestApp(ctx context.Context) error {
	l.logger.Log("level", "debug", "message", "waiting for loadtest-app deployment to be ready")

	o := func() error {
		lo := metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=loadtest-app",
		}
		l, err := l.guestFramework.K8sClient().AppsV1().Deployments(metav1.NamespaceDefault).List(lo)
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

	b := backoff.NewConstant(2*time.Minute, 15*time.Second)
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

// installChart is a helper method for installing helm charts.
func (l *LoadTest) installChart(ctx context.Context, helmClient helmclient.Interface, chartName string, jsonValues []byte) error {
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
