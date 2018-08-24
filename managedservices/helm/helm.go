package helm

import (
	"context"
	"fmt"
	"reflect"

	"github.com/giantswarm/apprclient"
	"github.com/giantswarm/e2e-harness/pkg/framework"
	frameworkresource "github.com/giantswarm/e2e-harness/pkg/framework/resource"
	"github.com/giantswarm/helmclient"
	"github.com/giantswarm/microerror"
	"github.com/giantswarm/micrologger"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Config struct {
	ChartConfig    ChartConfig
	ChartResources ChartResources

	ApprClient    apprclient.Interface
	HelmClient    helmclient.Interface
	HostFramework *framework.Host
	Logger        micrologger.Logger
}

type Helm struct {
	chartConfig    ChartConfig
	chartResources ChartResources

	apprClient    apprclient.Interface
	helmClient    helmclient.Interface
	hostFramework *framework.Host
	logger        micrologger.Logger
	resource      *frameworkresource.Resource
}

func New(config Config) (*Helm, error) {
	var err error

	err = validateChartConfig(config.ChartConfig)
	if err != nil {
		return nil, microerror.Mask(err)
	}
	err = validateChartResources(config.ChartResources)
	if err != nil {
		return nil, microerror.Mask(err)
	}

	if config.ApprClient == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.ApprClient must not be empty", config)
	}
	if config.HostFramework == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.HostFramework must not be empty", config)
	}
	if config.HelmClient == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.HelmClient must not be empty", config)
	}
	if config.Logger == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.Logger must not be empty", config)
	}

	var resource *frameworkresource.Resource
	{
		c := frameworkresource.ResourceConfig{
			ApprClient: config.ApprClient,
			HelmClient: config.HelmClient,
			Logger:     config.Logger,
			Namespace:  config.ChartConfig.Namespace,
		}

		resource, err = frameworkresource.New(c)
		if err != nil {
			return nil, microerror.Mask(err)
		}
	}

	h := &Helm{
		chartConfig:    config.ChartConfig,
		chartResources: config.ChartResources,

		apprClient:    config.ApprClient,
		helmClient:    config.HelmClient,
		hostFramework: config.HostFramework,
		logger:        config.Logger,
		resource:      resource,
	}

	return h, nil
}

func (h *Helm) Test(ctx context.Context) error {
	var err error

	{
		h.logger.LogCtx(ctx, "level", "debug", "message", fmt.Sprintf("installing chart %#q", h.chartConfig.ChartName))

		err = h.resource.InstallResource(h.chartConfig.ChartName, h.chartConfig.ChartValues, h.chartConfig.ChartValues)
		if err != nil {
			return microerror.Mask(err)
		}

		h.logger.LogCtx(ctx, "level", "debug", "message", fmt.Sprintf("installed chart %#q", h.chartConfig.ChartName))
	}

	{
		h.logger.LogCtx(ctx, "level", "debug", "message", "waiting for deployed status")

		err = h.resource.WaitForStatus(h.chartConfig.ReleaseName, "DEPLOYED")
		if err != nil {
			return microerror.Mask(err)
		}

		h.logger.LogCtx(ctx, "level", "debug", "message", "chart is deployed")
	}
	{
		h.logger.LogCtx(ctx, "level", "debug", "message", "checking resources")

		for _, ds := range h.chartResources.DaemonSets {
			h.logger.LogCtx(ctx, "level", "debug", "message", fmt.Sprintf("checking daemonset %#q", ds.Name))

			err = h.checkDaemonSet(ds)
			if err != nil {
				return microerror.Mask(err)
			}

			h.logger.LogCtx(ctx, "level", "debug", "message", fmt.Sprintf("daemonset %#q is correct", ds.Name))
		}

		for _, d := range h.chartResources.Deployments {
			h.logger.LogCtx(ctx, "level", "debug", "message", fmt.Sprintf("checking deployment %#q", d.Name))

			err = h.checkDeployment(d)
			if err != nil {
				return microerror.Mask(err)
			}

			h.logger.LogCtx(ctx, "level", "debug", "message", fmt.Sprintf("deployment %#q is correct", d.Name))
		}

		h.logger.LogCtx(ctx, "level", "debug", "message", "resources are correct")
	}

	{
		h.logger.LogCtx(ctx, "level", "debug", "message", "running release tests")

		err = h.helmClient.RunReleaseTest(h.chartConfig.ReleaseName)
		if err != nil {
			return microerror.Mask(err)
		}

		h.logger.LogCtx(ctx, "level", "debug", "message", "release tests passed")
	}

	return nil
}

// checkDaemonSet ensures that key properties of the daemonset are correct.
func (h *Helm) checkDaemonSet(expectedDaemonSet DaemonSet) error {
	ds, err := h.hostFramework.K8sClient().Apps().DaemonSets(expectedDaemonSet.Namespace).Get(expectedDaemonSet.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return microerror.Maskf(notFoundError, "daemonset: %#q", expectedDaemonSet.Name, err)
	} else if err != nil {
		return microerror.Mask(err)
	}

	// Check daemonset labels.
	if !reflect.DeepEqual(expectedDaemonSet.Labels, ds.ObjectMeta.Labels) {
		return microerror.Maskf(invalidLabelsError, "expected labels: %v got: %v", expectedDaemonSet.Labels, ds.ObjectMeta.Labels)
	}

	// Check selector match labels.
	if !reflect.DeepEqual(expectedDaemonSet.MatchLabels, ds.Spec.Selector.MatchLabels) {
		return microerror.Maskf(invalidLabelsError, "expected match labels: %v got: %v", expectedDaemonSet.MatchLabels, ds.Spec.Selector.MatchLabels)
	}

	// Check pod labels.
	if !reflect.DeepEqual(expectedDaemonSet.Labels, ds.Spec.Template.ObjectMeta.Labels) {
		return microerror.Maskf(invalidLabelsError, "expected pod labels: %v got: %v", expectedDaemonSet.Labels, ds.Spec.Template.ObjectMeta.Labels)
	}

	return nil
}

// checkDeployment ensures that key properties of the deployment are correct.
func (h *Helm) checkDeployment(expectedDeployment Deployment) error {
	ds, err := h.hostFramework.K8sClient().Apps().Deployments(expectedDeployment.Namespace).Get(expectedDeployment.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return microerror.Maskf(notFoundError, "deployment: %#q", expectedDeployment.Name, err)
	} else if err != nil {
		return microerror.Mask(err)
	}

	// Check replicas count.
	if int32(expectedDeployment.Replicas) != *ds.Spec.Replicas {
		return microerror.Maskf(invalidReplicasError, "expected %d replicas got: %d", expectedDeployment.Replicas, *ds.Spec.Replicas)
	}

	// Check deployment labels.
	if !reflect.DeepEqual(expectedDeployment.Labels, ds.ObjectMeta.Labels) {
		return microerror.Maskf(invalidLabelsError, "expected labels: %v got: %v", expectedDeployment.Labels, ds.ObjectMeta.Labels)
	}

	// Check selector match labels.
	if !reflect.DeepEqual(expectedDeployment.MatchLabels, ds.Spec.Selector.MatchLabels) {
		return microerror.Maskf(invalidLabelsError, "expected match labels: %v got: %v", expectedDeployment.MatchLabels, ds.Spec.Selector.MatchLabels)
	}

	// Check pod labels.
	if !reflect.DeepEqual(expectedDeployment.Labels, ds.Spec.Template.ObjectMeta.Labels) {
		return microerror.Newf("expected pod labels: %v got: %v", expectedDeployment.Labels, ds.Spec.Template.ObjectMeta.Labels)
	}

	return nil
}

func validateChartConfig(chartConfig ChartConfig) error {
	if chartConfig.ChannelName == "" {
		return microerror.Maskf(invalidConfigError, "%T.ChannelName must not be empty", chartConfig)
	}
	if chartConfig.ChartName == "" {
		return microerror.Maskf(invalidConfigError, "%T.ChartName must not be empty", chartConfig)
	}
	if chartConfig.Namespace == "" {
		return microerror.Maskf(invalidConfigError, "%T.Namespace must not be empty", chartConfig)
	}
	if chartConfig.ReleaseName == "" {
		return microerror.Maskf(invalidConfigError, "%T.ReleaseName must not be empty", chartConfig)
	}

	return nil
}

func validateChartResources(chartResources ChartResources) error {
	if len(chartResources.DaemonSets) == 0 && len(chartResources.Deployments) == 0 {
		return microerror.Maskf(invalidConfigError, "at least one daemonset or deployment must be specified")
	}

	return nil
}
