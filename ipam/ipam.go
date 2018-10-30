package ipam

import (
	"context"
	"fmt"
	"net"

	"github.com/giantswarm/e2e-harness/pkg/framework"
	"github.com/giantswarm/e2etests/ipam/provider"
	"github.com/giantswarm/ipam"
	"github.com/giantswarm/microerror"
	"github.com/giantswarm/micrologger"
)

type Config struct {
	HostFramework *framework.Host
	Logger        micrologger.Logger
	Provider      provider.Interface

	CommonDomain    string
	HostClusterName string
}

type IPAM struct {
	hostFramework *framework.Host
	logger        micrologger.Logger
	provider      provider.Interface

	commonDomain    string
	hostClusterName string
}

func New(config Config) (*IPAM, error) {
	if config.HostFramework == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.HostFramework must not be empty", config)
	}
	if config.Logger == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.Logger must not be empty", config)
	}
	if config.Provider == nil {
		return nil, microerror.Maskf(invalidConfigError, "%T.Provider must not be empty", config)
	}
	if config.CommonDomain == "" {
		return nil, microerror.Maskf(invalidConfigError, "%T.CommonDomain must not be empty", config)
	}
	if config.HostClusterName == "" {
		return nil, microerror.Maskf(invalidConfigError, "%T.HostClusterName must not be empty", config)
	}

	i := &IPAM{
		hostFramework: config.HostFramework,
		logger:        config.Logger,
		provider:      config.Provider,

		commonDomain:    config.CommonDomain,
		hostClusterName: config.HostClusterName,
	}

	return i, nil
}

func (i *IPAM) Test(ctx context.Context) error {
	var (
		// Clusters to be created during this test.
		clusterOne   = i.hostClusterName + "-cluster0"
		clusterTwo   = i.hostClusterName + "-cluster1"
		clusterThree = i.hostClusterName + "-cluster2"
		clusterFour  = i.hostClusterName + "-cluster3"

		// allocatedSubnets[clusterName]subnetCIDRStr
		allocatedSubnets = make(map[string]string)
		// guestFrameworks[clusterName]guestFramework
		guestFrameworks = make(map[string]*framework.Guest)
	)

	defer func() {
		for _, cn := range []string{clusterOne, clusterTwo, clusterThree, clusterFour} {
			err := i.provider.DeleteCluster(cn)
			if err != nil {
				i.logger.LogCtx(ctx, "level", "error", "message", fmt.Sprintf("cluster %s deletion failed", cn), "stack", fmt.Sprintf("%#v", err))
			}
		}
	}()

	// This is a list of clusters that are created in first test phase.
	clusters := []string{clusterOne, clusterTwo, clusterThree}

	{
		i.logger.LogCtx(ctx, "level", "debug", "message", fmt.Sprintf("creating three guest clusters: %#v", clusters))

		for _, cn := range clusters {
			err := i.provider.CreateCluster(cn)
			if err != nil {
				return microerror.Mask(err)
			}
			i.logger.LogCtx(ctx, "level", "debug", "message", fmt.Sprintf("created guest cluster %s", cn))
		}
	}

	{
		i.logger.LogCtx(ctx, "level", "debug", "message", fmt.Sprintf("waiting for three guest clusters to be ready: %#v", clusters))

		for _, cn := range clusters {
			cfg := framework.GuestConfig{
				Logger: i.logger,

				ClusterID:    cn,
				CommonDomain: i.commonDomain,
			}
			guestFramework, err := framework.NewGuest(cfg)
			if err != nil {
				return microerror.Mask(err)
			}

			guestFrameworks[cn] = guestFramework
			err = guestFramework.Setup()
			if err != nil {
				return microerror.Mask(err)
			}
			i.logger.LogCtx(ctx, "level", "debug", "message", fmt.Sprintf("guest cluster %s ready", cn))
		}

		i.logger.LogCtx(ctx, "level", "debug", "message", fmt.Sprintf("all three guest clusters are ready: %#v", clusters))
	}

	{
		i.logger.LogCtx(ctx, "level", "debug", "message", "verifying that subnet allocations don't overlap")

		for _, cn := range clusters {
			awsConfig, err := i.hostFramework.AWSCluster(cn)
			if err != nil {
				return microerror.Mask(err)
			}

			i.logger.LogCtx(ctx, "level", "debug", "message", "verify that there are no duplicate subnet allocations")
			subnet := awsConfig.Status.Cluster.Network.CIDR
			otherCluster, exists := allocatedSubnets[subnet]
			if exists {
				return microerror.Maskf(alreadyExistsError, "subnet %s already exists for %s", subnet, otherCluster)
			}

			// Verify that allocated subnets don't overlap.
			for k, _ := range allocatedSubnets {
				err := verifyNoOverlap(subnet, k)
				if err != nil {
					return microerror.Mask(err)
				}
			}

			allocatedSubnets[subnet] = cn
		}
	}

	{
		i.logger.LogCtx(ctx, "level", "debug", "message", fmt.Sprintf("terminating guest cluster %s and immediately creating new guest cluster %s", clusterTwo, clusterFour))

		err := i.provider.DeleteCluster(clusterTwo)
		if err != nil {
			return microerror.Mask(err)
		}

		err = i.provider.CreateCluster(clusterFour)
		if err != nil {
			return microerror.Mask(err)
		}
	}

	{
		i.logger.LogCtx(ctx, "level", "debug", "message", fmt.Sprintf("waiting for guest cluster %s to shutdown", clusterTwo))

		guest := guestFrameworks[clusterTwo]
		err := guest.WaitForAPIDown()
		if err != nil {
			return microerror.Mask(err)
		}
		delete(guestFrameworks, clusterTwo)

		i.logger.LogCtx(ctx, "level", "debug", "message", fmt.Sprintf("guest cluster %s down", clusterTwo))
	}

	{
		i.logger.LogCtx(ctx, "level", "debug", "message", fmt.Sprintf("waiting for guest cluster %s to come up", clusterFour))

		cfg := framework.GuestConfig{
			Logger: i.logger,

			ClusterID:    clusterFour,
			CommonDomain: i.commonDomain,
		}
		guestFramework, err := framework.NewGuest(cfg)
		if err != nil {
			return microerror.Mask(err)
		}

		guestFrameworks[clusterFour] = guestFramework
		err = guestFramework.Setup()
		if err != nil {
			return microerror.Mask(err)
		}

		i.logger.LogCtx(ctx, "level", "debug", "message", fmt.Sprintf("guest cluster %s up", clusterFour))
	}

	{
		i.logger.LogCtx(ctx, "level", "debug", "message", "verify that clusterFour subnet doesn't overlap with other allocations")

		awsConfig, err := i.hostFramework.AWSCluster(clusterFour)
		if err != nil {
			return microerror.Mask(err)
		}

		subnet := awsConfig.Status.Cluster.Network.CIDR
		otherCluster, exists := allocatedSubnets[subnet]
		if exists {
			return microerror.Maskf(alreadyExistsError, "subnet %s already exists for %s", subnet, otherCluster)
		}

		for k, _ := range allocatedSubnets {
			err := verifyNoOverlap(subnet, k)
			if err != nil {
				return microerror.Mask(err)
			}
		}
	}

	return nil
}

func verifyNoOverlap(subnet1, subnet2 string) error {
	_, net1, err := net.ParseCIDR(subnet1)
	if err != nil {
		return err
	}

	_, net2, err := net.ParseCIDR(subnet2)
	if err != nil {
		return err
	}

	if ipam.Contains(*net1, *net2) {
		return microerror.Maskf(subnetsOverlapError, "subnet %s contains %s", net1, net2)
	}

	if ipam.Contains(*net2, *net1) {
		return microerror.Maskf(subnetsOverlapError, "subnet %s contains %s", net2, net1)
	}

	return nil
}
