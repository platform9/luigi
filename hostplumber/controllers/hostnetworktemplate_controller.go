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
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	plumberv1 "hostplumber/api/v1"
	hoststate "hostplumber/pkg/hoststate"
	iputils "hostplumber/pkg/utils/ip"
	linkutils "hostplumber/pkg/utils/link"
	ovsutils "hostplumber/pkg/utils/ovs"
	sriovutils "hostplumber/pkg/utils/sriov"
)

var log logr.Logger

// HostNetworkTemplateReconciler reconciles a HostNetworkTemplate object
type HostNetworkTemplateReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Log       logr.Logger
	NodeName  string
	Namespace string
}

//+kubebuilder:rbac:groups=plumber.k8s.pf9.io,resources=hostnetworktemplates,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=plumber.k8s.pf9.io,resources=hostnetworktemplates/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=plumber.k8s.pf9.io,resources=hostnetworktemplates/finalizers,verbs=update
//+kubebuilder:rbac:groups=*,resources=*,verbs=*

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the HostNetworkTemplate object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.2/pkg/reconcile
func (r *HostNetworkTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	//_ := log.FromContext(ctx)
	log = r.Log.WithValues("hostconfig", req.NamespacedName)

	var hostConfigReq = plumberv1.HostNetworkTemplate{}
	if err := r.Get(ctx, req.NamespacedName, &hostConfigReq); err != nil {
		log.Error(err, "unable to fetch HostNetworkTemplate")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling: ", "HostNetworkTemplate", hostConfigReq)

	myNode := &corev1.Node{}
	nsm := types.NamespacedName{Name: r.NodeName}
	if err := r.Get(ctx, nsm, myNode); err != nil {
		log.Error(err, "Failed to get Node with name", "NodeName", r.NodeName)
		return ctrl.Result{}, err
	}

	// name of our custom finalizer
	ovsFinalizerName := "ovsFinalizer"

	ovsConfigList := hostConfigReq.Spec.OvsConfig

	// examine DeletionTimestamp to determine if object is under deletion
	if hostConfigReq.ObjectMeta.DeletionTimestamp.IsZero() {

		if len(ovsConfigList) > 0 {
			log.Info(" Reconcile triggered for create/update hostnetworktemplate")
			// The object is not being deleted, so if it does not have our finalizer,
			// then lets add the finalizer and update the object.
			if !containsString(hostConfigReq.GetFinalizers(), ovsFinalizerName) {
				controllerutil.AddFinalizer(&hostConfigReq, ovsFinalizerName)
				log.Info("Adding Finalizer for ovscleanup")

				if err := r.Update(ctx, &hostConfigReq); err != nil {
					log.Error(err, "Error Adding Finalizer")
					return ctrl.Result{}, err
				}
			}
		}
	} else {
		// The object is being deleted
		if containsString(hostConfigReq.GetFinalizers(), ovsFinalizerName) {
			if err := deleteOvsConfig(ovsConfigList); err != nil {
				// return so that it can be retried
				return ctrl.Result{}, err
			}

			// remove finalizer from the list and update it.
			controllerutil.RemoveFinalizer(&hostConfigReq, ovsFinalizerName)
			log.Info(" Removing ovscleanup Finalizer in delete ")
			if err := r.Update(ctx, &hostConfigReq); err != nil {
				log.Error(err, "removing Finalizer failed")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	selector := labels.SelectorFromSet(hostConfigReq.Spec.NodeSelector)
	if !selector.Matches(labels.Set(myNode.Labels)) {
		log.Info("Node labels don't match template selectors, skipping", "nodeSelector", selector)
		return ctrl.Result{}, nil
	} else {
		log.Info("Labels match, applying HostNetworkTemplate", "nodeSelector", selector)
	}

	sriovConfigList := hostConfigReq.Spec.SriovConfig
	if len(sriovConfigList) > 0 {
		if err := applySriovConfig(sriovConfigList); err != nil {
			log.Error(err, "Failed to apply SriovConfig")
			return ctrl.Result{}, err
		}
	} else {
		log.Info("SriovConfig is empty")
	}

	// Everything that is traditonally done under "ifconfig <ifname>" handled here
	// Alternatively newer "ip addr" and "ip link" - see https://www.redhat.com/sysadmin/ifconfig-vs-ip
	// MTUs, IPs, routes, link up/down, etc...
	ifConfigList := hostConfigReq.Spec.InterfaceConfig
	if len(ifConfigList) > 0 {
		log.Info("ifConfigList not empty")
		if err := applyInterfaceConfig(ifConfigList, hostConfigReq.Name); err != nil {
			log.Error(err, "Failed to apply interfaceConfig")
			return ctrl.Result{}, err
		}
	} else {
		log.Info("interfaceConfig is empty")
	}

	if len(ovsConfigList) > 0 {
		if err := applyOvsConfig(ovsConfigList); err != nil {
			log.Error(err, "Failed to apply OVS config")
			return ctrl.Result{}, err
		} else {
			log.Info("Successfully applied OVS config")
		}
	} else {
		log.Info("No OVS config present")
	}

	hni := hoststate.New(r.NodeName, r.Namespace, r.Client)
	hni.DiscoverHostState()

	return ctrl.Result{}, nil
}

func applyInterfaceConfig(ifConfigList []plumberv1.InterfaceConfig, templateName string) error {
	for _, ifConfig := range ifConfigList {

		if err := createVlanInterfaces(ifConfig, templateName); err != nil {
			return err
		}

		if err := configureMtu(ifConfig); err != nil {
			return err
		}

		if err := configureIPs(ifConfig); err != nil {
			return err
		}
	}
	return nil
}

func createVlanInterfaces(ifConfig plumberv1.InterfaceConfig, templateName string) error {
	var newVlanIfs []string

	ifName := *ifConfig.Name
	vlanConfig := ifConfig.Vlan
	if len(vlanConfig) > 0 {
		var vlanIfName string
		for _, vlanIf := range vlanConfig {
			vid := *vlanIf.VlanId
			if vlanIf.Name != nil {
				vlanIfName = *vlanIf.Name
			} else {
				vlanIfName = fmt.Sprintf("%s.%d", ifName, vid)
			}
			if err := linkutils.CreateVlanIf(vlanIfName, ifName, vid); err != nil {
				log.Error(err, "Failed to create vlan interface", "vlan", vlanIfName, "ifName", ifName)
				return err
			}
			newVlanIfs = append(newVlanIfs, vlanIfName)
		}
	}
	fmt.Printf("templateName = %s ifName=%s, newVlanIfs = %s\n", templateName, ifName, newVlanIfs)
	err := linkutils.ReplaceManagedVlans(templateName, ifName, newVlanIfs)
	if err != nil {
		log.Error(err, "Failed to update managed vlans config", "ifName", ifName)
		return err
	}
	return nil
}

func configureMtu(ifConfig plumberv1.InterfaceConfig) error {
	ifName := *ifConfig.Name

	if ifConfig.MTU != nil && *ifConfig.MTU >= 576 {
		log.Info("interfaceConfig MTU", "MTU", *ifConfig.MTU)
		if err := linkutils.SetMtuForPf(ifName, *ifConfig.MTU); err != nil {
			log.Error(err, "Failed to set MTU for ifName", "ifName", ifName, "MTU", *ifConfig.MTU)
			return err
		}
	}
	return nil
}

func configureIPs(ifConfig plumberv1.InterfaceConfig) error {
	ifName := *ifConfig.Name

	if ifConfig.IPv4 != nil {
		v4Config := ifConfig.IPv4
		if len(v4Config.Address) == 0 {
			log.Info("No IPv4 addresses specified... skipping...")
		} else {
			// If an address(s) is specified, first unconfigure any old ones
			// ipv4.address should reflect desired IP state
			ipv4Addrs, err := iputils.GetIpv4Cidr(ifName)
			if err != nil || len(*ipv4Addrs) == 0 {
				log.Info("Error getting IPv4 for interface", "err", err, "ifName", ifName, "ipv4Addrs", *ipv4Addrs)
			} else {
				for _, addr := range *ipv4Addrs {
					log.Info("Removing old IP", "ifName", ifName, "IPv4", addr)
					if err := iputils.DelIpv4Cidr(ifName, addr); err != nil {
						log.Error(err, "Failed to Del IP", "ifName", ifName, "IPv4", addr)
						return err
					}
				}
			}

			for _, addr := range v4Config.Address {
				log.Info("Attempting to configure IP", "ifName", ifName, "IPv4", addr)
				if err := iputils.SetIpv4Cidr(ifName, addr); err != nil {
					log.Error(err, "Failed to Add IP", "ifName", ifName, "IPv4", addr)
					return err
				}
			}
		}
	}

	if ifConfig.IPv6 != nil {
		v6Config := ifConfig.IPv6
		if len(v6Config.Address) == 0 {
			log.Info("No IPv6 addresses specified... skipping...")
		} else {
			// If an address(s) is specified, first unconfigure any old ones
			// ipv4.address should reflect desired IP state
			ipv6Addrs, err := iputils.GetIpv6Cidr(ifName)
			if err != nil || len(*ipv6Addrs) == 0 {
				log.Info("Error getting IPv4 for interface", "err", err, "ifName", ifName, "ipv6Addrs", *ipv6Addrs)
			} else {
				for _, addr := range *ipv6Addrs {
					log.Info("Removing old IP", "ifName", ifName, "IPv6", addr)
					if err := iputils.DelIpv6Cidr(ifName, addr); err != nil {
						log.Error(err, "Failed to Del IP", "ifName", ifName, "IPv6", addr)
						return err
					}
				}
			}

			for _, addr := range v6Config.Address {
				log.Info("Attempting to configure IP", "ifName", ifName, "IPv6", addr)
				if err := iputils.SetIpv6Cidr(ifName, addr); err != nil {
					log.Error(err, "Failed to set IP for ifName", "ifName", ifName, "IPv6", addr)
					return err
				}
			}
		}
	}
	return nil
}

func deleteOvsConfig(ovsConfigList []*plumberv1.OvsConfig) error {
	/*delete ovs bridge present in ovsConfigList*/

	for _, ovsConfig := range ovsConfigList {
		br := (*ovsConfig).BridgeName
		log.Info("Deleting ovs bridge ", "bridge", br)
		err := ovsutils.DeleteOvsBr(br)
		if err != nil {
			log.Error(err, "Error deleting ovs bridge", "br", br)
			return err
		}
	}
	return nil
}

func applyOvsConfig(ovsConfigList []*plumberv1.OvsConfig) error {
	for _, ovsConfig := range ovsConfigList {
		nodeInterface := (*ovsConfig).NodeInterface
		bridgeName := (*ovsConfig).BridgeName
		dpdk := (*ovsConfig).Dpdk
		var BondMode, Lacp string
                var MtuRequest int
		if (*ovsConfig).Params != nil {
                        MtuRequest = (*ovsConfig).Params.MtuRequest
                        BondMode = (*ovsConfig).Params.BondMode
                        Lacp = (*ovsConfig).Params.Lacp
                }

		var nic1, nic2 string
		interfaces := strings.Split(nodeInterface, ",")
		length := len(interfaces)
		if length > 2 {
			err := errors.New("More than 2 interfaces specified for OVS bridge")
			log.Error(err, "bridgeName", "need 1 for bridge", "or 2 for bond")
			return nil
		}

		if length == 1 && dpdk == false {
			if err := createOvsBridge(nodeInterface, bridgeName); err != nil {
				log.Error(err, "Failed to create", "OVS bridge", bridgeName)
				return err
			} else {
				log.Info("Successfully created", "OVS bridge", bridgeName)
			}
		}
		if length == 1 && dpdk == true {
			if err := createDpdkBridge(nodeInterface, bridgeName, MtuRequest); err != nil {
				log.Error(err, "Failed to create", "OVS-DPDK bridge", bridgeName)
				return err
			} else {
				log.Info("Successfully created", "OVS-DPDK bridge", bridgeName)
			}
		}

		if length == 2 {
                        nic1 = interfaces[0]
                        nic2 = interfaces[1]
                }
		if length == 2 && dpdk == false {
			if err := createOvsBond(nic1, nic2, bridgeName, MtuRequest, BondMode, Lacp); err != nil {
				log.Error(err, "Failed to create", "OVS bond for", bridgeName)
				return err
			} else {
				log.Info("Successfully created", "OVS bond for", bridgeName)
			}
		}

		if length == 2 && dpdk == true {
			if err := createOvsDpdkBond(nic1, nic2, bridgeName, MtuRequest, BondMode, Lacp); err != nil {
				log.Error(err, "Failed to create", "OVS-DPDK bond for", bridgeName)
				return err
			} else {
				log.Info("Successfully created", "OVS-DPDK bond for", bridgeName)
			}
		}
	}
	return nil
}

func createOvsBridge(nodeInterface string, bridgeName string) error {
	log.Info("Physical interface name: ", "physnet", nodeInterface)
	log.Info("Bridge interface name: ", "ovsbr", bridgeName)
	cmd := exec.Command("ovs-vsctl", "br-exists", bridgeName)
	output, err := cmd.CombinedOutput()
	add_port_to_br := false
	if err != nil {
		// if err.Error() == "exit status 2"
		exitError, ok := err.(*exec.ExitError)
		log.Info("Bridge missing", "ok", ok, "out", output, "err", exitError)
		if ok {
			exec.Command("ovs-vsctl", "add-br", bridgeName).Run()
			add_port_to_br = true
		}
	} else {
		cmd := exec.Command("ovs-vsctl", "list-ports", bridgeName)
		output, err := cmd.Output()
		if err != nil {
			return err
		}
		exists := contains(output, nodeInterface)
		if exists {
			log.Info("Bridge already has a port for this node interface")
		} else {
			add_port_to_br = true
		}
	}
	// Check if port already belongs to another bridge, remove it first if so
	if add_port_to_br {
		cmd := exec.Command("ovs-vsctl", "port-to-br", nodeInterface)
		output, err := cmd.Output()
		if err == nil {
			br := strings.TrimSuffix(string(output), "\n")
			log.Info("Interface already attached to another bridge", "ovsbr", br)
			cmd := exec.Command("ovs-vsctl", "del-port", br, nodeInterface)
			if err := cmd.Run(); err != nil {
				log.Error(err, "Failed to remove interface from current bridge")
				return err
			}
		}
		cmd = exec.Command("ovs-vsctl", "add-port", bridgeName, nodeInterface)
		if err := cmd.Run(); err != nil {
			log.Error(err, "Failed to add interface to specified bridge")
			return err
		}
		log.Info("Added node interface to ovs bridge", "ovsbr", bridgeName)

		// Move interface IPs (if any) to the corresponding OVS bridge
		ipv4Addrs, err := iputils.GetIpv4Cidr(nodeInterface)
		move_ips := false
		if err != nil {
			log.Error(err, "Error getting IPv4 address for interface", "ifName", nodeInterface)
		} else if len(*ipv4Addrs) == 0 {
			log.Info("No IPv4 address for interface", "ifName", nodeInterface)
		} else {
			log.Info("IPv4 address(es) for interface", "ifName", nodeInterface, "ip", *ipv4Addrs)
			for _, addr := range *ipv4Addrs {
				log.Info("Removing interface IP", "ifName", nodeInterface, "ip", addr)
				if err := iputils.DelIpv4Cidr(nodeInterface, addr); err != nil {
					log.Error(err, "Failed to flush IP", "ifName", nodeInterface, "ip", addr)
					return err
				}
			}
			move_ips = true
		}
		if move_ips {
			for _, addr := range *ipv4Addrs {
				log.Info("Attempting to assign IP to bridge", "ovsbr", bridgeName, "ip", addr)
				if err := iputils.SetIpv4Cidr(bridgeName, addr); err != nil {
					log.Info("Failed to assign IP to bridge", "ovsbr", bridgeName, "ip", addr)
					return err
				}
			}
		}
	}
	return nil
}

func createDpdkBridge(nodeInterface string, bridgeName string, mtuRequest int) error {
	log.Info("Physical interface name: ", "physnet", nodeInterface)
	log.Info("Bridge interface name: ", "ovsbr", bridgeName)
	portName := "dpdk-" + bridgeName
	cmd := exec.Command("ovs-vsctl", "br-exists", bridgeName)
	output, err := cmd.CombinedOutput()
	add_port_to_br := false
	if err != nil {
		// if err.Error() == "exit status 2"
		exitError, ok := err.(*exec.ExitError)
		log.Info("Bridge missing", "ok", ok, "out", output, "err", exitError)
		if ok {
			brCmd := "ovs-vsctl add-br " + bridgeName + " -- set bridge " + bridgeName + " datapath_type=netdev"
			cmd = exec.Command("bash", "-c", brCmd)
			_, err = cmd.CombinedOutput()
			if err != nil {
				return err
			}
			log.Info("Created : ", "ovsbr", bridgeName)
			add_port_to_br = true
		}
	} else {
		cmd := exec.Command("ovs-vsctl", "list-ports", bridgeName)
		output, err := cmd.Output()
		if err != nil {
			return err
		}
		exists := strings.Contains(strings.TrimSuffix(string(output), "\n"), portName)
		if exists {
			log.Info("Bridge", bridgeName, "already has a port for this node interface")
		} else {
			add_port_to_br = true
		}
	}
	// Check if port already belongs to another bridge, remove it first if so
	if add_port_to_br {
		//get the PCI address for the NIC’s and bind vfio-pci driver:
		pciAddr, err := findPciAddr(nodeInterface)
		if err != nil {
			return err
		}
		log.Info("Pci Address of", nodeInterface, pciAddr)
		bindCmd := "dpdk-devbind.py --bind=vfio-pci " + pciAddr
		cmd = exec.Command("bash", "-c", bindCmd)
		_, err = cmd.CombinedOutput()
		if err != nil {
			log.Error(err, "Error binding", "interface", nodeInterface)
			return err
		}
		addportCmd := "ovs-vsctl add-port " + bridgeName + " " + portName + " -- set Interface " + portName + " type=dpdk options:dpdk-devargs=" + pciAddr
		cmd = exec.Command("bash", "-c", addportCmd)
		_, err = cmd.CombinedOutput()
		if err != nil {
			log.Error(err, "Failed to add interface to", "bridge", bridgeName)
			return err
		}
		var setIfaceCmd string
		if mtuRequest != 0 {
			setIfaceCmd = "ovs-vsctl set Interface " + portName + " mtu_request=" + strconv.Itoa(mtuRequest)
			cmd = exec.Command("bash", "-c", setIfaceCmd)
			_, err = cmd.CombinedOutput()
			if err != nil {
				log.Error(err, "Could not set ", "mtu_request=", mtuRequest)
				return err
			}
		}
	}
	return nil
}

func createOvsBond(nic1 string, nic2 string, bridgeName string, mtuRequest int, bondMode string, lacp string) error {
	log.Info("Physical interface1 name: ", "physnet", nic1)
	log.Info("Physical interface2 name: ", "physnet", nic2)
	log.Info("Bridge interface name: ", "ovsbr", bridgeName)
	bondName := "bond-" + bridgeName
	cmd := exec.Command("ovs-vsctl", "br-exists", bridgeName)
	output, err := cmd.CombinedOutput()
	add_bond_to_br := false
	if err != nil {
		// if err.Error() == "exit status 2"
		exitError, ok := err.(*exec.ExitError)
		log.Info("Bridge missing", "ok", ok, "out", output, "err", exitError)
		if ok {
			cmd = exec.Command("ovs-vsctl", "add-br", bridgeName)
			_, err = cmd.CombinedOutput()
			if err != nil {
				return err
			}
			log.Info("Created : ", "ovsbr", bridgeName)
			add_bond_to_br = true
		}
	} else {
		cmd = exec.Command("ovs-vsctl", "list-ports", bridgeName)
		output, err = cmd.Output()
		if err != nil {
			return err
		}
		exists := strings.Contains(strings.TrimSuffix(string(output), "\n"), bondName)
		cmd = exec.Command("ovs-vsctl", "list-ifaces", bridgeName)
		output, err = cmd.Output()
		if err != nil {
			return err
		}
		if exists && strings.Contains(strings.TrimSuffix(string(output), "\n"), nic1) && strings.Contains(strings.TrimSuffix(string(output), "\n"), nic2) {
			log.Info("Bridge", bridgeName, "already has a bond added to it")
		} else {
			add_bond_to_br = true
		}

	}
	if add_bond_to_br {
		var setPortCmd, setIfaceCmd string
		bondCmd := "ovs-vsctl add-bond " + bridgeName + " " + bondName + " " + nic1 + " " + nic2
		cmd = exec.Command("bash", "-c", bondCmd)
		_, err = cmd.CombinedOutput()
		if err != nil {
			log.Error(err, "Error adding ", "OVS bond to bridge", bridgeName)
			return err
		}
		if mtuRequest != 0 {
			setIfaceCmd = "ovs-vsctl set Interface " + bridgeName + " mtu_request=" + strconv.Itoa(mtuRequest)
			cmd = exec.Command("bash", "-c", setIfaceCmd)
			_, err = cmd.CombinedOutput()
			if err != nil {
				log.Error(err, "Could not set ", "mtu_request=", mtuRequest)
				return err
			}
		}
		if bondMode != "" {
			setPortCmd = "ovs-vsctl set port bond-" + bridgeName + " bond_mode=" + bondMode
			cmd = exec.Command("bash", "-c", setPortCmd)
			_, err = cmd.CombinedOutput()
			if err != nil {
				log.Error(err, "Could not set ", "bond_mode=", bondMode)
				return err
			}
		}
		if lacp != "" {
			setPortCmd = "ovs-vsctl set port bond-" + bridgeName + " lacp=" + lacp
			cmd = exec.Command("bash", "-c", setPortCmd)
			_, err = cmd.CombinedOutput()
			if err != nil {
				log.Error(err, "Could not set lacp")
				return err
			}
		}
	}
	return nil
}

func createOvsDpdkBond(nic1 string, nic2 string, bridgeName string, mtuRequest int, bondMode string, lacp string) error {
	log.Info("Physical interface1 name: ", "physnet", nic1)
	log.Info("Physical interface2 name: ", "physnet", nic2)
	log.Info("Bridge interface name: ", "ovsbr", bridgeName)
	bondName := "dpdkbond-" + bridgeName
	cmd := exec.Command("ovs-vsctl", "br-exists", bridgeName)
	output, err := cmd.CombinedOutput()
	add_bond_to_br := false
	if err != nil {
		// if err.Error() == "exit status 2"
		exitError, ok := err.(*exec.ExitError)
		log.Info("Bridge missing", "ok", ok, "out", output, "err", exitError)
		if ok {
			brCmd := "ovs-vsctl add-br " + bridgeName + " -- set bridge " + bridgeName + " datapath_type=netdev"
			cmd = exec.Command("bash", "-c", brCmd)
			_, err = cmd.CombinedOutput()
			if err != nil {
				return err
			}
			log.Info("Created : ", "ovsbr", bridgeName)
			add_bond_to_br = true
		}
	} else {
		cmd := exec.Command("ovs-vsctl", "list-ports", bridgeName)
		output, err := cmd.Output()
		if err != nil {
			return err
		}
		exists := strings.Contains(strings.TrimSuffix(string(output), "\n"), bondName)
		if exists {
			log.Info("Bridge", bridgeName, "already has a bond added to it")
		} else {
			add_bond_to_br = true
		}
	}

	if add_bond_to_br {
		pciNic1, err := findPciAddr(nic1)
		if err != nil {
			log.Error(err, "Could not find PCI address of ", "Interface", nic1)
			return err
		}
		pciNic2, err := findPciAddr(nic2)
		if err != nil {
			log.Error(err, "Could not find PCI address of ", "Interface", nic2)
			return err
		}

		log.Info("Pci Address of", nic1, pciNic1)
		bindCmd := "dpdk-devbind.py --bind=vfio-pci " + pciNic1
		cmd = exec.Command("bash", "-c", bindCmd)
		_, err = cmd.CombinedOutput()
		if err != nil {
			log.Error(err, "Error binding", "interface", nic1)
			return err
		}
		log.Info("Pci Address of", nic2, pciNic2)
		bindCmd = "dpdk-devbind.py --bind=vfio-pci " + pciNic2
		cmd = exec.Command("bash", "-c", bindCmd)
		_, err = cmd.CombinedOutput()
		if err != nil {
			log.Error(err, "Error binding", "interface", nic2)
			return err
		}

		var bondCmd, setIfaceCmd, setPortCmd string
		bondCmd = "ovs-vsctl add-bond " + bridgeName + " " + bondName + " " + bridgeName + "-dpdk0 " + bridgeName + "-dpdk1" + " -- set Interface " + bridgeName + "-dpdk0 type=dpdk options:dpdk-devargs=" + pciNic1 + " -- set Interface " + bridgeName + "-dpdk1 type=dpdk options:dpdk-devargs=" + pciNic2
		cmd = exec.Command("bash", "-c", bondCmd)
		_, err = cmd.CombinedOutput()
		if err != nil {
			log.Error(err, "Error adding ", "DPDK bond to bridge", bridgeName)
			return err
		}
		if mtuRequest != 0 {
			setIfaceCmd = "ovs-vsctl set Interface " + bridgeName + "  mtu_request=" + strconv.Itoa(mtuRequest)
			cmd = exec.Command("bash", "-c", setIfaceCmd)
			_, err := cmd.CombinedOutput()
			if err != nil {
				log.Error(err, "Could not set ", "mtu_request=", mtuRequest)
				return err
			}
		}
		if bondMode != "" {
			setPortCmd = "ovs-vsctl set port dpdkbond-" + bridgeName + " bond_mode=" + bondMode
			cmd = exec.Command("bash", "-c", setPortCmd)
			_, err := cmd.CombinedOutput()
			if err != nil {
				log.Error(err, "Could not set ", "bond_mode=", bondMode)
				return err
			}
		}
		if lacp != "" {
			setPortCmd = "ovs-vsctl set port dpdkbond-" + bridgeName + " lacp=" + lacp
			cmd = exec.Command("bash", "-c", setPortCmd)
			_, err := cmd.CombinedOutput()
			if err != nil {
				log.Error(err, "Could not set lacp")
				return err
			}
		}
	}
	return nil
}

func applySriovConfig(sriovConfigList []plumberv1.SriovConfig) error {
	for _, sriovConfig := range sriovConfigList {
		var pfName string
		var pfList []string
		var err error
		if sriovConfig.PfName != nil {
			log.Info("Configuring via PF:", "PfName", *sriovConfig.PfName)
			pfName = *sriovConfig.PfName
			pfList = append(pfList, pfName)
		} else if sriovConfig.PciAddr != nil {
			log.Info("Configuring via PCI address:", "PciAddr", *sriovConfig.PciAddr)
			pfName, err = sriovutils.GetPfNameForPciAddr(*sriovConfig.PciAddr)
			if err != nil {
				return err
			}
			log.Info("Got pfName matching PciAddr", "pfName", pfName, "PciAddr", *sriovConfig.PciAddr)
			pfList = append(pfList, pfName)
		} else if sriovConfig.VendorId != nil && sriovConfig.DeviceId != nil {
			log.Info("Configuring via device/vendor ID", "VendorId", *sriovConfig.VendorId, "DeviceId", *sriovConfig.DeviceId)
			pfList, err = sriovutils.GetPfListForVendorAndDevice(*sriovConfig.VendorId, *sriovConfig.DeviceId)
			if err != nil {
				return err
			}
		}
		log.Info("Configuring interfaces", "pfList", pfList)
		for _, pfName := range pfList {
			if !sriovutils.VerifyPfExists(pfName) {
				log.Info("NIC does not exist on host, skipping...", "pfName", pfName)
				continue
			}
			if err := sriovutils.CreateVfsForPfName(pfName, *sriovConfig.NumVfs); err != nil {
				log.Info("Failed to create VFs for PF", "pfName", pfName, "numVfs", *sriovConfig.NumVfs)
				return err
			}

			if sriovConfig.VfDriver != nil {
				if err := sriovutils.EnableDriverForVfs(pfName, *sriovConfig.VfDriver); err != nil {
					log.Info("Failed to set vfDriver", "vfDriver", *sriovConfig.VfDriver, "err", err)
					return err
				}
			} else {
				// If driver field is omitted, set the default kernel driver
				// TODO: How to determine default driver for different NICs?
				if err := sriovutils.EnableDriverForVfs(pfName, "i40evf"); err != nil {
					log.Info("Failed to set vfDriver i40evf", err)
					return err
				}
			}

			if sriovConfig.MTU != nil && *sriovConfig.MTU >= 576 {
				log.Info("sriovConfig MTU", "MTU", *sriovConfig.MTU)
				if err := linkutils.SetMtuForPf(pfName, *sriovConfig.MTU); err != nil {
					log.Info("Failed to set MTU for PF and its VFs", "pfName", pfName, "MTU", *sriovConfig.MTU)
					return err
				}
			}
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HostNetworkTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&plumberv1.HostNetworkTemplate{}).
		Complete(r)
}

// Helper functions to check and remove string from a slice of strings.
func contains(s []byte, str string) bool {
	for _, v := range s {
		log.Info("contains ", "item", string(v))
		if string(v) == str {
			return true
		}
	}
	return false
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}

func findPciAddr(nodeInterface string) (string, error) {
	cmd := "ethtool -i " + nodeInterface + "| grep bus-info | awk '{print $2}'"
	out, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		log.Error(err, "Error finding PCI address of", "interface", nodeInterface)
		return "", err
	}
	pci := strings.TrimSuffix(string(out), "\n")
	return pci, nil
}
