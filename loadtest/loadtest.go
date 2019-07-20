package loadtest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/giantswarm/apiextensions/pkg/apis/core/v1alpha1"
	"github.com/giantswarm/apprclient"
	"github.com/giantswarm/backoff"
	"github.com/giantswarm/e2esetup/k8s"
	"github.com/giantswarm/microerror"
	"github.com/giantswarm/micrologger"
	yaml "gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/helm/pkg/helm"
)

type Config struct {
	ApprClient apprclient.Interface
	Logger     micrologger.Logger
	TCClients  *k8s.Clients

	ClusterID            string
	CommonDomain         string
	StormForgerAuthToken string
}

type LoadTest struct {
	apprClient apprclient.Interface
	tcClients  *k8s.Clients
	logger     micrologger.Logger

	clusterID            string
	commonDomain         string
	stormForgerAuthToken string
}

func New(config Config) (*LoadTest, error) {
	if config.ApprClient == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.ApprClient must not be empty", config)
	}
	if config.Logger == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.Logger must not be empty", config)
	}
	if config.TCClients == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.TCClients must not be empty", config)
	}

	if config.ClusterID == "" {
		return nil, microerror.Maskf(invalidConfigError, "%T.ClusterID must not be empty", config)
	}
	if config.CommonDomain == "" {
		return nil, microerror.Maskf(invalidConfigError, "%T.CommonDomain must not be empty", config)
	}
	if config.StormForgerAuthToken == "" {
		return nil, microerror.Maskf(invalidConfigError, "%T.StormForgerAuthToken must not be empty", config)
	}

	s := &LoadTest{
		apprClient: config.ApprClient,
		logger:     config.Logger,
		tcClients:  config.TCClients,

		clusterID:            config.ClusterID,
		commonDomain:         config.CommonDomain,
		stormForgerAuthToken: config.StormForgerAuthToken,
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
		l.logger.LogCtx(ctx, "level", "debug", "message", "waiting for tenant cluster kubernetes API to be up")

		err = l.waitForAPIUp(ctx)
		if err != nil {
			return microerror.Mask(err)
		}

		l.logger.LogCtx(ctx, "level", "debug", "message", "waited for tenant cluster kubernetes API to be up")
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

	{
		l.logger.LogCtx(ctx, "level", "debug", "message", "enabling HPA for Nginx Ingress Controller")

		err = l.enableIngressControllerHPA(ctx)
		if err != nil {
			return microerror.Mask(err)
		}

		l.logger.LogCtx(ctx, "level", "debug", "message", "enabled HPA for Nginx Ingress Controller")
	}

	return nil
}

// enableIngressControllerHPA enables HPA via the user configmap and updates
// the chartconfig CR so chart-operator reconciles the config change.
func (l *LoadTest) enableIngressControllerHPA(ctx context.Context) error {
	var err error

	l.logger.Log("level", "debug", "message", fmt.Sprintf("waiting for %#q configmap to be created", UserConfigMapName))

	o := func() error {
		lo := metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app=%s,cluster-operator.giantswarm.io/configmap-type=user", UserConfigMapName),
		}
		l, err := l.tcClients.K8sClient().CoreV1().ConfigMaps(metav1.NamespaceDefault).List(lo)
		if err != nil {
			return microerror.Mask(err)
		}
		if len(l.Items) != 1 {
			return microerror.Maskf(waitError, "want %d configmaps found %d", 1, len(l.Items))
		}

		return nil
	}

	b := backoff.NewConstant(10*time.Minute, 15*time.Second)
	n := func(err error, delay time.Duration) {
		l.logger.Log("level", "debug", "message", err.Error())
	}

	err = backoff.RetryNotify(o, b, n)
	if err != nil {
		return microerror.Mask(err)
	}

	l.logger.Log("level", "debug", "message", fmt.Sprintf("waited for %#q configmap to be created", UserConfigMapName))

	values := map[string]interface{}{
		"autoscaling-enabled": true,
	}

	var data []byte

	data, err = yaml.Marshal(values)
	if err != nil {
		return microerror.Mask(err)
	}

	_, err = l.tcClients.K8sClient().CoreV1().ConfigMaps(metav1.NamespaceSystem).Patch(UserConfigMapName, types.StrategicMergePatchType, data)
	if err != nil {
		return microerror.Mask(err)
	}

	var cr *v1alpha1.ChartConfig

	cr, err = l.tcClients.G8sClient().CoreV1alpha1().ChartConfigs(CustomResourceNamespace).Get(CustomResourceName, metav1.GetOptions{})
	if err != nil {
		return microerror.Mask(err)
	}

	// Set dummy annotation to trigger an update event.
	annotations := cr.Annotations
	annotations["test"] = "test"
	cr.Annotations = annotations

	_, err = l.tcClients.G8sClient().CoreV1alpha1().ChartConfigs(CustomResourceNamespace).Update(cr)
	if err != nil {
		return microerror.Mask(err)
	}

	return nil
}

// installChart is a helper method for installing helm charts.
func (l *LoadTest) installChart(ctx context.Context, chartName string, jsonValues []byte) error {
	var err error
	var tarballPath string

	{
		l.logger.Log("level", "debug", "message", fmt.Sprintf("installing %#q", chartName))

		tarballPath, err = l.apprClient.PullChartTarball(ctx, chartName, ChartChannel)
		if err != nil {
			return microerror.Mask(err)
		}

		err = l.tcClients.HelmClient().InstallReleaseFromTarball(ctx, tarballPath, ChartNamespace, helm.ValueOverrides(jsonValues))
		if err != nil {
			return microerror.Mask(err)
		}

		l.logger.Log("level", "debug", "message", fmt.Sprintf("installed %#q", chartName))
	}

	return nil
}

// installLoadTestApp installs a chart that deploys the Stormforger test app
// in the tenant cluster as the test workload for the load test.
func (l *LoadTest) installTestApp(ctx context.Context, loadTestEndpoint string) error {
	var err error

	var jsonValues []byte
	{
		values := map[string]interface{}{
			"ingress": map[string]interface{}{
				"hosts": []string{
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
		err = l.tcClients.HelmClient().EnsureTillerInstalled(ctx)
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

func (l *LoadTest) waitForAPIUp(ctx context.Context) error {
	l.logger.Log("level", "debug", "message", "waiting for k8s API to be up")

	o := func() error {
		_, err := l.tcClients.K8sClient().CoreV1().Services(metav1.NamespaceDefault).Get("kubernetes", metav1.GetOptions{})
		if err != nil {
			return microerror.Maskf(waitError, err.Error())
		}

		return nil
	}
	b := backoff.NewConstant(40*time.Minute, 30*time.Second)
	n := func(err error, delay time.Duration) {
		l.logger.Log("level", "debug", "message", err.Error())
	}

	err := backoff.RetryNotify(o, b, n)
	if err != nil {
		return microerror.Mask(err)
	}

	l.logger.Log("level", "debug", "message", "k8s API is up")

	return nil
}

// waitForLoadTestApp waits for all pods of the test app to be ready.
func (l *LoadTest) waitForLoadTestApp(ctx context.Context) error {
	l.logger.Log("level", "debug", "message", "waiting for loadtest-app deployment to be ready")

	o := func() error {
		lo := metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=loadtest-app",
		}
		l, err := l.tcClients.K8sClient().AppsV1().Deployments(metav1.NamespaceDefault).List(lo)
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
