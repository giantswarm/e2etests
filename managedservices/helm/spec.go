package helm

import "context"

type ChartConfig struct {
	ChannelName string
	ChartName   string
	ChartValues string
	Namespace   string
	ReleaseName string
}

type ChartResources struct {
	DaemonSets  []DaemonSet
	Deployments []Deployment
}

// DaemonSet is a daemonset to be tested.
type DaemonSet struct {
	Name        string
	Namespace   string
	Labels      map[string]string
	MatchLabels map[string]string
	Replicas    int
}

// Deployment is a deployment to be tested.
type Deployment struct {
	Name        string
	Namespace   string
	Labels      map[string]string
	MatchLabels map[string]string
	Replicas    int
}

type Interface interface {
	// Test executes the test of the helm chart.
	//
	// - Install chart.
	// - Check chart is deployed.
	// - Check resources are correct.
	// - Run helm release tests.
	//
	Test(ctx context.Context) error
}
