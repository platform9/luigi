package kubernetes

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	dhcpserverv1alpha1 "dhcpserver/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

var (
	serverLog = ctrl.Log.WithName("server")
)

// Client has info on how to connect to the kubernetes cluster
type Client struct {
	client    client.Client
	clientSet *kubernetes.Clientset
	timeout   time.Duration
}

func NewClient(timeout time.Duration) (*Client, error) {
	scheme := runtime.NewScheme()
	_ = dhcpserverv1alpha1.AddToScheme(scheme)
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	return newClient(config, scheme, timeout)
}

func newClient(config *rest.Config, schema *runtime.Scheme, timeout time.Duration) (*Client, error) {
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	mapper, err := apiutil.NewDiscoveryRESTMapper(config)
	if err != nil {
		return nil, err
	}
	c, err := client.New(config, client.Options{Scheme: schema, Mapper: mapper})
	if err != nil {
		return nil, err
	}

	return newKubernetesClient(c, clientSet, timeout), nil
}

func newKubernetesClient(k8sClient client.Client, k8sClientSet *kubernetes.Clientset, timeout time.Duration) *Client {
	return &Client{
		client:    k8sClient,
		clientSet: k8sClientSet,
		timeout:   timeout,
	}
}

func (i *Client) CreateIPAllocation(ctx context.Context, macid string, vmiref string, ip string) (*dhcpserverv1alpha1.IPAllocation, error) {

	// Does not create IPAllocation when backup is restored
	ipAllocation, err := i.GetIPAllocation(context.TODO(), ip)
	if err != nil {
		return ipAllocation, err
	}

	var alloc = map[string]dhcpserverv1alpha1.IPAllocationOwner{ip: dhcpserverv1alpha1.IPAllocationOwner{MacId: macid, VmiRef: vmiref}}

	ipAllocationCreate := &dhcpserverv1alpha1.IPAllocation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ip,
			Namespace: "default",
		},
		Spec: dhcpserverv1alpha1.IPAllocationSpec{
			Allocations: alloc,
			Range:       ip,
		},
	}

	err = i.client.Create(context.TODO(), ipAllocationCreate)
	if err != nil {
		return nil, err
	}
	return ipAllocationCreate, nil

}

func (i *Client) UpdateIPAllocation(ctx context.Context, macid string, vmiref string, ip string) (*dhcpserverv1alpha1.IPAllocation, error) {

	var alloc = map[string]dhcpserverv1alpha1.IPAllocationOwner{ip: dhcpserverv1alpha1.IPAllocationOwner{MacId: macid, VmiRef: vmiref}}

	ipAllocation, err := i.GetIPAllocation(context.TODO(), ip)
	if err != nil {
		return ipAllocation, err
	}
	serverLog.Info("Found IPAllocation " + ip + " to update")
	ipAllocation.Spec = dhcpserverv1alpha1.IPAllocationSpec{
		Allocations: alloc,
		Range:       ip,
	}

	err = i.client.Update(context.TODO(), ipAllocation)
	if err != nil {
		return nil, err
	}
	return ipAllocation, nil
}

func (i *Client) GetIPAllocation(ctx context.Context, name string) (*dhcpserverv1alpha1.IPAllocation, error) {
	ipAllocation := &dhcpserverv1alpha1.IPAllocation{}
	err := i.client.Get(context.TODO(), types.NamespacedName{
		Name:      name,
		Namespace: "default",
	}, ipAllocation)
	if err != nil {
		return nil, err
	}
	return ipAllocation, nil

}

func (i *Client) ListIPAllocations(ctx context.Context) ([]dhcpserverv1alpha1.IPAllocation, error) {
	ipAllocationList := &dhcpserverv1alpha1.IPAllocationList{}

	if err := i.client.List(context.TODO(), ipAllocationList, &client.ListOptions{}); err != nil {
		return nil, err
	}

	return ipAllocationList.Items, nil
}

func (i *Client) DeleteIPAllocation(ctx context.Context, name string) (bool, error) {
	ipAllocation, err := i.GetIPAllocation(context.TODO(), name)
	if err != nil {
		return false, err
	}

	if err := i.client.Delete(context.TODO(), ipAllocation); err != nil {
		return false, err
	}
	return true, nil
}
