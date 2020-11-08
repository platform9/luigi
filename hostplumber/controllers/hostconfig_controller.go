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
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	plumberv1 "hostplumber/api/v1"
	hoststate "hostplumber/pkg/hoststate"
)

const (
	sysClassNet   = "/host/sys/class/net/"
	sysPciDrivers = "/host/sys/bus/pci/drivers/"
	sysPciDevices = "/host/sys/bus/pci/devices/"
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

	sriovConfigList := hostConfigReq.Spec.SriovConfig
	if len(sriovConfigList) > 0 {
		if err := applySriovConfig(sriovConfigList); err != nil {
			log.Error(err, "Failed to apply SriovConfig")
			return ctrl.Result{}, err
		}
	} else {
		log.Info("SriovConfig is empty")
	}

	hoststate.DiscoverHostState(r.NodeName, r.Namespace, r.Client)

	return ctrl.Result{}, nil
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
			pfName, err = getPfNameForPciAddr(*sriovConfig.PciAddr)
			if err != nil {
				return err
			}
			log.Info("Got pfName matching PciAddr", "pfName", pfName, "PciAddr", *sriovConfig.PciAddr)
			pfList = append(pfList, pfName)
		} else if sriovConfig.VendorId != nil && sriovConfig.DeviceId != nil {
			log.Info("Configuring via device/vendor ID", "VendorId", *sriovConfig.VendorId, "DeviceId", *sriovConfig.DeviceId)
			pfList, err = getPfListForVendorAndDevice(*sriovConfig.VendorId, *sriovConfig.DeviceId)
			if err != nil {
				return err
			}
		}
		log.Info("Configuring interfaces", "pfList", pfList)
		for _, pfName := range pfList {
			if !verifyPfExists(pfName) {
				log.Info("NIC does not exist on host, skipping...", "pfName", pfName)
				continue
			}
			createVfsForPfName(pfName, *sriovConfig.NumVfs)

			if sriovConfig.VfDriver != nil {
				enableDriverForVfs(pfName, *sriovConfig.VfDriver)
			} else {
				// If driver field is omitted, set the default kernel driver
				enableDriverForVfs(pfName, "ixgbevf")
			}

			if sriovConfig.MTU != nil && *sriovConfig.MTU > 0 {
				if err := setMtuForPf(pfName, *sriovConfig.MTU); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func verifyPfExists(pfName string) bool {
	_, err := os.Lstat(filepath.Join(sysClassNet, pfName))
	return err == nil
}

func setMtuForPf(pfName string, mtu int) error {
	mtuStr := []byte(strconv.Itoa(mtu))
	mtuFile := filepath.Join(sysClassNet, pfName, "mtu")
	if err := ioutil.WriteFile(mtuFile, mtuStr, 0644); err != nil {
		log.Error(err, "Failed to set MTU for interface", "MTU", mtu, "pfName", pfName)
		return err
	}

	return setMtuForAllVfs(pfName, mtu)
}

func setMtuForAllVfs(pfName string, mtu int) error {
	pfDevice, _ := filepath.EvalSymlinks(filepath.Join(sysClassNet, pfName, "device"))
	fmt.Printf("Setting MTU for all VFs under PF path: %s\n", pfDevice)
	err := filepath.Walk(pfDevice, func(path string, info os.FileInfo, err error) error {
		name := info.Name()
		if info.Mode()&os.ModeSymlink == os.ModeSymlink {
			isVF, _ := filepath.Match("virtfn*", name)
			if isVF == false {
				return nil
			}
			vfPath, _ := filepath.EvalSymlinks(path)
			if err := setMtuForVf(vfPath, mtu); err != nil {
				fmt.Printf("Failed setting MTU for VF: %s\n", vfPath)
				return err
			}
		}
		return nil
	})
	if err != nil {
		fmt.Printf("Failed to set MTU on VFs for PF %s\n", pfName)
		return err
	}
	return nil
}

func setMtuForVf(vfPath string, mtu int) error {
	mtuStr := []byte(strconv.Itoa(mtu))
	vfNetPath := filepath.Join(vfPath, "net")
	vfEthDevice := vfNetPath + "/eth*"
	vfEth, _ := filepath.Glob(vfEthDevice)
	// This is a valid error, no eth device for DPDK driver or non-Ethernet devices
	if len(vfEth) == 0 {
		fmt.Printf("Skipping VF %s, no kernel Ethernet device\n", filepath.Base(vfPath))
		return nil
	}
	// There will only ever be 1 ethernet device per VF on kernel driver
	eth := vfEth[0]
	mtuFile := filepath.Join(eth, "mtu")
	if err := ioutil.WriteFile(mtuFile, mtuStr, 0644); err != nil {
		fmt.Printf("Failed to set MTU %d for interface %s: %s\n", mtu, filepath.Base(vfPath), err)
		return err
	}
	return nil
}

func getTotalVfsForPf(pfName string) (int, error) {
	totalVfsFile := filepath.Join(sysClassNet, pfName, "device", "sriov_totalvfs")
	fd, err := os.Open(totalVfsFile)
	if err != nil {
		log.Error(err, "Error opening sriov_totalvfs file")
		return 0, err
	}

	var totalVfs int
	_, err = fmt.Fscanf(fd, "%d\n", &totalVfs)
	if err != nil {
		log.Error(err, "Error parsing sriov_totalvfs")
		return 0, err
	}
	return totalVfs, nil
}

func getCurrentNumVfsForPf(pfName string) (int, error) {
	numVfsFile := filepath.Join(sysClassNet, pfName, "device", "sriov_numvfs")
	fd, err := os.Open(numVfsFile)
	if err != nil {
		log.Error(err, "Error opening sriov_numvfs file")
		return 0, err
	}

	var numVfs int
	_, err = fmt.Fscanf(fd, "%d\n", &numVfs)
	if err != nil {
		log.Error(err, "Error parsing sriov_numvfs")
		return 0, err
	}
	return numVfs, nil
}

func createVfsForPfName(pfName string, numVfs int) error {
	totalVfs, err := getTotalVfsForPf(pfName)
	if err != nil {
		return err
	}
	if numVfs > totalVfs {
		log.Info("Cannot create more VFs than supported by NIC", "numVfs", numVfs, "totalVfs", totalVfs)
		return errors.NewBadRequest("Can't create more VFs than total supported by PF")
	}

	currVfs, err := getCurrentNumVfsForPf(pfName)
	if err != nil {
		return err
	}

	// Only create VFs if mismatch as it is a disruptive operation
	// Changing sriov_numvfs requires clearing it by writing 0 VFs first
	if currVfs == numVfs {
		log.Info("numvfs is already set to requested, not re-creating", "sriov_numvfs", currVfs)
		return nil
	}

	numVfsFile := filepath.Join(sysClassNet, pfName, "device", "sriov_numvfs")

	zero := []byte(strconv.Itoa(0))
	err = ioutil.WriteFile(numVfsFile, zero, os.ModeAppend)
	if err != nil {
		return err
	}

	vfs := []byte(strconv.Itoa(numVfs))
	err = ioutil.WriteFile(numVfsFile, vfs, os.ModeAppend)
	if err != nil {
		return err
	}

	return nil
}

func setDriverOverride(vfPath string, driver string) error {
	driverBytes := []byte(driver)
	driverOverride := filepath.Join(vfPath, "driver_override")
	err := ioutil.WriteFile(driverOverride, driverBytes, 0644)
	if err != nil {
		log.Error(err, "Failed to set driver_override", "driver", driver, "driver_override", driverOverride)
		return err
	}
	return nil
}

func unbindOldDriver(vfPath string) error {
	driverPathLink := filepath.Join(vfPath, "driver")
	driverPath, err := filepath.EvalSymlinks(driverPathLink)
	if err != nil {
		log.Error(err, "driver symlink not found for VF", "vfPath", vfPath, "driverPathLink", driverPathLink)
		return err
	}
	pciAddr := filepath.Base(vfPath)
	pciAddrBytes := []byte(pciAddr)
	err = ioutil.WriteFile(filepath.Join(driverPath, "unbind"), pciAddrBytes, 0644)
	if err != nil {
		log.Error(err, "Error unbinding VF from old driver", "pciAddr", pciAddr, "vfPath", vfPath)
		return err
	}
	return nil
}

func bindNewDriver(vfPath string, driver string) error {
	pciAddr := filepath.Base(vfPath)
	pciAddrBytes := []byte(pciAddr)
	driverPath := filepath.Join(sysPciDrivers, driver)

	err := ioutil.WriteFile(filepath.Join(driverPath, "bind"), pciAddrBytes, 0644)
	if err != nil {
		log.Error(err, "Error binding VF to new driver", "pciAddr", pciAddr, "driver", driver)
		return err
	}
	log.Info("Success binding VF to driver", "pciAddr", pciAddr, "driver", driver)
	return nil
}

func setDriverForVf(vfPath string, driver string) error {
	if err := unbindOldDriver(vfPath); err != nil {
		return err
	}
	if err := setDriverOverride(vfPath, driver); err != nil {
		return err
	}
	if err := bindNewDriver(vfPath, driver); err != nil {
		return err
	}
	return nil
}

func enableDriverForVfs(pfName string, driver string) {
	devicePathLink := filepath.Join(sysClassNet, pfName, "device")
	devicePath, _ := filepath.EvalSymlinks(devicePathLink)
	err := filepath.Walk(devicePath, func(path string, info os.FileInfo, err error) error {
		name := info.Name()

		if info.Mode()&os.ModeSymlink == os.ModeSymlink {
			isVF, _ := filepath.Match("virtfn*", name)
			if isVF == false {
				return nil
			}
			fullPath, _ := filepath.EvalSymlinks(path)
			if err := setDriverForVf(fullPath, driver); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		log.Error(err, "Failed doing filepath.Walk on device!")
		return
	}
}

func getPfListForVendorAndDevice(vendor, device string) ([]string, error) {
	// This function will return any IF that matches vendor and device ID
	// If optional PCI address is given, it must match
	var matchingPfs []string
	err := filepath.Walk(sysClassNet, func(path string, info os.FileInfo, err error) error {
		ifName := info.Name()
		// Everything in this directory is a symlink
		ifPath, _ := filepath.EvalSymlinks(path)
		if ifPath == sysClassNet {
			// Skip the root /sys/class/net folder
			return nil
		}
		if ifName == "lo" {
			return nil
		}

		devicePathLink := filepath.Join(ifPath, "device")
		devicePath, _ := filepath.EvalSymlinks(devicePathLink)
		deviceIdFile := filepath.Join(devicePath, "device")

		if _, err := os.Stat(deviceIdFile); err != nil {
			return nil
		}
		fd, err := os.Open(deviceIdFile)
		if err != nil {
			log.Error(err, "Error opening device ID file", "deviceIdFile", deviceIdFile)
			return err
		}
		var deviceId string
		_, err = fmt.Fscanf(fd, "0x%s\n", &deviceId)
		if err != nil {
			log.Error(err, "Error parsing device ID")
			return err
		}

		vendorIdFile := filepath.Join(devicePath, "vendor")
		if _, err := os.Stat(vendorIdFile); err != nil {
			return nil
		}
		fd, err = os.Open(vendorIdFile)
		if err != nil {
			log.Error(err, "Error opening vendor ID file", "vendorIdFile", vendorIdFile)
			return err
		}
		var vendorId string
		_, err = fmt.Fscanf(fd, "0x%s\n", &vendorId)
		if err != nil {
			log.Error(err, "Error parsing vendor ID")
			return err
		}

		if vendor == vendorId && device == deviceId {
			matchingPfs = append(matchingPfs, ifName)
		}
		return nil
	})

	if len(matchingPfs) == 0 {
		err = errors.NewBadRequest("Failed to find any devices matching vendor and device ID")
	}
	return matchingPfs, err
}

func getPfNameForPciAddr(pciAddr string) (string, error) {
	var matchingPf string
	err := filepath.Walk(sysClassNet, func(path string, info os.FileInfo, err error) error {
		ifName := info.Name()
		// Everything in this directory is a symlink
		ifPath, _ := filepath.EvalSymlinks(path)
		if ifPath == sysClassNet {
			return nil
		}
		if ifName == "lo" {
			return nil
		}

		devicePathLink := filepath.Join(ifPath, "device")
		devicePath, _ := filepath.EvalSymlinks(devicePathLink)

		devPci := filepath.Base(devicePath)
		if devPci == pciAddr {
			matchingPf = ifName
			// Short circuit because we found a match
			return io.EOF
		}
		return nil
	})
	if err != nil {
		if err == io.EOF {
			err = nil
		}
	} else {
		err = errors.NewBadRequest("Failed to find interface with matching PCI address")
	}
	return matchingPf, err
}

func (r *HostNetworkTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&plumberv1.HostNetworkTemplate{}).
		Complete(r)
}
