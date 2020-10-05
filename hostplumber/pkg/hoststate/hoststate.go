package hoststate

import (
	"context"
	"fmt"
	plumberv1 "hostplumber/api/v1"
	"os"
	"path/filepath"

	"github.com/vishvananda/netlink"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	sysClassNet   = "/host/sys/class/net/"
	sysPciDrivers = "/host/sys/bus/pci/drivers/"
	sysPciDevices = "/host/sys/bus/pci/devices/"
)

type HostStateReconciler struct {
	client      client.Client
	currentSpec *plumberv1.HostStateSpec
}

func getTotalVfs(devicePath string) int {
	totalVfsFile := filepath.Join(devicePath, "sriov_totalvfs")
	fd, err := os.Open(totalVfsFile)
	defer fd.Close()
	if err != nil {
		return -1
	}

	var totalVfs int
	_, err = fmt.Fscanf(fd, "%d\n", &totalVfs)
	if err != nil {
		return -1
	}
	return totalVfs
}

func getNumVfs(devicePath string) int {
	numVfsFile := filepath.Join(devicePath, "sriov_numvfs")
	fd, err := os.Open(numVfsFile)
	defer fd.Close()
	if err != nil {
		return -1
	}

	var numVfs int
	_, err = fmt.Fscanf(fd, "%d\n", &numVfs)
	if err != nil {
		return -1
	}
	return numVfs
}

func getVfPciAddrById(pfDevicePath string, vfId int) string {
	virtFn := fmt.Sprintf("virtfn%d", vfId)
	vfDeviceLink := filepath.Join(pfDevicePath, virtFn)
	vfDevicePath, _ := filepath.EvalSymlinks(vfDeviceLink)
	pciAddr := filepath.Base(vfDevicePath)
	return pciAddr
}

func getVfDriverByPci(vfPciAddr string) string {
	driverLink := filepath.Join(sysPciDevices, vfPciAddr, "driver")
	driverPath, _ := filepath.EvalSymlinks(driverLink)
	driverName := filepath.Base(driverPath)
	return driverName
}

func getPfDeviceForVf(devicePath string) (string, bool) {
	physfnFile := filepath.Join(devicePath, "physfn")
	if _, err := os.Lstat(physfnFile); err != nil {
		return "", false
	}
	pfDevicePath, _ := filepath.EvalSymlinks(physfnFile)
	return pfDevicePath, true
}

func getIfMtu(devicePath, ifName string) (int, error) {
	pfNetPath := filepath.Join(devicePath, "net", ifName)
	var mtu int
	fd, err := os.Open(filepath.Join(pfNetPath, "mtu"))
	defer fd.Close()
	if err != nil {
		fmt.Printf("Failed to open mtu for %s\n", pfNetPath)
		return 0, err
	}
	_, err = fmt.Fscanf(fd, "%d\n", &mtu)
	if err != nil {
		fmt.Printf("Failed to read MTU\n")
		return 0, err
	}
	return mtu, nil
}

func getIfMac(devicePath, ifName string) (string, error) {
	pfNetPath := filepath.Join(devicePath, "net", ifName)
	var macAddr string
	fd, err := os.Open(filepath.Join(pfNetPath, "address"))
	defer fd.Close()
	if err != nil {
		fmt.Printf("Failed to open address for %s\n", pfNetPath)
		return "", err
	}
	_, err = fmt.Fscanf(fd, "%s\n", &macAddr)
	if err != nil {
		fmt.Printf("Failed to read MAC Address\n")
		return "", err
	}
	return macAddr, nil
}

func populateVfInfo(info *plumberv1.SriovStatus, devicePath, pfName string) error {
	linkInfo, err := netlink.LinkByName(pfName)
	if err != nil {
		fmt.Printf("Failed to get VfInfo for PF %s from netlink library\n", pfName)
		return err
	}

	var vfList []*plumberv1.VfInfo
	for _, vfLink := range linkInfo.Attrs().Vfs {
		var vf *plumberv1.VfInfo = new(plumberv1.VfInfo)
		vf.ID = vfLink.ID
		vf.Mac = vfLink.Mac.String()
		vf.Vlan = vfLink.Vlan
		vf.Qos = vfLink.Qos
		vf.Spoofchk = vfLink.Spoofchk
		// netlink not returning trust mode, filed bug: https://github.com/vishvananda/netlink/issues/580
		vf.Trust = false
		vf.PciAddr = getVfPciAddrById(devicePath, vf.ID)
		vf.VfDriver = getVfDriverByPci(vf.PciAddr)
		vfList = append(vfList, vf)
	}

	info.Vfs = vfList

	return nil
}

func (r *HostStateReconciler) addNetPciDevice(devicePath, ifName string) error {
	var ifStatus *plumberv1.InterfaceStatus = new(plumberv1.InterfaceStatus)

	ifStatus.PfName = ifName
	pciAddr := filepath.Base(devicePath)
	ifStatus.PciAddr = pciAddr

	fd, err := os.Open(filepath.Join(devicePath, "vendor"))
	defer fd.Close()
	if err != nil {
		fmt.Printf("No device file\n")
		return err
	}
	var vendorId string
	_, err = fmt.Fscanf(fd, "0x%s\n", &vendorId)
	if err != nil {
		fmt.Printf("Error getting vendor ID for ifName = %s\n", ifName)
		return err
	}
	ifStatus.VendorId = vendorId

	fd, err = os.Open(filepath.Join(devicePath, "device"))
	defer fd.Close()
	if err != nil {
		fmt.Printf("No device file\n")
		return err
	}
	var deviceId string
	_, err = fmt.Fscanf(fd, "0x%s\n", &deviceId)
	if err != nil {
		fmt.Printf("Error getting device ID for ifName = %s\n", ifName)
		return err
	}
	ifStatus.DeviceId = deviceId

	driverLink := filepath.Join(devicePath, "driver")
	driverPath, _ := filepath.EvalSymlinks(driverLink)
	driverName := filepath.Base(driverPath)
	ifStatus.PfDriver = driverName

	mac, err := getIfMac(devicePath, ifName)
	if err != nil {
		return err
	}
	ifStatus.MacAddr = mac

	mtu, err := getIfMtu(devicePath, ifName)
	if err != nil {
		return err
	}
	ifStatus.MTU = mtu

	var totalVfs int
	totalVfs = getTotalVfs(devicePath)
	if totalVfs > 0 {
		fmt.Printf("SRIOV enabled: totalVfs = %d\n", totalVfs)
		ifStatus.SriovEnabled = true
		ifStatus.SriovStatus = new(plumberv1.SriovStatus)
		ifStatus.SriovStatus.TotalVfs = totalVfs
		ifStatus.SriovStatus.NumVfs = getNumVfs(devicePath)
		if err := populateVfInfo(ifStatus.SriovStatus, devicePath, ifName); err != nil {
			fmt.Printf("Failed to retrieve VF details: %s\n", err)
			// Ignore error, don't populate VF info
		}
	} else {
		fmt.Printf("SRIOV disabled for %s\n", ifName)
		ifStatus.SriovEnabled = false
	}

	r.currentSpec.InterfaceStatus = append(r.currentSpec.InterfaceStatus, ifStatus)

	return nil
}

func (r *HostStateReconciler) discoverHwState() error {
	err := filepath.Walk(sysClassNet, func(path string, info os.FileInfo, err error) error {
		ifName := info.Name()
		ifPath, _ := filepath.EvalSymlinks(path)
		if ifPath == sysClassNet {
			fmt.Printf("skipping netDir\n")
			return nil
		}
		if ifName == "lo" {
			return nil
		}
		fmt.Printf("Inspecting ifPath = %s\n", ifPath)
		devicePathLink := filepath.Join(ifPath, "device")
		devicePath, err := filepath.EvalSymlinks(devicePathLink)
		if err != nil {
			fmt.Printf("ifName %s ifPath %s is NOT a device\n", ifName, ifPath)
			return nil
		}

		if _, isVf := getPfDeviceForVf(devicePath); isVf == true {
			// VF info populated later, skip
			fmt.Printf("%s is a VF ethernet device, skipping...\n", ifName)
			return nil
		}

		fmt.Printf("ifName %s ifPath %s IS a physical device\n", ifName, ifPath)
		if err := r.addNetPciDevice(devicePath, ifName); err != nil {
			fmt.Printf("Error processing: %s\n", ifName)
			return nil
		}
		return nil
	})
	if err != nil {
		fmt.Printf("failed to traverse sys/class/net\n")
		return err
	}
	return nil
}

func DiscoverHostState(nodeName string, k8sclient client.Client) {
	ctx := context.Background()
	linkList, _ := netlink.LinkList()
	for _, link := range linkList {
		fmt.Printf("link = %+v\n", link)
	}

	hrc := new(HostStateReconciler)
	hrc.client = k8sclient
	hrc.currentSpec = new(plumberv1.HostStateSpec)

	hrc.discoverHwState()

	d, err := yaml.Marshal(&hrc.currentSpec)
	if err != nil {
		fmt.Printf("Failed to marshal!!!: %s\n", err)
	}
	fmt.Printf("\n%s\n\n", string(d))

	oldHostState := &plumberv1.HostState{}
	newHostState := &plumberv1.HostState{}
	newHostState.Name = nodeName
	newHostState.Namespace = os.Getenv("K8S_NAMESPACE")
	newHostState.Spec = *hrc.currentSpec

	nsn := types.NamespacedName{Name: nodeName, Namespace: "luigi-system"}
	err = hrc.client.Get(ctx, nsn, oldHostState)
	if err != nil && errors.IsNotFound(err) {
		fmt.Printf("HostStateSpec not found... creating\n")
		if err := hrc.client.Create(ctx, newHostState); err != nil {
			fmt.Printf("Failed to created new HostState for Node %s\n", nodeName)
			return
		}
	} else if err != nil {
		fmt.Printf("Error fetching HostState CRD and not a NotFound err\n")
		if err := hrc.client.Create(ctx, newHostState); err != nil {
			fmt.Printf("Failed to created new HostState for Node %s\n", nodeName)
			return
		}
		return
	} else {
		if err := hrc.client.Delete(ctx, oldHostState); err != nil {
			fmt.Printf("Error deleting old HostState\n")
			return
		}
		if err := hrc.client.Create(ctx, newHostState); err != nil {
			fmt.Printf("Failed to created new HostState for Node %s\n", nodeName)
			return
		}
	}
}
