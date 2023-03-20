package cni

import (
	"fmt"

	plumberv1 "github.com/platform9/luigi/yoshi/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CniProvider interface {
	IsSupported() bool
	CreateNetwork(*plumberv1.NetworkWizard) error
	DeleteNetwork(string) error
	VerifyNetwork(string) error
}

type CniOpts interface {
	SetOpts()
}

type PublicNetProvider interface {
	CreatePublic() error
	DeletePublic(string) error
}

func NewCniProvider(client client.Client, plugin string, opts *CniOpts) (CniProvider, error) {
	switch plugin {
	case "calico":
		return NewCalicoProvider(client), nil
	case "ovs":
		return nil, fmt.Errorf("Plugin not implemented yet")
	default:
		return nil, fmt.Errorf("Undefined plugin")
	}
}
