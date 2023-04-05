package cni

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	plumberv1 "github.com/platform9/luigi/yoshi/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CNIProvider interface {
	IsSupported() bool
	CreateNetwork(context.Context, *plumberv1.NetworkWizard) error
	DeleteNetwork(context.Context, string) error
	VerifyNetwork(context.Context, string) error
}

type CNIOpts struct {
	Client client.Client
	Log    logr.Logger
}

type PublicNetProvider interface {
	CreatePublic() error
	DeletePublic(string) error
}

func NewCNIProvider(ctx context.Context, plugin string, opts *CNIOpts) (CNIProvider, error) {
	switch plugin {
	case "calico":
		return NewCalicoProvider(ctx, opts), nil
	case "ovs":
		return nil, fmt.Errorf("Plugin not implemented yet")
	default:
		return nil, fmt.Errorf("Undefined plugin")
	}
}
