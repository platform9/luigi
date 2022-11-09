package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	kubevirtv1 "kubevirt.io/api/core/v1"

	dhcpserverv1alpha1 "dhcpserver/api/v1alpha1"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

var (
	serverLog      = ctrl.Log.WithName("server")
	RestartDnsmasq = make(chan []string)
)

// Client has info on how to connect to the kubernetes cluster
type Client struct {
	client    client.Client
	clientSet *kubernetes.Clientset
	timeout   time.Duration
}

type VMKey struct {
	name      string
	namespace string
}

func NewClient(timeout time.Duration) (*Client, error) {
	scheme := runtime.NewScheme()
	_ = dhcpserverv1alpha1.AddToScheme(scheme)
	_ = kubevirtv1.AddToScheme(scheme)
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
func (i *Client) WatchPod() {

	stopper := make(chan struct{})
	defer close(stopper)

	factory := informers.NewSharedInformerFactory(i.clientSet, 0)
	informer := factory.Core().V1().Pods().Informer()

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: func(obj interface{}) {
			var diff []string
			pod, _ := obj.(*corev1.Pod)
			if strings.Contains(pod.Name, "virt-launcher") {
				return
			}
			networkstatus := []map[string]string{}
			json.Unmarshal([]byte(pod.Annotations["k8s.v1.cni.cncf.io/network-status"]), &networkstatus)

			for _, network := range networkstatus {
				if _, ok := network["mac"]; ok {
					diff = append(diff, network["mac"])
				}
			}
			if len(diff) > 0 {
				RestartDnsmasq <- diff
			}
		},
	})

	go informer.Run(stopper)
	<-stopper
}

func (i *Client) WatchVm() {
	oldvmlist, err := i.ListVm(context.TODO())
	oldvmilist, err := i.ListVmi(context.TODO())

	if err != nil {
		serverLog.Error(err, "Could not list vm")
	}
	ticker := time.NewTicker(10 * time.Minute)
	for {
		select {
		case _ = <-ticker.C:
			newvmlist, err := i.ListVm(context.TODO())
			newvmilist, err := i.ListVmi(context.TODO())
			if err != nil {
				serverLog.Error(err, "Could not list vm")
			}
			if reflect.DeepEqual(oldvmlist, newvmlist) == false {
				m := make(map[VMKey]bool)
				var diff []string

				for _, newvm := range newvmlist {
					m[VMKey{
						name:      newvm.Name,
						namespace: newvm.Namespace,
					}] = true
				}

				for _, oldvm := range oldvmlist {
					if _, ok := m[VMKey{
						name:      oldvm.Name,
						namespace: oldvm.Namespace,
					}]; !ok {
						for _, vmi := range oldvmilist {
							if vmi.Name == oldvm.Name && vmi.Namespace == oldvm.Namespace {
								for _, netinterface := range vmi.Status.Interfaces {
									diff = append(diff, netinterface.MAC)
								}
							}
						}
					}
				}
				oldvmlist = newvmlist
				oldvmilist = newvmilist
				// fmt.Println(m, diff)
				if len(diff) > 0 {
					RestartDnsmasq <- diff
				}
			}
		}
	}
}

func (i *Client) CreateIPAllocation(ctx context.Context, epochexpiry string, macid string, vmiref string, ip string, vlanid string) (*dhcpserverv1alpha1.IPAllocation, error) {

	// Set vmiref for the ipAllocation
	vmilist, err := i.ListVmi(context.TODO())
	if err != nil {
		serverLog.Error(err, "Could not list vmi")
	}

foundvmi:
	for _, vmi := range vmilist {
		for _, netinterface := range vmi.Status.Interfaces {
			if macid == netinterface.MAC {
				vmiref = vmi.ObjectMeta.Name
				break foundvmi
			}
		}
	}
	// Does not create IPAllocation when backup is restored
	ipAllocation, err := i.GetIPAllocation(context.TODO(), ip)
	if ipAllocation != nil {
		return ipAllocation, err
	}

	var alloc = map[string]dhcpserverv1alpha1.IPAllocationOwner{ip: dhcpserverv1alpha1.IPAllocationOwner{MacAddr: macid, ObjectRef: vmiref}}
	serverLog.Info(fmt.Sprintf("Creating IPAllocation: %+v", alloc))

	ipAllocationCreate := &dhcpserverv1alpha1.IPAllocation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ip,
			Namespace: "default",
		},
		Spec: dhcpserverv1alpha1.IPAllocationSpec{
			Allocations: alloc,
			Range:       ip + "@" + vlanid,
			LeaseExpiry: epochexpiry,
		},
	}

	err = i.client.Create(context.TODO(), ipAllocationCreate)
	if err != nil {
		return nil, err
	}
	return ipAllocationCreate, nil

}

func (i *Client) UpdateIPAllocation(ctx context.Context, epochexpiry string, macid string, vmiref string, ip string, vlanid string) (*dhcpserverv1alpha1.IPAllocation, error) {

	ipAllocation, err := i.GetIPAllocation(context.TODO(), ip)
	if err != nil {
		return ipAllocation, err
	}

	var alloc = map[string]dhcpserverv1alpha1.IPAllocationOwner{ip: dhcpserverv1alpha1.IPAllocationOwner{MacAddr: macid, ObjectRef: ipAllocation.Spec.Allocations[ip].ObjectRef}}

	serverLog.Info(fmt.Sprintf("IPAllocation created: %+v", alloc))

	serverLog.Info(fmt.Sprintf("Found IPAllocation %s to update: %+v", ip, ipAllocation.Spec.Allocations))
	ipAllocation.Spec = dhcpserverv1alpha1.IPAllocationSpec{
		Allocations: alloc,
		Range:       ip + "@" + vlanid,
		LeaseExpiry: epochexpiry,
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

func (i *Client) ListVmi(ctx context.Context) ([]kubevirtv1.VirtualMachineInstance, error) {
	vmiList := &kubevirtv1.VirtualMachineInstanceList{}

	if err := i.client.List(context.TODO(), vmiList, &client.ListOptions{}); err != nil {
		return nil, err
	}

	return vmiList.Items, nil
}

func (i *Client) ListVm(ctx context.Context) ([]kubevirtv1.VirtualMachine, error) {
	vmList := &kubevirtv1.VirtualMachineList{}

	if err := i.client.List(context.TODO(), vmList, &client.ListOptions{}); err != nil {
		return nil, err
	}

	return vmList.Items, nil
}
