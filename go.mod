module github.com/giantswarm/e2etests

go 1.14

require (
	github.com/giantswarm/apiextensions v0.4.14-0.20200714152258-d202c698cf21
	github.com/giantswarm/apprclient v0.2.1-0.20200714164930-8ed30555e572
	github.com/giantswarm/backoff v0.2.0
	github.com/giantswarm/helmclient v1.0.5-0.20200714164134-5926fe4dda96
	github.com/giantswarm/ipam v0.2.0
	github.com/giantswarm/k8sclient/v3 v3.1.2-0.20200714162319-da5f60c453e3
	github.com/giantswarm/microerror v0.2.0
	github.com/giantswarm/micrologger v0.3.1
	github.com/spf13/afero v1.3.1
	k8s.io/api v0.18.5
	k8s.io/apimachinery v0.18.5
	k8s.io/client-go v0.18.5
	sigs.k8s.io/yaml v1.2.0
)
