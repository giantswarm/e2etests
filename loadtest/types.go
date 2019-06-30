package loadtest

import (
	"github.com/giantswarm/helmclient"
	"k8s.io/client-go/kubernetes"
)

// Clients are configured in the e2e test.
type Clients struct {
	ControlPlaneHelmClient helmclient.Interface
	ControlPlaneK8sClient  kubernetes.Interface
}

// AppValues passes values to the loadtest-app chart.
type AppValues struct {
	Ingress AppValuesIngress `json:"ingress"`
}

type AppValuesIngress struct {
	Hosts []string `json:"hosts"`
}

// UserConfigMapValues enables autoscaling for Nginx Ingress Controller.
type UserConfigMapValues struct {
	Data UserConfigMapValuesData `yaml:"autoscaling-enabled"`
}

type UserConfigMapValuesData struct {
	AutoscalingEnabled bool `yaml:"autoscaling-enabled"`
}
