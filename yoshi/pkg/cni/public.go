package cni

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	plumberv1 "github.com/platform9/luigi/yoshi/api/v1"
	calicov3 "github.com/projectcalico/api/pkg/apis/projectcalico/v3"
	"github.com/projectcalico/api/pkg/lib/numorstring"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	GlobalBGPConfigName = "default"
)

const MaxASNNumber uint32 = 65535

type PublicProvider struct {
	log        logr.Logger
	client     client.Client
	enableIPAM bool
}

func NewPublicProvider(ctx context.Context, opts *CNIOpts) *PublicProvider {
	public := new(PublicProvider)
	public.client = opts.Client
	public.log = opts.Log
	public.enableIPAM = false

	return public
}

func (public *PublicProvider) IsSupported() bool {
	return true
}

func (public *PublicProvider) CreateNetwork(ctx context.Context, network *plumberv1.NetworkWizard) error {
	if err := validateBGPConfig(network); err != nil {
		public.log.Error(err, "Invalid BGP Configuration")
		return err
	}

	if err := public.CreateOrUpdateBGPConfig(ctx, network); err != nil {
		return err
	}

	if err := public.CreateOrUpdateBGPPeer(ctx, network); err != nil {
		return err
	}

	return nil
}

func validateBGPConfig(network *plumberv1.NetworkWizard) error {
	if network.Spec.BGPConfig == nil {
		return fmt.Errorf("BGPConfig not provided for public network type")
	}

	if len(network.Spec.BGPConfig.RemotePeers) < 1 {
		return fmt.Errorf("No BGP Peers provided for public network")
	}

	if network.Spec.BGPConfig.RemoteASN == nil || *network.Spec.BGPConfig.RemoteASN < 0 || *network.Spec.BGPConfig.RemoteASN > MaxASNNumber {
		return fmt.Errorf("Invalid or unspecified remote ASN. Must be integer 0-65535")
	}

	if network.Spec.BGPConfig.MyASN == nil || *network.Spec.BGPConfig.MyASN < 0 || *network.Spec.BGPConfig.MyASN > MaxASNNumber {
		return fmt.Errorf("Invalid or unspecified local ASN. Must be integer 0-65535")
	}

	if network.Spec.BGPConfig.RemoteASN == network.Spec.BGPConfig.MyASN {
		return fmt.Errorf("Remote ASN cannot match local ASN")
	}

	return nil
}

func (public *PublicProvider) CreateOrUpdateBGPConfig(ctx context.Context, network *plumberv1.NetworkWizard) error {
	bgpConfig := calicov3.NewBGPConfiguration()
	bgpConfig.Name = GlobalBGPConfigName

	op, err := controllerutil.CreateOrUpdate(ctx, public.client, bgpConfig, func() error {
		myASN := numorstring.ASNumber(*network.Spec.BGPConfig.MyASN)
		bgpConfig.Spec.ASNumber = &myASN
		if network.Spec.CIDR != nil {
			externalIP := calicov3.ServiceExternalIPBlock{CIDR: *network.Spec.CIDR}
			bgpConfig.Spec.ServiceExternalIPs = append(bgpConfig.Spec.ServiceExternalIPs, externalIP)
			public.enableIPAM = true
		}

		cidrSet := make(map[calicov3.ServiceExternalIPBlock]struct{})
		for _, cidr := range bgpConfig.Spec.ServiceExternalIPs {
			cidrSet[cidr] = struct{}{}
		}
		filteredCidrs := make([]calicov3.ServiceExternalIPBlock, 0, len(cidrSet))
		for k := range cidrSet {
			filteredCidrs = append(filteredCidrs, k)
		}
		bgpConfig.Spec.ServiceExternalIPs = filteredCidrs

		return nil
	})

	if err != nil {
		public.log.Error(err, "Failed to CreateOrUpdate BGPConfig")
		return err
	}

	public.log.Info("BGPConfig success", "op", op)

	return nil
}

// Creates a cluster-wide BGP Peer, with all nodes peering
func (public *PublicProvider) CreateOrUpdateBGPPeer(ctx context.Context, network *plumberv1.NetworkWizard) error {
	for _, peer := range network.Spec.BGPConfig.RemotePeers {
		bgpPeer := calicov3.NewBGPPeer()
		// The name of BGPPeer resource is same as the IP
		bgpPeer.Name = peer

		op, err := controllerutil.CreateOrUpdate(ctx, public.client, bgpPeer, func() error {
			bgpPeer.Spec.PeerIP = peer
			bgpPeer.Spec.ASNumber = numorstring.ASNumber(*network.Spec.BGPConfig.RemoteASN)

			return nil
		})
		if err != nil {
			public.log.Error(err, "Failed to CreateOrUpdate BGPPeer")
			return err
		}

		public.log.Info("BGPPeer success", "op", op)

	}
	return nil
}

func (public *PublicProvider) DeleteNetwork(ctx context.Context, name string) error {
	// TODO: Not implemented yet
	return nil
}

func (public *PublicProvider) VerifyNetwork(ctx context.Context, name string) error {
	// TODO: For now return error so Reconcile Creates or Updates
	// Perhaps we can get rid of this? This was before using controllerutil.CreateOrUpdate
	return fmt.Errorf("dummy error to force CreateOrUpdate")
}
