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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
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

// HostNetworkConfigReconciler reconciles a HostNetworkConfig object
type HostNetworkConfigReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	NodeName string
}

// +kubebuilder:rbac:groups=plumber.k8s.pf9.io,resources=hostconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=plumber.k8s.pf9.io,resources=hostconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=*,resources=*,verbs=*

func (r *HostNetworkConfigReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("hostconfig", req.NamespacedName)

	var specApplied bool = false
	var hostConfigReq = plumberv1.HostNetworkConfig{}
	if err := r.Get(ctx, req.NamespacedName, &hostConfigReq); err != nil {
		log.Error(err, "unable to fetch HostNetworkConfig")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	sriovConfigList := hostConfigReq.Spec.SriovConfig

	for _, sriovConfig := range sriovConfigList {
		var pfName string
		var pfList []string
		var err error
		if sriovConfig.PfName != nil {
			fmt.Printf("Configuring via PF: %s\n", *sriovConfig.PfName)
			pfName = *sriovConfig.PfName
			pfList = append(pfList, pfName)
		} else if sriovConfig.PciAddr != nil {
			fmt.Printf("Now configuring via PCI: %s\n", *sriovConfig.PciAddr)
			pfName, err = getPfNameForPciAddr(*sriovConfig.PciAddr)
			if err != nil {
				fmt.Printf("Failed to find PciAddr %s\n", *sriovConfig.PciAddr)
				return ctrl.Result{}, err
			}
			fmt.Printf("Got pfName %s matching PCI %s\n", pfName, *sriovConfig.PciAddr)
			fmt.Printf("pfList before = %s\n", pfList)
			pfList = append(pfList, pfName)
			fmt.Printf("pfList after = %s\n", pfList)
		} else if sriovConfig.VendorId != nil && sriovConfig.DeviceId != nil {
			fmt.Printf("Configuring via device/vendor\n")
			pfList, err = getPfListForVendorAndDevice(*sriovConfig.VendorId, *sriovConfig.DeviceId)
			if err != nil {
				fmt.Printf("Failed to find any devices with vendor %s device %s\n", *sriovConfig.VendorId, *sriovConfig.DeviceId)
				return ctrl.Result{}, err
			}
		}
		fmt.Printf("Got matching PFs: %v\n", pfList)
		for _, pfName := range pfList {
			if !verifyPfExists(pfName) {
				fmt.Printf("NIC %s does not exist on host, skipping...\n", pfName)
				specApplied = false
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
					fmt.Printf("Failed setting MTU %d on %s\n", *sriovConfig.MTU, pfName)
					return ctrl.Result{}, err
				}
			}
		}
	}
	if specApplied == true {
		fmt.Printf("Spec was applied on this host!\n")
	}

	hoststate.DiscoverHostState(r.NodeName, r.Client)

	return ctrl.Result{}, nil
}

func verifyPfExists(pfName string) bool {
	_, err := os.Lstat(filepath.Join(sysClassNet, pfName))
	return err == nil
}

func setMtuForPf(pfName string, mtu int) error {
	mtuStr := []byte(strconv.Itoa(mtu))
	mtuFile := filepath.Join(sysClassNet, pfName, "mtu")
	if err := ioutil.WriteFile(mtuFile, mtuStr, 0644); err != nil {
		fmt.Printf("Failed to set MTU %d for interface %s: %s\n", mtu, pfName, err)
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
		fmt.Printf("Error opening sriov_totalvfs file: %s\n", err)
		return 0, err
	}

	var totalVfs int
	_, err = fmt.Fscanf(fd, "%d\n", &totalVfs)
	if err != nil {
		fmt.Printf("Error parsing sriov_totalvfs: %s\n", err)
		return 0, err
	}
	fmt.Printf("totalvfs = %d\n", totalVfs)
	return totalVfs, nil
}

func getCurrentNumVfsForPf(pfName string) (int, error) {
	numVfsFile := filepath.Join(sysClassNet, pfName, "device", "sriov_numvfs")
	fd, err := os.Open(numVfsFile)
	if err != nil {
		fmt.Printf("Error opening sriov_numvfs file: %s\n", err)
		return 0, err
	}

	var numVfs int
	_, err = fmt.Fscanf(fd, "%d\n", &numVfs)
	if err != nil {
		fmt.Printf("Error parsing sriov_numvfs: %s\n", err)
		return 0, err
	}
	fmt.Printf("numvfs = %d\n", numVfs)
	return numVfs, nil
}

func createVfsForPfName(pfName string, numVfs int) error {
	totalVfs, err := getTotalVfsForPf(pfName)
	if err != nil {
		fmt.Printf("error getting totalvfs\n")
		return err
	}
	if numVfs > totalVfs {
		fmt.Printf("Error: Trying to create %d VFs, NIC only supports %d\n", numVfs, totalVfs)
		return errors.NewBadRequest("Can't create more VFs than total supported by PF")
	}

	currVfs, err := getCurrentNumVfsForPf(pfName)
	if err != nil {
		fmt.Printf("error getting current VFs\n")
		return err
	}

	// Only create VFs if mismatch as it is a disruptive operation
	// Changing sriov_numvfs requires clearing it by writing 0 VFs first
	if currVfs == numVfs {
		fmt.Printf("numvfs is already set to %d, not re-creating\n", currVfs)
		return nil
	}

	numVfsFile := filepath.Join(sysClassNet, pfName, "device", "sriov_numvfs")
	fmt.Printf("Creating %d VFs for file %s\n", numVfs, numVfsFile)

	zero := []byte(strconv.Itoa(0))
	err = ioutil.WriteFile(numVfsFile, zero, os.ModeAppend)
	if err != nil {
		fmt.Printf("resetSriovNumVfs(): fail to reset NumVfs file %s\n", numVfsFile)
		return err
	}

	vfs := []byte(strconv.Itoa(numVfs))
	err = ioutil.WriteFile(numVfsFile, vfs, os.ModeAppend)
	if err != nil {
		fmt.Printf("setSriovNumVfs(): fail to set NumVfs file %s\n", numVfsFile)
		return err
	}

	return nil
}

func setDriverOverride(vfPath string, driver string) error {
	driverBytes := []byte(driver)
	driverOverride := filepath.Join(vfPath, "driver_override")
	err := ioutil.WriteFile(driverOverride, driverBytes, 0644)
	if err != nil {
		fmt.Printf("Failed to set driver %s to %s\n", driver, driverOverride)
		return err
	}
	return nil
}

func unbindOldDriver(vfPath string) error {
	driverPathLink := filepath.Join(vfPath, "driver")
	driverPath, err := filepath.EvalSymlinks(driverPathLink)
	if err != nil {
		fmt.Printf("driver symlink not found... asuming device is unbound to any driver\n")
		return err
	}
	pciAddr := filepath.Base(vfPath)
	pciAddrBytes := []byte(pciAddr)
	err = ioutil.WriteFile(filepath.Join(driverPath, "unbind"), pciAddrBytes, 0644)
	if err != nil {
		fmt.Printf("Error unbinding device %s\n", pciAddr)
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
		fmt.Printf("Error binding device %s to driver %s\n", pciAddr, driver)
		return err
	}
	fmt.Printf("Success binding device %s to driver %s\n", pciAddr, driver)
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
	vendorId := "0"
	deviceId := "0"
	devicePathLink := filepath.Join(sysClassNet, pfName, "device")
	devicePath, _ := filepath.EvalSymlinks(devicePathLink)

	err := filepath.Walk(devicePath, func(path string, info os.FileInfo, err error) error {
		name := info.Name()
		fmt.Printf("Path = %s\n", path)
		fmt.Printf("Inspecting... %s\n", name)

		if info.Name() == "vendor" {
			data, err := ioutil.ReadFile(filepath.Join(devicePath, name))
			if err != nil {
				fmt.Printf("Error reading vendor file for pfName %s\n", pfName)
				return nil
			}
			vendorId = string(data)
			fmt.Printf("Got vendorId = %s\n", vendorId)
		}

		if name == "device" {
			data, err := ioutil.ReadFile(filepath.Join(devicePath, info.Name()))
			if err != nil {
				fmt.Printf("Error reading device file for pfName %s\n", pfName)
				return nil
			}
			deviceId = string(data)
			fmt.Printf("Got deviceId = %s\n", deviceId)
		}

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
		fmt.Printf("Failed doing filepath.Walk on device!\n")
		return
	}
}

func getPfListForVendorAndDevice(vendor, device string) ([]string, error) {
	// This function will return any IF that matches vendor and device ID
	// If optional PCI address is given, it must match
	var matchingPfs []string
	fmt.Printf("foo\n")
	err := filepath.Walk(sysClassNet, func(path string, info os.FileInfo, err error) error {
		ifName := info.Name()
		// Everything in this directory is a symlink
		ifPath, _ := filepath.EvalSymlinks(path)
		if ifPath == sysClassNet {
			fmt.Printf("skipping netDir\n")
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
			fmt.Printf("Error opening device ID file: %s\n", err)
			return err
		}
		var deviceId string
		_, err = fmt.Fscanf(fd, "0x%s\n", &deviceId)
		if err != nil {
			fmt.Printf("Error getting device ID\n")
			return err
		}

		vendorIdFile := filepath.Join(devicePath, "vendor")
		if _, err := os.Stat(vendorIdFile); err != nil {
			return nil
		}
		fd, err = os.Open(vendorIdFile)
		if err != nil {
			fmt.Printf("Error opening vendor ID file: %s\n", err)
			return err
		}
		var vendorId string
		_, err = fmt.Fscanf(fd, "0x%s\n", &vendorId)
		if err != nil {
			fmt.Printf("Error getting vendor ID\n")
			return err
		}
		fmt.Printf("Got vendor %s device %s\n", vendorId, deviceId)
		if vendor == vendorId && device == deviceId {
			matchingPfs = append(matchingPfs, ifName)
			fmt.Printf("Found matching PF: %s\n", ifName)
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

func (r *HostNetworkConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&plumberv1.HostNetworkConfig{}).
		Complete(r)
}
