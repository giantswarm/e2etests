module github.com/giantswarm/e2etests

go 1.14

require (
	github.com/giantswarm/apiextensions v0.4.17-0.20200723160042-89aed92d1080
	github.com/giantswarm/apprclient v0.2.1-0.20200724085653-63c7eb430dcf
	github.com/giantswarm/backoff v0.2.0
	github.com/giantswarm/helmclient v1.0.6-0.20200724131413-ea0311052b6e
	github.com/giantswarm/ipam v0.2.0
	github.com/giantswarm/k8sclient/v3 v3.1.3-0.20200724085258-345602646ea8
	github.com/giantswarm/microerror v0.2.0
	github.com/giantswarm/micrologger v0.3.1
	github.com/spf13/afero v1.3.2
	k8s.io/api v0.18.5
	k8s.io/apimachinery v0.18.5
	k8s.io/client-go v0.18.5
	sigs.k8s.io/yaml v1.2.0
)
