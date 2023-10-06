/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"

	cidr "github.com/apparentlymart/go-cidr/cidr"
	"github.com/dustin/go-humanize"
	"github.com/go-logr/logr"
	nettypes "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	kubevirtv1 "kubevirt.io/api/core/v1"
	poolv1alpha1 "kubevirt.io/api/pool/v1alpha1"

	"net"
	"os"
	"reflect"

	runtimeschema "k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"

	"strconv"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dhcpv1alpha1 "dhcp-controller/api/v1alpha1"
)

const (
	envVarDockerRegistry  = "DOCKER_REGISTRY"
	defaultDockerRegistry = ""
	dhcpServerImage       = "docker.io/platform9/dhcpserver:v1"
)

// DHCPServerReconciler reconciles a DHCPServer object
type DHCPServerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

//+kubebuilder:rbac:groups=dhcp.plumber.k8s.pf9.io,resources=dhcpservers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dhcp.plumber.k8s.pf9.io,resources=dhcpservers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dhcp.plumber.k8s.pf9.io,resources=dhcpservers/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=create;delete;deletecollection;get;list;patch;update;watch
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=create;delete;deletecollection;get;list;patch;update;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DHCPServer object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.1/pkg/reconcile
func (r *DHCPServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	//_ = log.FromContext(ctx)

	var dhcpConfigReq = dhcpv1alpha1.DHCPServerList{}
	if err := r.List(ctx, &dhcpConfigReq); err != nil {
		log.Error(err, "unable to fetch DHCPServer")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling")

	serverList := dhcpConfigReq.Items
	if len(serverList) > 0 {
		for _, server := range serverList {
			log.Infof("server %s", server.Name)
			r.writeKubeconfig(server.Namespace)
			result, err := r.ensureServer(req, server, r.backendVM(server))
			if result != nil {
				log.Error(err, "DHCP server Not ready")
				return *result, err
			}
		}
	} else {
		log.Info("serverList is Empty")
	}

	return ctrl.Result{}, nil
}

func (r *DHCPServerReconciler) genConfigMap(dhcpserver dhcpv1alpha1.DHCPServer) (error, *corev1.ConfigMap) {
	configMapData := make(map[string]string, 0)
	dnsmasqConfData := "port=0\n"

	for idx, network := range dhcpserver.Spec.Networks {

		_, ipvNet, err := net.ParseCIDR(network.NetworkCIDR.CIDRIP)
		if err != nil {
			return err, nil
		}
		firstIP, lastIP := cidr.AddressRange(ipvNet)
		if network.NetworkCIDR.RangeStartIp == "" {
			network.NetworkCIDR.RangeStartIp = cidr.Inc(firstIP).String()
		}
		if network.NetworkCIDR.RangeEndIp == "" {
			network.NetworkCIDR.RangeEndIp = cidr.Dec(lastIP).String()
		}
		RangeNetMask := net.IP(ipvNet.Mask).String()

		if network.LeaseDuration == "" {
			network.LeaseDuration = "1h"
		}

		if network.VlanID == "" {
			dnsmasqConfData = dnsmasqConfData + fmt.Sprintf("dhcp-range=%s,%s,%s,%s\n", network.NetworkCIDR.RangeStartIp, network.NetworkCIDR.RangeEndIp, RangeNetMask, network.LeaseDuration)
		} else {
			dnsmasqConfData = dnsmasqConfData + fmt.Sprintf("dhcp-range=set:%s,%s,%s,%s,%s\n", network.VlanID, network.NetworkCIDR.RangeStartIp, network.NetworkCIDR.RangeEndIp, RangeNetMask, network.LeaseDuration)
		}
		if network.NetworkCIDR.GwAddress != "" {
			if network.VlanID == "" {
				dnsmasqConfData = dnsmasqConfData + fmt.Sprintf("dhcp-option=3,%s\n", network.NetworkCIDR.GwAddress)
			} else {
				dnsmasqConfData = dnsmasqConfData + fmt.Sprintf("dhcp-option=set:%s,3,%s\n", network.VlanID, network.NetworkCIDR.GwAddress)

			}
		}
		// Interface to serve
		dnsmasqConfData = dnsmasqConfData + fmt.Sprintf("interface=eth%d\n", idx+1)
	}
	configMapData["dnsmasq.conf"] = dnsmasqConfData
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dhcpserver.Name,
			Namespace: dhcpserver.Namespace,
		},
		Data: configMapData,
	}
	return nil, configMap
}

func (r *DHCPServerReconciler) ensureServer(request reconcile.Request,
	server dhcpv1alpha1.DHCPServer,
	vmpool *unstructured.Unstructured,
) (*reconcile.Result, error) {

	found := &unstructured.Unstructured{}
	found.SetGroupVersionKind(runtimeschema.GroupVersionKind{
		Group:   "pool.kubevirt.io",
		Kind:    "VirtualMachinePool",
		Version: "v1alpha1",
	})
	err, cm := r.genConfigMap(server)
	if err != nil {
		log.Error(err, "Failed to generate ConfigMap")
		return &reconcile.Result{}, err
	}

	oldcm := &corev1.ConfigMap{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: server.Name, Namespace: server.Namespace}, oldcm)
	if err != nil && errors.IsNotFound(err) {
		log.Info(" Creating Configmap...")
		controllerutil.SetControllerReference(&server, cm, r.Scheme)
		if err := r.Create(context.TODO(), cm); err != nil {
			log.Error(err, "Failed to create new ConfigMap")
			return &reconcile.Result{}, err
		}
	} else if err != nil {
		log.Error(err, "Failed to get pre-existing ConfigMap")
	} else if err == nil {
		log.Info("Configmap exists with same name")
		if reflect.DeepEqual(cm.Data, oldcm.Data) == false {
			controllerutil.SetControllerReference(&server, cm, r.Scheme)
			if err := r.Update(context.TODO(), cm); err != nil {
				log.Error(err, "Failed to update ConfigMap")
				return &reconcile.Result{}, err
			}
			log.Info("server spec is changed, updating ConfigMap")
			// Delete the VM, will be recreated with new configmap
			err = r.Delete(context.TODO(), vmpool)
			if err != nil {
				log.Error(err, "Failed to delete the VM or there is none")
			}
			err = r.Get(context.TODO(), types.NamespacedName{Name: server.Name, Namespace: server.Namespace}, found)
			for {
				if err != nil && errors.IsNotFound(err) {
					break
				}
				err = r.Get(context.TODO(), types.NamespacedName{Name: server.Name, Namespace: server.Namespace}, found)
				time.Sleep(2 * time.Second)
			}
		} else {
			log.Info("server spec is unchanged, not updating ConfigMap")
		}
	}
	// See if VM already exists and create if it doesn't
	err = r.Get(context.TODO(), types.NamespacedName{
		Name:      server.Name,
		Namespace: server.Namespace,
	}, found)
	if err != nil && errors.IsNotFound(err) {

		// Create the VM
		err = r.Create(context.TODO(), vmpool)

		if err != nil {
			// VM failed
			return &reconcile.Result{}, err
		} else {
			// VM was successful
			return nil, nil
		}
	} else if err != nil {
		// Error that isn't due to the VM not existing
		return &reconcile.Result{}, err
	}

	return nil, nil
}

// backendVM is a code for Creating VM
func (r *DHCPServerReconciler) backendVM(v dhcpv1alpha1.DHCPServer) *unstructured.Unstructured {
	networkNames, interfaceIps := parseNetwork(v.Spec.Networks)
	t := true
	cores := resource.NewMilliQuantity(1000, resource.DecimalSI)
	memReq := resource.NewQuantity(1024*1024*1024, resource.BinarySI)
	memLimit := resource.NewQuantity(1024*1024*1024, resource.BinarySI)

	// Make the cloudinit userData
	tmpl, err := os.ReadFile("cloudinit.tmpl")
	if err != nil {
		log.Error(err, "unable to read template")
	}
	cloudinit := strings.Replace(string(tmpl), "{{.interfaceip}}", interfaceIps, 1)

	cloudinit_secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      v.Name + "-cloudinit",
			Namespace: v.Namespace,
		},
		StringData: map[string]string{"userData": cloudinit},
	}
	err = r.Delete(context.TODO(), cloudinit_secret)
	if err != nil {
		log.Error(err, "Failed to delete the cloudinit secret or there is none")
	}
	err = r.Create(context.TODO(), cloudinit_secret)
	if err != nil {
		log.Error(err, "Failed to create the cloudinit secret")
	}

	// Make the cloudinit networkData
	tmpl, err = os.ReadFile("netcloudinit.tmpl")
	netcloudinit := "    version: 2\n    ethernets:\n"
	for idx, interfaceIp := range strings.Split(interfaceIps, ",") {
		str_tmpl := string(tmpl)
		str_tmpl = strings.Replace(str_tmpl, "{{.num}}", strconv.Itoa(idx+1), 2)
		netcloudinit = netcloudinit + strings.Replace(str_tmpl, "{{.ip}}", interfaceIp, 1)
	}

	netcloudinit_secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      v.Name + "-netcloudinit",
			Namespace: v.Namespace,
		},
		StringData: map[string]string{"networkData": netcloudinit},
	}
	err = r.Delete(context.TODO(), netcloudinit_secret)
	if err != nil {
		log.Error(err, "Failed to delete the netcloudinit secret or there is none")
	}
	err = r.Create(context.TODO(), netcloudinit_secret)
	if err != nil {
		log.Error(err, "Failed to create the netcloudinit secret")
	}

	// Make the vm password secret
	vmi_pwd_secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      v.Name + "-pwd",
			Namespace: v.Namespace,
		},
		StringData: map[string]string{"root": "root"},
	}
	err = r.Delete(context.TODO(), vmi_pwd_secret)
	if err != nil {
		log.Error(err, "Failed to delete the vm password secret or there is none")
	}
	err = r.Create(context.TODO(), vmi_pwd_secret)
	if err != nil {
		log.Error(err, "Failed to create the vm password secret")
	}

	// Hugepages
	interfaceTypes := r.getInterfaceTypes(networkNames, v.Namespace)
	hugepageMemory := &kubevirtv1.Memory{}
	if isDPDK(interfaceTypes) {
		hugepageMemory = &kubevirtv1.Memory{
			Hugepages: &kubevirtv1.Hugepages{
				PageSize: getHugepageSize(),
			},
		}
	}

	// The network and interface lists
	interfaceList := []kubevirtv1.Interface{{
		Name: "default",
		InterfaceBindingMethod: kubevirtv1.InterfaceBindingMethod{
			Masquerade: &kubevirtv1.InterfaceMasquerade{},
		},
	}}
	networkList := []kubevirtv1.Network{{
		Name: "default",
		NetworkSource: kubevirtv1.NetworkSource{
			Pod: &kubevirtv1.PodNetwork{},
		},
	}}

	dpdknetworks := make(map[string]bool)
	for idx, networkName := range strings.Split(networkNames, ",") {
		var interfaceBindingMethod kubevirtv1.InterfaceBindingMethod
		switch interfaceTypes[networkName] {
		case "sriov":
			interfaceBindingMethod = kubevirtv1.InterfaceBindingMethod{
				SRIOV: &kubevirtv1.InterfaceSRIOV{},
			}
		case "ovs":
			interfaceBindingMethod = kubevirtv1.InterfaceBindingMethod{
				Bridge: &kubevirtv1.InterfaceBridge{},
			}
		case "userspace":
			dpdknetworks["interface"+strconv.Itoa(idx)] = true
			interfaceBindingMethod = kubevirtv1.InterfaceBindingMethod{
				Bridge: &kubevirtv1.InterfaceBridge{},
			}
		default:
			interfaceBindingMethod = kubevirtv1.InterfaceBindingMethod{
				Bridge: &kubevirtv1.InterfaceBridge{},
			}
		}
		interfaceList = append(interfaceList, kubevirtv1.Interface{
			Name:                   "interface" + strconv.Itoa(idx),
			InterfaceBindingMethod: interfaceBindingMethod,
		})
		networkList = append(networkList, kubevirtv1.Network{
			Name: "interface" + strconv.Itoa(idx),
			NetworkSource: kubevirtv1.NetworkSource{
				Multus: &kubevirtv1.MultusNetwork{
					NetworkName: networkName,
				},
			},
		})
	}

	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      v.Name,
			Namespace: v.Namespace,
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			Running: &t,
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					NodeSelector: v.Spec.NodeSelector,
					AccessCredentials: []kubevirtv1.AccessCredential{{
						UserPassword: &kubevirtv1.UserPasswordAccessCredential{
							Source: kubevirtv1.UserPasswordAccessCredentialSource{
								Secret: &kubevirtv1.AccessCredentialSecretSource{
									SecretName: v.Name + "-pwd",
								},
							},
							PropagationMethod: kubevirtv1.UserPasswordAccessCredentialPropagationMethod{
								QemuGuestAgent: &kubevirtv1.QemuGuestAgentUserPasswordAccessCredentialPropagation{},
							},
						},
					}},
					Domain: kubevirtv1.DomainSpec{
						Memory: hugepageMemory,
						Devices: kubevirtv1.Devices{
							Disks: []kubevirtv1.Disk{{
								Name: "containerdisk",
								DiskDevice: kubevirtv1.DiskDevice{
									Disk: &kubevirtv1.DiskTarget{
										Bus: "virtio",
									},
								},
							}, {
								Name: "cloudinitdisk",
								DiskDevice: kubevirtv1.DiskDevice{
									Disk: &kubevirtv1.DiskTarget{
										Bus: "virtio",
									},
								},
							}, {
								Name: "kc-disk",
							}, {
								Name: v.Name + "-disk",
							}},
							Interfaces: interfaceList,
						},
						Resources: kubevirtv1.ResourceRequirements{
							Limits: corev1.ResourceList{
								"memory": *memLimit,
								"cpu":    *cores,
							},
							Requests: corev1.ResourceList{
								"memory": *memReq,
								"cpu":    *cores,
							},
						},
					},
					Networks: networkList,
					Volumes: []kubevirtv1.Volume{{
						Name: v.Name + "-disk",
						VolumeSource: kubevirtv1.VolumeSource{
							ConfigMap: &kubevirtv1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: v.Name,
								},
							},
						},
					}, {
						Name: "kc-disk",
						VolumeSource: kubevirtv1.VolumeSource{
							Secret: &kubevirtv1.SecretVolumeSource{
								SecretName: "dhcp-controller-kc",
							},
						},
					}, {
						Name: "containerdisk",
						VolumeSource: kubevirtv1.VolumeSource{
							ContainerDisk: &kubevirtv1.ContainerDiskSource{
								Image: dhcpServerImage,
							},
						},
					}, {
						Name: "cloudinitdisk",
						VolumeSource: kubevirtv1.VolumeSource{
							CloudInitNoCloud: &kubevirtv1.CloudInitNoCloudSource{
								UserDataSecretRef: &corev1.LocalObjectReference{
									Name: v.Name + "-cloudinit",
								},
								NetworkDataSecretRef: &corev1.LocalObjectReference{
									Name: v.Name + "-netcloudinit",
								},
							},
						},
					}},
				},
			},
		},
	}

	replicas := int32(1)
	vmpool := &poolv1alpha1.VirtualMachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      v.Name,
			Namespace: v.Namespace,
		},
		Spec: poolv1alpha1.VirtualMachinePoolSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"select": v.Name},
			},
			VirtualMachineTemplate: &poolv1alpha1.VirtualMachineTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"select": v.Name},
				},
				Spec: vm.Spec,
			},
		},
	}

	// This section replaces "bridge" interface type with "vhostuser" for DPDK networks. Since this is not a valid type in the KubeVirt client (as its a custom change), It is manually replaced in an unstructured format.
	// If vhostuser becomes an offical interface in the future in the KubeVirt client, this section can be removed and generated VM spec can be edited to reflect it.
	// Convert to unstructured
	unstructuredvm, err := runtime.DefaultUnstructuredConverter.ToUnstructured(vmpool)
	if err != nil {
		log.Error(err, "unable to convert VM spec to unstructured")
	}
	unstructuredvm_final := &unstructured.Unstructured{Object: unstructuredvm}

	// Get the nested interface slice
	interfacelist, _, err := unstructured.NestedSlice(unstructuredvm_final.Object, "spec", "virtualMachineTemplate", "spec", "template", "spec", "domain", "devices", "interfaces")
	if err != nil {
		log.Error(err, "unable to get interfaces from unstructured VM spec")
	}
	// Replace bridge with vhostuser for dpdk interfaces
	for _, netinterface := range interfacelist {
		name, _ := netinterface.(map[string]interface{})["name"].(string)
		if dpdknetworks[name] {
			delete(netinterface.(map[string]interface{}), "bridge")
			netinterface.(map[string]interface{})["vhostuser"] = map[string]interface{}{}
		}
	}
	// Replace the old interface slice
	unstructured.SetNestedSlice(unstructuredvm_final.Object, interfacelist, "spec", "virtualMachineTemplate", "spec", "template", "spec", "domain", "devices", "interfaces")
	unstructuredvm_final.SetGroupVersionKind(runtimeschema.GroupVersionKind{
		Group:   "pool.kubevirt.io",
		Kind:    "VirtualMachinePool",
		Version: "v1alpha1",
	})

	controllerutil.SetControllerReference(&v, unstructuredvm_final, r.Scheme)
	return unstructuredvm_final
}

func (r *DHCPServerReconciler) getInterfaceTypes(networkNames string, networknamespace string) map[string]string {
	// Get type of interface by querying the NADs
	interfaceTypes := make(map[string]string)
	for _, networkName := range strings.Split(networkNames, ",") {
		nad := &nettypes.NetworkAttachmentDefinition{}
		err := r.Get(context.TODO(), types.NamespacedName{Name: networkName, Namespace: networknamespace}, nad)
		if err != nil {
			log.Error(err, "unable to get nad "+networkName)
			return interfaceTypes
		}
		nadconfig := map[string]string{}
		json.Unmarshal([]byte(nad.Spec.Config), &nadconfig)
		val, ok := nadconfig["type"]
		if ok {
			interfaceTypes[networkName] = val
		}
	}
	return interfaceTypes
}

func isDPDK(interfaceTypes map[string]string) bool {
	for _, interfacetype := range interfaceTypes {
		if interfacetype == "userspace" {
			return true
		}
	}
	return false
}

func getHugepageSize() string {
	// Get the hugepage size from meminfo and convert it into bibytes from KB
	var hugepagesize int64
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		log.Error(err, "unable to open /proc/meminfo")
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		if bytes.HasPrefix(s.Bytes(), []byte(`Hugepagesize:`)) {
			_, err = fmt.Sscanf(s.Text(), "Hugepagesize:%d", &hugepagesize)
			if err != nil {
				log.Error(err, "unable to read Hugepagesize from /proc/info")
			}
			break
		}
	}
	if err = s.Err(); err != nil {
		log.Error(err, "scanner error")
	}
	// Converting size in KB to bibytes annotation. For example 1.0 GiB -> 1Gi
	r := humanize.BigIBytes(big.NewInt(hugepagesize * 1024))
	r = strings.Replace(r, ".0 ", "", 1)
	r = strings.Replace(r, "B", "", 1)
	log.Infof("Hugepages: %+v", r)
	return r
}

// getRegistry gets the override registry value or the default one
func getRegistry(envVar, defaultValue string) string {
	registry := os.Getenv(envVar)
	if registry == "" {
		registry = defaultValue
	}
	return registry
}

func parseNetwork(networks []dhcpv1alpha1.Network) (string, string) {
	var name []string
	var ip []string
	for _, network := range networks {
		name = append(name, network.NetworkName)
		ip = append(ip, network.InterfaceIp)
	}
	return strings.Join(name, ","), strings.Join(ip, ",")
}

// Generate a kubeconfig to be used by the DHCPServer VM
func (r *DHCPServerReconciler) writeKubeconfig(namespace string) {

	restconfig, err := rest.InClusterConfig()
	if err != nil {
		log.Error(err, "unable to get clusterconfig")
	}

	cadata, err := os.ReadFile(restconfig.TLSClientConfig.CAFile)
	if err != nil {
		log.Error(err, "unable to read cafile")
	}

	clusters := make(map[string]*clientcmdapi.Cluster)
	clusters["default-cluster"] = &clientcmdapi.Cluster{
		Server:                   restconfig.Host,
		CertificateAuthorityData: cadata,
	}

	contexts := make(map[string]*clientcmdapi.Context)
	contexts["default-context"] = &clientcmdapi.Context{
		Cluster:   "default-cluster",
		Namespace: "default",
		AuthInfo:  "default",
	}

	// Create long lived token
	token := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dhcp-controller-controller-manager-token",
			Namespace:   "dhcp-controller-system",
			Annotations: map[string]string{"kubernetes.io/service-account.name": "dhcp-controller-controller-manager"},
		},
		Type: "kubernetes.io/service-account-token",
	}
	err = r.Create(context.TODO(), token)
	if err != nil {
		log.Error(err, "Failed to create the token secret or it already exists")
	}

	dhcpsecrettoken := &corev1.Secret{}

	r.Get(context.TODO(), types.NamespacedName{
		Name:      "dhcp-controller-controller-manager-token",
		Namespace: "dhcp-controller-system",
	}, dhcpsecrettoken)

	authinfos := make(map[string]*clientcmdapi.AuthInfo)
	authinfos["default"] = &clientcmdapi.AuthInfo{
		Token: string(dhcpsecrettoken.Data["token"]),
	}

	clientConfig := clientcmdapi.Config{
		Kind:           "Config",
		APIVersion:     "v1",
		Clusters:       clusters,
		Contexts:       contexts,
		CurrentContext: "default-context",
		AuthInfos:      authinfos,
	}
	clientcmd.WriteToFile(clientConfig, "/var/tmp/default.yaml")
	filebytes, err := os.ReadFile("/var/tmp/default.yaml")
	if err != nil {
		log.Error(err, "unable to read generated kubeconfig")
	}

	// Make secret kubeconfig
	kc_secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dhcp-controller-kc",
			Namespace: namespace,
		},
		StringData: map[string]string{"default.yaml": string(filebytes)},
	}
	err = r.Delete(context.TODO(), kc_secret)
	if err != nil {
		log.Error(err, "Failed to delete the kubeconfig secret or there is none")
	}
	err = r.Create(context.TODO(), kc_secret)
	if err != nil {
		log.Error(err, "Failed to create the kubeconfig secret")
	}

}

// SetupWithManager sets up the controller with the Manager.
func (r *DHCPServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dhcpv1alpha1.DHCPServer{}).
		Complete(r)
}
