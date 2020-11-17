/*


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

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	plumberv1 "hostplumber/api/v1"
	hoststate "hostplumber/pkg/hoststate"
	iputils "hostplumber/pkg/utils/ip"
	linkutils "hostplumber/pkg/utils/link"
	sriovutils "hostplumber/pkg/utils/sriov"
)

var log logr.Logger

// HostNetworkTemplateReconciler reconciles a HostNetworkTemplate object
type HostNetworkTemplateReconciler struct {
	client.Client
	Log       logr.Logger
	Scheme    *runtime.Scheme
	NodeName  string
	Namespace string
}

// +kubebuilder:rbac:groups=plumber.k8s.pf9.io,resources=hostnetworktemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=plumber.k8s.pf9.io,resources=hostnetworktemplates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=*,resources=*,verbs=*

func (r *HostNetworkTemplateReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log = r.Log.WithValues("hostconfig", req.NamespacedName)

	var hostConfigReq = plumberv1.HostNetworkTemplate{}
	if err := r.Get(ctx, req.NamespacedName, &hostConfigReq); err != nil {
		log.Error(err, "unable to fetch HostNetworkTemplate")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	myNode := &corev1.Node{}
	nsm := types.NamespacedName{Name: r.NodeName}
	if err := r.Get(ctx, nsm, myNode); err != nil {
		log.Error(err, "Failed to get Node with name", "NodeName", r.NodeName)
		return ctrl.Result{}, err
	}

	selector := labels.SelectorFromSet(hostConfigReq.Spec.NodeSelector)
	if !selector.Matches(labels.Set(myNode.Labels)) {
		log.Info("Node labels don't match template selectors, skipping", "nodeSelector", selector)
		return ctrl.Result{}, nil
	} else {
		log.Info("Labels match, applying HostNetworkTemplate", "nodeSelector", selector)
	}

	// Everything that is traditonally done under "ifconfig <ifname>" handled here
	// Alternatively newer "ip addr" and "ip link" - see https://www.redhat.com/sysadmin/ifconfig-vs-ip
	// MTUs, IPs, routes, link up/down, etc...
	ifConfigList := hostConfigReq.Spec.InterfaceConfig
	if len(ifConfigList) > 0 {
		if err := applyInterfaceConfig(ifConfigList); err != nil {
			log.Error(err, "Failed to apply interfaceConfig")
			return ctrl.Result{}, err
		}
	} else {
		log.Info("interfaceConfig is empty")
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

	hni := hoststate.New(r.NodeName, r.Namespace, r.Client)
	hni.DiscoverHostState()

	return ctrl.Result{}, nil
}

func applyInterfaceConfig(ifConfigList []plumberv1.InterfaceConfig) error {
	for _, ifConfig := range ifConfigList {
		var ifName string

		// TODO: nil check needed? This does NOT have an omitempty json tag in _types.go
		ifName = *ifConfig.Name
		if ifConfig.MTU != nil {
			if err := linkutils.SetMtuForPf(ifName, *ifConfig.MTU); err != nil {
				log.Error(err, "Failed to set MTU for ifName", "ifName", ifName, "MTU", *ifConfig.MTU)
				return err
			}
		}

		if ifConfig.IPv4 != nil {
			v4Config := ifConfig.IPv4
			if len(v4Config.Address) == 0 {
				log.Info("No IPv4 addresses specified... skipping...")
			} else {
				for _, addr := range v4Config.Address {
					log.Info("Attempting to configure IP", "ifName", ifName, "IPv4", addr)
					if err := iputils.SetIpv4Cidr(ifName, addr); err != nil {
						log.Error(err, "Failed to IP for ifName", "ifName", ifName, "IPv4", addr)
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
				for _, addr := range v6Config.Address {
					log.Info("Attempting to configure IP", "ifName", ifName, "IPv6", addr)
					if err := iputils.SetIpv6Cidr(ifName, addr); err != nil {
						log.Error(err, "Failed to set IP for ifName", "ifName", ifName, "IPv6", addr)
						return err
					}
				}
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
				sriovutils.EnableDriverForVfs(pfName, *sriovConfig.VfDriver)
			} else {
				// If driver field is omitted, set the default kernel driver
				// TODO: How to determine default driver for different NICs?
				sriovutils.EnableDriverForVfs(pfName, "ixgbevf")
			}

			if sriovConfig.MTU != nil && *sriovConfig.MTU > 0 {
				if err := linkutils.SetMtuForPf(pfName, *sriovConfig.MTU); err != nil {
					log.Info("Failed to set MTU for PF and its VFs", "pfName", pfName, "MTU", *sriovConfig.MTU)
					return err
				}
			}
		}
	}
	return nil
}

func (r *HostNetworkTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&plumberv1.HostNetworkTemplate{}).
		Complete(r)
}
