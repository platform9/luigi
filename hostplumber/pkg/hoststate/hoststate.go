package hoststate

import (
	"context"
	"fmt"
	plumberv1 "hostplumber/api/v1"
	"hostplumber/pkg/consts"
	linkutils "hostplumber/pkg/utils/link"
	sriovutils "hostplumber/pkg/utils/sriov"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var log logr.Logger

type HostNetworkInfo struct {
	client        client.Client
	currentStatus *plumberv1.HostNetworkStatus
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
		vf.PciAddr = sriovutils.GetVfPciAddrById(devicePath, vf.ID)
		vf.VfDriver = sriovutils.GetVfDriverByPci(vf.PciAddr)
		vfList = append(vfList, vf)
	}

	info.Vfs = vfList

	return nil
}

func (r *HostNetworkInfo) addNetPciDevice(devicePath, ifName string) error {
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

	mac, err := linkutils.GetIfMac(devicePath, ifName)
	if err != nil {
		return err
	}
	ifStatus.MacAddr = mac

	mtu, err := linkutils.GetIfMtu(devicePath, ifName)
	if err != nil {
		return err
	}
	ifStatus.MTU = mtu

	var totalVfs int
	totalVfs = sriovutils.GetTotalVfs(devicePath)
	if totalVfs > 0 {
		fmt.Printf("SRIOV enabled: totalVfs = %d\n", totalVfs)
		ifStatus.SriovEnabled = true
		ifStatus.SriovStatus = new(plumberv1.SriovStatus)
		ifStatus.SriovStatus.TotalVfs = totalVfs
		ifStatus.SriovStatus.NumVfs = sriovutils.GetNumVfs(devicePath)
		if err := populateVfInfo(ifStatus.SriovStatus, devicePath, ifName); err != nil {
			fmt.Printf("Failed to retrieve VF details: %s\n", err)
			// Ignore error, don't populate VF info
		}
	} else {
		fmt.Printf("SRIOV disabled for %s\n", ifName)
		ifStatus.SriovEnabled = false
	}

	r.currentStatus.InterfaceStatus = append(r.currentStatus.InterfaceStatus, ifStatus)

	return nil
}

func (r *HostNetworkInfo) discoverInterfaceStatus() error {
	err := filepath.Walk(consts.SysClassNet, func(path string, info os.FileInfo, err error) error {
		ifName := info.Name()
		ifPath, _ := filepath.EvalSymlinks(path)
		if ifPath == consts.SysClassNet {
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

		if _, isVf := sriovutils.GetPfDeviceForVf(devicePath); isVf == true {
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

func DiscoverHostState(nodeName, namespace string, k8sclient client.Client) {
	ctx := context.Background()
	linkList, _ := netlink.LinkList()
	for _, link := range linkList {
		fmt.Printf("link = %+v\n", link)
	}

	hni := new(HostNetworkInfo)
	hni.client = k8sclient
	hni.currentStatus = new(plumberv1.HostNetworkStatus)

	hni.discoverInterfaceStatus()

	d, err := yaml.Marshal(&hni.currentStatus)
	if err != nil {
		fmt.Printf("Failed to marshal!!!: %s\n", err)
	}
	fmt.Printf("\n%s\n\n", string(d))

	oldHostState := &plumberv1.HostNetwork{}
	newHostState := &plumberv1.HostNetwork{}
	newHostState.Name = nodeName
	newHostState.Namespace = namespace
	newHostState.Status = *hni.currentStatus

	nsn := types.NamespacedName{Name: nodeName, Namespace: newHostState.Namespace}
	err = hni.client.Get(ctx, nsn, oldHostState)
	if err != nil && errors.IsNotFound(err) {
		fmt.Printf("HostStateSpec not found... creating\n")
		if err := hni.client.Create(ctx, newHostState); err != nil {
			fmt.Printf("Failed to created new HostState for Node %s\n", nodeName)
			return
		}
	} else if err != nil {
		fmt.Printf("Error fetching HostState CRD and not a NotFound err\n")
		if err := hni.client.Create(ctx, newHostState); err != nil {
			fmt.Printf("Failed to created new HostState for Node %s\n", nodeName)
			return
		}
		return
	} else {
		if err := hni.client.Delete(ctx, oldHostState); err != nil {
			fmt.Printf("Error deleting old HostState\n")
			return
		}
		if err := hni.client.Create(ctx, newHostState); err != nil {
			fmt.Printf("Failed to created new HostState for Node %s\n", nodeName)
			return
		}
	}
}
