package cni

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	plumberv1 "github.com/platform9/luigi/yoshi/api/v1"
	calicov3 "github.com/projectcalico/api/pkg/apis/projectcalico/v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultCalicoBlockSize  = 26
	DefaultNatOutgoing      = true
	DefaultIpIpMode         = "Never"
	DefaultVXLANMode        = "Always"
	CalicoFixedIpAnnotation = "cni.projectcalico.org/ipAddrs"
	CalicoMACAnnotation     = "cni.projectcalico.org/hwAddr"
)

type CalicoProvider struct {
	log    logr.Logger
	client client.Client
}

func NewCalicoProvider(ctx context.Context, opts *CNIOpts) *CalicoProvider {
	calico := new(CalicoProvider)
	calico.client = opts.Client
	calico.log = opts.Log

	return calico
}

func (calico *CalicoProvider) IsSupported() bool {
	return true
}

func (calico *CalicoProvider) CreateNetwork(ctx context.Context, network *plumberv1.NetworkWizard) error {
	_, err := calico.GetNetwork(ctx, network.Name)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("Failed to get IPPool: %v", err)
	} else if err == nil {
		calico.log.Info("Calico IPPool already exists", "ippool", network.Name)
		return nil
	}

	ippool := calicov3.NewIPPool()
	ippool.Name = network.Name
	ippool.Spec.CIDR = *network.Spec.CIDR
	ippool.Spec.BlockSize = DefaultCalicoBlockSize
	ippool.Spec.NATOutgoing = DefaultNatOutgoing
	ippool.Spec.IPIPMode = DefaultIpIpMode
	ippool.Spec.VXLANMode = DefaultVXLANMode

	err = calico.client.Create(ctx, ippool)
	if err != nil {
		calico.log.Error(err, "Failed to create Calico IPPool")
		return err
	}

	return nil
}

// Delete the network if it exists, do not return an error if already deleted
func (calico *CalicoProvider) DeleteNetwork(ctx context.Context, name string) error {
	pool, err := calico.GetNetwork(ctx, name)
	if err != nil && !apierrors.IsNotFound(err) {
		calico.log.Info("IPPool not found for network, nothing to delete")
		return nil
	} else if err != nil {
		return fmt.Errorf("Error fetching Calico IPPool: %v", err)
	}

	err = calico.client.Delete(ctx, pool)
	if err != nil {
		return fmt.Errorf("Failed to delete Calicp IPPool %s: %v", pool.Name, err)
	}

	return nil
}

func (calico *CalicoProvider) VerifyNetwork(ctx context.Context, name string) error {
	_, err := calico.GetNetwork(ctx, name)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("IPPool not found for network")
	} else if err != nil {
		return fmt.Errorf("Error fetching Calico IPPool: %v", err)
	}
	return nil
}

func (calico *CalicoProvider) GetNetwork(ctx context.Context, name string) (*calicov3.IPPool, error) {
	ippool := &calicov3.IPPool{}
	nsm := types.NamespacedName{Name: name, Namespace: "default"}
	err := calico.client.Get(ctx, nsm, ippool)
	if err != nil {
		calico.log.Error(err, "Failed to get IPpool", "ippool", name)
		return nil, err
	}

	calico.log.Info("Got IPPool", "ippool", ippool)
	return ippool, nil
}

func (calico *CalicoProvider) ListNetworks(ctx context.Context) (*calicov3.IPPoolList, error) {
	ipPoolList := &calicov3.IPPoolList{}

	calico.client.List(ctx, ipPoolList)

	for _, pool := range ipPoolList.Items {
		calico.log.Info("Got IPPool", "pool", pool)
	}

	return ipPoolList, nil
}
