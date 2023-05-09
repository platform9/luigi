package cni

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	plumberv1 "github.com/platform9/luigi/yoshi/api/v1"
	calicov3 "github.com/projectcalico/api/pkg/apis/projectcalico/v3"
	"github.com/projectcalico/api/pkg/lib/numorstring"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	GlobalBGPConfigName = "default"
	DefaultPeer1        = "169.254.255.1"
	DefaultPeer2        = "169.254.255.2"
)

const MaxASNNumber uint32 = 65535
const DefaultMyASN uint32 = 64512
const DefaultRemoteASN uint32 = 64514

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

	if network.Spec.BGPConfig.RemoteASN != nil && (*network.Spec.BGPConfig.RemoteASN < 0 || *network.Spec.BGPConfig.RemoteASN > MaxASNNumber) {
		return fmt.Errorf("Invalid or unspecified remote ASN. Must be integer 0-65535")
	}

	if network.Spec.BGPConfig.MyASN != nil && (*network.Spec.BGPConfig.MyASN < 0 || *network.Spec.BGPConfig.MyASN > MaxASNNumber) {
		return fmt.Errorf("Invalid or unspecified local ASN. Must be integer 0-65535")
	}

	if network.Spec.BGPConfig.MyASN != nil && network.Spec.BGPConfig.RemoteASN != nil {
		if network.Spec.BGPConfig.RemoteASN == network.Spec.BGPConfig.MyASN {
			return fmt.Errorf("Remote ASN cannot match local ASN")
		}
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

		filteredCidrs := filterDuplicateCidrs(bgpConfig.Spec.ServiceExternalIPs)
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

func (public *PublicProvider) AddBGPPublicIP(ctx context.Context, publicIP string) error {
	bgpConfig, err := public.GetDefaultBGPConfig(ctx)
	if err != nil {
		public.log.Error(err, "Failed to get default BGPConfig")
		return err
	}

	ipNoMask := strings.Split(publicIP, "/")
	publicIPCIDR := ipNoMask[0] + "/32"
	externalIP := calicov3.ServiceExternalIPBlock{CIDR: publicIPCIDR}

	if bgpConfig.Spec.ServiceExternalIPs == nil {
		bgpConfig.Spec.ServiceExternalIPs = make([]calicov3.ServiceExternalIPBlock, 0)
		bgpConfig.Spec.ServiceExternalIPs = append(bgpConfig.Spec.ServiceExternalIPs, externalIP)
	} else {
		bgpConfig.Spec.ServiceExternalIPs = append(bgpConfig.Spec.ServiceExternalIPs, externalIP)
		filteredCidrs := filterDuplicateCidrs(bgpConfig.Spec.ServiceExternalIPs)
		bgpConfig.Spec.ServiceExternalIPs = filteredCidrs
	}

	public.log.Info("New BGP external IPs", "externalIPs", bgpConfig.Spec.ServiceExternalIPs)

	if err := public.client.Update(ctx, bgpConfig); err != nil {
		public.log.Error(err, "Failed to update BGPConfig", "bgpConfig.Spec", bgpConfig.Spec)
		return err
	}

	return nil
}

func (public *PublicProvider) DelBGPPublicIP(ctx context.Context, publicIP string) error {
	bgpConfig, err := public.GetDefaultBGPConfig(ctx)
	if err != nil {
		public.log.Error(err, "Failed to get default BGPConfig")
		return err
	}

	publicIPCIDR := publicIP + "/32"

	for idx, externalIP := range bgpConfig.Spec.ServiceExternalIPs {
		public.log.Info("matching IPs", "publicIP", publicIP, "bgp.ExternalIP", externalIP.CIDR)
		if publicIPCIDR == externalIP.CIDR {
			public.log.Info("Removing external IP", "IP", publicIPCIDR)
			bgpConfig.Spec.ServiceExternalIPs = append(bgpConfig.Spec.ServiceExternalIPs[:idx], bgpConfig.Spec.ServiceExternalIPs[idx+1:]...)
		}
	}

	if err := public.client.Update(ctx, bgpConfig); err != nil {
		public.log.Error(err, "Failed to update BGPConfig", bgpConfig)
		return err
	}

	return nil
}

func (public *PublicProvider) GetDefaultBGPConfig(ctx context.Context) (*calicov3.BGPConfiguration, error) {
	bgpConfig := &calicov3.BGPConfiguration{}
	nsm := types.NamespacedName{Name: GlobalBGPConfigName, Namespace: "default"}
	err := public.client.Get(ctx, nsm, bgpConfig)
	if err != nil && errors.IsNotFound(err) {
		public.log.Info("No BGPConfig resource found", "BGPConfig", nsm)
		return nil, err
	} else if err != nil {
		public.log.Info("client error fetching BGPConfig", "err", err)
		return nil, err
	}

	return bgpConfig, nil
}

// Creates a cluster-wide BGP Peer, with all nodes peering
func (public *PublicProvider) CreateOrUpdateBGPPeer(ctx context.Context, network *plumberv1.NetworkWizard) error {
	for _, peer := range network.Spec.BGPConfig.RemotePeers {
		public.log.Info("Setting up peer", "peer", peer)
		bgpPeer := calicov3.NewBGPPeer()
		// The name of BGPPeer resource is same as the IP
		bgpPeer.Name = *peer.PeerIP

		op, err := controllerutil.CreateOrUpdate(ctx, public.client, bgpPeer, func() error {
			bgpPeer.Spec.PeerIP = *peer.PeerIP
			bgpPeer.Spec.ASNumber = numorstring.ASNumber(*network.Spec.BGPConfig.RemoteASN)

			if peer.ReachableBy != nil {
				public.log.Info("Peer has reachableBy:", "reachableBy", *peer.ReachableBy)
				bgpPeer.Spec.ReachableBy = *peer.ReachableBy
			}

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
	// Not sure this should even be implemented. Could be destructive to entire cluster if non-NGPC using BGP
	return nil
}

func (public *PublicProvider) VerifyNetwork(ctx context.Context, name string) error {
	// TODO: For now return error so Reconcile Creates or Updates
	// Perhaps we can get rid of this? This was before using controllerutil.CreateOrUpdate
	return fmt.Errorf("dummy error to force CreateOrUpdate")
}

func filterDuplicateCidrs(externalIPs []calicov3.ServiceExternalIPBlock) []calicov3.ServiceExternalIPBlock {
	cidrSet := make(map[calicov3.ServiceExternalIPBlock]struct{})
	for _, cidr := range externalIPs {
		cidrSet[cidr] = struct{}{}
	}
	filteredCidrs := make([]calicov3.ServiceExternalIPBlock, 0, len(cidrSet))
	for k := range cidrSet {
		filteredCidrs = append(filteredCidrs, k)
	}

	return filteredCidrs
}
