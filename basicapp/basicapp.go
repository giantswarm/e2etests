package basicapp

import (
	"context"
	"fmt"
	"reflect"

	"github.com/giantswarm/helmclient"
	"github.com/giantswarm/microerror"
	"github.com/giantswarm/micrologger"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/giantswarm/e2etests/basicapp/legacyresource"
)

type Config struct {
	Clients    Clients
	HelmClient *helmclient.Client
	Logger     micrologger.Logger

	App            Chart
	ChartResources ChartResources
}

type BasicApp struct {
	clients    Clients
	helmClient *helmclient.Client
	logger     micrologger.Logger
	resource   *legacyresource.Resource

	chart          Chart
	chartResources ChartResources
}

func New(config Config) (*BasicApp, error) {
	var err error

	if config.HelmClient == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.HelmClient must not be empty", config)
	}
	if config.Clients == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.Clients must not be empty", config)
	}
	if config.Logger == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.Logger must not be empty", config)
	}

	err = config.App.Validate()
	if err != nil {
		return nil, microerror.Mask(err)
	}
	err = config.ChartResources.Validate()
	if err != nil {
		return nil, microerror.Mask(err)
	}

	var resource *legacyresource.Resource
	{
		c := legacyresource.Config{
			HelmClient: config.HelmClient,
			Logger:     config.Logger,
			Namespace:  config.App.Namespace,
		}

		resource, err = legacyresource.New(c)
		if err != nil {
			return nil, microerror.Mask(err)
		}
	}

	ms := &BasicApp{
		clients:    config.Clients,
		helmClient: config.HelmClient,
		logger:     config.Logger,
		resource:   resource,

		chart:          config.App,
		chartResources: config.ChartResources,
	}

	return ms, nil
}

func (b *BasicApp) Test(ctx context.Context) error {
	var err error

	{
		b.logger.LogCtx(ctx, "level", "debug", "message", fmt.Sprintf("installing chart %#q", b.chart.Name))

		err = b.resource.Install(b.chart.Name, b.chart.URL, b.chart.ChartValues)
		if err != nil {
			return microerror.Mask(err)
		}

		b.logger.LogCtx(ctx, "level", "debug", "message", fmt.Sprintf("installed chart %#q", b.chart.Name))
	}

	{
		b.logger.LogCtx(ctx, "level", "debug", "message", "waiting for deployed status")

		err = b.resource.WaitForStatus(b.chart.Name, "DEPLOYED")
		if err != nil {
			return microerror.Mask(err)
		}

		b.logger.LogCtx(ctx, "level", "debug", "message", "chart is deployed")
	}
	{
		b.logger.LogCtx(ctx, "level", "debug", "message", "checking resources")

		for _, ds := range b.chartResources.DaemonSets {
			b.logger.LogCtx(ctx, "level", "debug", "message", fmt.Sprintf("checking daemonset %#q", ds.Name))

			err = b.checkDaemonSet(ds)
			if err != nil {
				return microerror.Mask(err)
			}

			b.logger.LogCtx(ctx, "level", "debug", "message", fmt.Sprintf("daemonset %#q is correct", ds.Name))
		}

		for _, d := range b.chartResources.Deployments {
			b.logger.LogCtx(ctx, "level", "debug", "message", fmt.Sprintf("checking deployment %#q", d.Name))

			err = b.checkDeployment(d)
			if err != nil {
				return microerror.Mask(err)
			}

			b.logger.LogCtx(ctx, "level", "debug", "message", fmt.Sprintf("deployment %#q is correct", d.Name))
		}

		b.logger.LogCtx(ctx, "level", "debug", "message", "resources are correct")
	}

	{
		b.logger.LogCtx(ctx, "level", "debug", "message", "running release tests")

		err = b.helmClient.RunReleaseTest(ctx, b.chart.Name)
		if err != nil {
			return microerror.Mask(err)
		}

		b.logger.LogCtx(ctx, "level", "debug", "message", "release tests passed")
	}

	return nil
}

// checkDaemonSet ensures that key properties of the daemonset are correct.
func (b *BasicApp) checkDaemonSet(expectedDaemonSet DaemonSet) error {
	ds, err := b.clients.K8sClient().Apps().DaemonSets(expectedDaemonSet.Namespace).Get(expectedDaemonSet.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return microerror.Maskf(notFoundError, "daemonset %#q", expectedDaemonSet.Name)
	} else if err != nil {
		return microerror.Mask(err)
	}

	err = b.checkLabels("daemonset labels", expectedDaemonSet.Labels, ds.ObjectMeta.Labels)
	if err != nil {
		return microerror.Mask(err)
	}

	err = b.checkLabels("daemonset matchLabels", expectedDaemonSet.MatchLabels, ds.Spec.Selector.MatchLabels)
	if err != nil {
		return microerror.Mask(err)
	}

	err = b.checkLabels("daemonset pod labels", expectedDaemonSet.Labels, ds.Spec.Template.ObjectMeta.Labels)
	if err != nil {
		return microerror.Mask(err)
	}

	return nil
}

// checkDeployment ensures that key properties of the deployment are correct.
func (b *BasicApp) checkDeployment(expectedDeployment Deployment) error {
	ds, err := b.clients.K8sClient().Apps().Deployments(expectedDeployment.Namespace).Get(expectedDeployment.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return microerror.Maskf(notFoundError, "deployment: %#q", expectedDeployment.Name)
	} else if err != nil {
		return microerror.Mask(err)
	}

	if int32(expectedDeployment.Replicas) != *ds.Spec.Replicas {
		return microerror.Maskf(invalidReplicasError, "expected %d replicas got: %d", expectedDeployment.Replicas, *ds.Spec.Replicas)
	}

	err = b.checkLabels("deployment labels", expectedDeployment.DeploymentLabels, ds.ObjectMeta.Labels)
	if err != nil {
		return microerror.Mask(err)
	}

	err = b.checkLabels("deployment matchLabels", expectedDeployment.MatchLabels, ds.Spec.Selector.MatchLabels)
	if err != nil {
		return microerror.Mask(err)
	}

	err = b.checkLabels("deployment pod labels", expectedDeployment.PodLabels, ds.Spec.Template.ObjectMeta.Labels)
	if err != nil {
		return microerror.Mask(err)
	}

	return nil
}

func (b *BasicApp) checkLabels(labelType string, expectedLabels, labels map[string]string) error {
	if !reflect.DeepEqual(expectedLabels, labels) {
		b.logger.Log("level", "debug", "message", fmt.Sprintf("expected %s: %v got: %v", labelType, expectedLabels, labels))
		return microerror.Maskf(invalidLabelsError, "%s do not match expected labels", labelType)
	}

	return nil
}
