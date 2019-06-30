package loadtest

import (
	"context"
	"testing"

	"github.com/giantswarm/e2e-harness/pkg/framework"
	"github.com/giantswarm/helmclient/helmclienttest"
	"github.com/giantswarm/micrologger/microloggertest"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	apdexFails = `{
		"data": {
			"id": "kITVZoC0",
			"type": "test_runs",
			"attributes": {
				"basic_statistics": {
					"apdex_75": 0.50
				}
			}
		}
	}`
	apdexPasses = `{
		"data": {
			"id": "kITVZoC0",
			"type": "test_runs",
			"attributes": {
				"basic_statistics": {
					"apdex_75": 0.95
				}
			}
		}
	}`
)

func Test_LoadTest_CheckLoadTestResults(t *testing.T) {
	var err error

	testCases := []struct {
		name         string
		results      string
		errorMatcher func(error) bool
	}{
		{
			name:         "case 0: apdex passes",
			results:      apdexPasses,
			errorMatcher: nil,
		},
		{
			name:         "case 1: apdex fails",
			results:      apdexFails,
			errorMatcher: IsInvalidExecution,
		},
	}

	ctx := context.Background()

	c := Config{
		Clients: &Clients{
			ControlPlaneHelmClient: helmclienttest.New(helmclienttest.Config{}),
			ControlPlaneK8sClient:  fake.NewSimpleClientset(),
		},
		GuestFramework: &framework.Guest{},
		Logger:         microloggertest.New(),

		AuthToken:    "secret",
		ClusterID:    "eggs2",
		CommonDomain: "aws.gigantic.io",
	}

	l, err := New(c)
	if err != nil {
		t.Fatalf("error == %#v, want nil", err)
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err = l.checkLoadTestResults(ctx, []byte(tc.results))

			switch {
			case err != nil && tc.errorMatcher == nil:
				t.Fatalf("error == %#v, want nil", err)
			case err == nil && tc.errorMatcher != nil:
				t.Fatalf("error == nil, want non-nil")
			case err != nil && !tc.errorMatcher(err):
				t.Fatalf("error == %#v, want matching", err)
			}
		})
	}
}
