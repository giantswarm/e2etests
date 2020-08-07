module github.com/giantswarm/e2etests/v2

go 1.14

require (
	github.com/giantswarm/apiextensions/v2 v2.0.0-20200806181735-9cd3e758e49e
	github.com/giantswarm/apprclient/v2 v2.0.0-20200807082146-02053a5c7c4d
	github.com/giantswarm/backoff v0.2.0
	github.com/giantswarm/helmclient/v2 v2.0.0-20200807083927-a727a3bb1283
	github.com/giantswarm/ipam v0.2.0
	github.com/giantswarm/k8sclient/v4 v4.0.0-20200806115259-2d3b230ace59
	github.com/giantswarm/microerror v0.2.1
	github.com/giantswarm/micrologger v0.3.1
	github.com/spf13/afero v1.3.3
	k8s.io/api v0.18.5
	k8s.io/apimachinery v0.18.5
	k8s.io/client-go v0.18.5
	sigs.k8s.io/yaml v1.2.0
)
