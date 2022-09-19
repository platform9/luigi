package kubernetes

import (
	"context"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	dhcpserverv1alpha1 "dhcpserver/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"time"
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

func (i *Client) CreateIPPool(ctx context.Context, isupdate bool, macid string, vmiref string, ip string) (*dhcpserverv1alpha1.IPPool, error) {

	//length, _ := net.IPMask(net.ParseIP(os.Getenv("IP_RANGE_NETMASK")).To4()).Size()

	if isupdate {
		ipPool, err := i.GetIPPool(context.TODO(), ip)
		if err != nil {
			return ipPool, nil
		}
		fmt.Println("Found IPPool " + ip + " to update")
		err = i.client.Update(context.TODO(), ipPool)
		if err != nil {
			return nil, err
		}
		return ipPool, nil
	}
	ipPool, err := i.GetIPPool(context.TODO(), ip)
	if err == nil {
		return ipPool, nil
	}

	var alloc = map[string]dhcpserverv1alpha1.IPAllocation{ip: dhcpserverv1alpha1.IPAllocation{MacId: macid, VmiRef: vmiref}}
	ipPoolCreate := &dhcpserverv1alpha1.IPPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ip,
			Namespace: "default",
		},
		Spec: dhcpserverv1alpha1.IPPoolSpec{
			Allocations: alloc,
			Range:       ip,
			//Range:       ip + "/" + strconv.Itoa(length),
		},
	}

	err = i.client.Create(context.TODO(), ipPoolCreate)
	if err != nil {
		return nil, err
	}
	return ipPoolCreate, nil

}

func (i *Client) GetIPPool(ctx context.Context, name string) (*dhcpserverv1alpha1.IPPool, error) {
	ipPool := &dhcpserverv1alpha1.IPPool{}
	err := i.client.Get(context.TODO(), types.NamespacedName{
		Name:      name,
		Namespace: "default",
	}, ipPool)
	if err != nil {
		return nil, err
	}
	return ipPool, nil

}

func (i *Client) ListIPPools(ctx context.Context) ([]dhcpserverv1alpha1.IPPool, error) {
	ipPoolList := &dhcpserverv1alpha1.IPPoolList{}

	if err := i.client.List(context.TODO(), ipPoolList, &client.ListOptions{}); err != nil {
		return nil, err
	}

	return ipPoolList.Items, nil
}

func (i *Client) DeleteIPPool(ctx context.Context, name string) (bool, error) {
	ipPool, err := i.GetIPPool(context.TODO(), name)
	if err != nil {
		return false, err
	}

	if err := i.client.Delete(context.TODO(), ipPool); err != nil {
		return false, err
	}
	return true, nil
}
