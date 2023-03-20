package cni

import (
	"context"
	"fmt"

	plumberv1 "github.com/platform9/luigi/yoshi/api/v1"
	calicov3 "github.com/projectcalico/api/pkg/apis/projectcalico/v3"
	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultCalicoBlockSize  = 26
	DefaultNatOutgoing      = false
	DefaultIpIpMode         = "Always"
	CalicoFixedIpAnnotation = "cni.projectcalico.org/ipAddrs"
)

type CalicoProvider struct {
	log    *zap.SugaredLogger
	client client.Client
}

func NewCalicoProvider(client client.Client) *CalicoProvider {
	calico := new(CalicoProvider)
	calico.client = client
	logger, _ := zap.NewProduction()
	calico.log = logger.Sugar()

	return calico
}

func (calico *CalicoProvider) IsSupported() bool {
	return true
}

func (calico *CalicoProvider) CreateNetwork(network *plumberv1.NetworkWizard) error {
	_, err := calico.GetNetwork(network.Name)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("Failed to get IPPool: %v", err)
	} else if err == nil {
		calico.log.Info("Calico IPPool %s already exists", network.Name)
		return nil
	}

	ippool := calicov3.NewIPPool()
	ippool.Name = network.Name
	ippool.Spec.CIDR = network.Spec.Cidr
	ippool.Spec.BlockSize = DefaultCalicoBlockSize
	ippool.Spec.NATOutgoing = DefaultNatOutgoing
	ippool.Spec.IPIPMode = DefaultIpIpMode

	err = calico.client.Create(context.TODO(), ippool)
	if err != nil {
		calico.log.Error(err, "Failed to create Calico IPPool")
		return err
	}

	return nil
}

// Delete the network if it exists, do not return an error if already deleted
func (calico *CalicoProvider) DeleteNetwork(name string) error {
	pool, err := calico.GetNetwork(name)
	if err != nil && !apierrors.IsNotFound(err) {
		calico.log.Info("IPPool not found for network, nothing to delete")
		return nil
	} else if err != nil {
		return fmt.Errorf("Error fetching Calico IPPool: %v", err)
	}

	err = calico.client.Delete(context.TODO(), pool)
	if err != nil {
		return fmt.Errorf("Failed to delete Calicp IPPool %s: %v", pool.Name, err)
	}

	return nil
}

func (calico *CalicoProvider) VerifyNetwork(name string) error {
	_, err := calico.GetNetwork(name)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("IPPool not found for network")
	} else if err != nil {
		return fmt.Errorf("Error fetching Calico IPPool: %v", err)
	}
	return nil
}

func (calico *CalicoProvider) GetNetwork(name string) (*calicov3.IPPool, error) {
	ippool := &calicov3.IPPool{}
	nsm := types.NamespacedName{Name: name, Namespace: "default"}
	err := calico.client.Get(context.TODO(), nsm, ippool)
	if err != nil {
		calico.log.Error("Failed to get IPpool %s: %v", name, err)
		return nil, err
	}

	calico.log.Info("Got IPPool: %v", ippool)
	return ippool, nil
}

func (calico *CalicoProvider) ListNetworks() (*calicov3.IPPoolList, error) {
	ipPoolList := &calicov3.IPPoolList{}

	calico.client.List(context.TODO(), ipPoolList)

	for _, pool := range ipPoolList.Items {
		calico.log.Info("Got IPPool", "pool", pool)
	}

	return ipPoolList, nil
}
