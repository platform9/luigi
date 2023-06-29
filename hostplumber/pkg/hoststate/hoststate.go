package hoststate

import (
	"context"
	"fmt"
	plumberv1 "hostplumber/api/v1"
	"hostplumber/pkg/consts"
	iputils "hostplumber/pkg/utils/ip"
	linkutils "hostplumber/pkg/utils/link"
	sriovutils "hostplumber/pkg/utils/sriov"
	"os"
	"path/filepath"

	"github.com/vishvananda/netlink"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type HostNetworkInfo struct {
	log           *zap.SugaredLogger
	client        client.Client
	currentStatus *plumberv1.HostNetworkStatus
	nodeName      string
	namespace     string
}

func New(nodeName, namespace string, k8sclient client.Client) *HostNetworkInfo {
	hni := new(HostNetworkInfo)
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	hni.log = logger.Sugar()
	hni.client = k8sclient
	hni.currentStatus = new(plumberv1.HostNetworkStatus)
	hni.nodeName = nodeName
	hni.namespace = namespace

	return hni
}

func (hni *HostNetworkInfo) DiscoverHostState() {
	ctx := context.Background()

	if err := hni.discoverRoutingTable(); err != nil {
		hni.log.Error("Failed to discover routing table ", zap.Error(err))
		// TODO: Don't return? Discover as much info as we can
		//return
	}
	hni.discoverInterfaceStatus()

	// TODO: Discover OVS related info here
	// hni.discoverOvsInfo()

	oldHostState := &plumberv1.HostNetwork{}
	newHostState := &plumberv1.HostNetwork{}
	newHostState.Name = hni.nodeName
	newHostState.Namespace = hni.namespace
	newHostState.Status = *hni.currentStatus

	nsn := types.NamespacedName{Name: hni.nodeName, Namespace: hni.namespace}
	err := hni.client.Get(ctx, nsn, oldHostState)
	if err != nil && errors.IsNotFound(err) {
		hni.log.Error("HostStateSpec not found... creating ", zap.Error(err))
		if err := hni.client.Create(ctx, newHostState); err != nil {
			hni.log.Infof("Failed to created new HostState for Node %s", hni.nodeName)
			return
		}
	} else if err != nil {
		hni.log.Error("Failed to fetch HostNetwork ", zap.Error(err))
		if err := hni.client.Create(ctx, newHostState); err != nil {
			hni.log.Error("Failed to create new HostNetwork ", zap.Error(err))
			return
		}
		return
	} else {
		if err := hni.client.Delete(ctx, oldHostState); err != nil {
			hni.log.Error("Error deleting old HostState ", zap.Error(err))
			return
		}
		if err := hni.client.Create(ctx, newHostState); err != nil {
			hni.log.Error("Failed to create new HostNetwork ", zap.Error(err))
			return
		}
	}
}

func (hni *HostNetworkInfo) discoverRoutingTable() error {
	var currRoutes *plumberv1.Routes = new(plumberv1.Routes)
	var currV4Routes []*plumberv1.Route
	var currV6Routes []*plumberv1.Route
	linkList, err := netlink.LinkList()
	if err != nil {
		return err
	}
	for _, link := range linkList {
		linkAttrs := link.Attrs()
		linkName := linkAttrs.Name

		v4routes, err := netlink.RouteList(link, netlink.FAMILY_V4)
		if err != nil {
			hni.log.Warnw("No IPv4 routes for link", "interface", linkName, "err", err)
		}

		for _, v4route := range v4routes {
			var route *plumberv1.Route = new(plumberv1.Route)
			if v4route.Dst != nil {
				route.Dst = (*v4route.Dst).String()
			}
			if v4route.Gw != nil {
				route.Gw = (v4route.Gw).String()
			}
			if v4route.Src != nil {
				route.Src = (v4route.Src).String()
			}
			route.Dev = linkName
			currV4Routes = append(currV4Routes, route)
		}

		v6routes, err := netlink.RouteList(link, netlink.FAMILY_V6)
		if err != nil {
			hni.log.Warnw("No IPv6 routes for link", "interface", linkName, "err", err)
		}

		for _, v6route := range v6routes {
			var route *plumberv1.Route = new(plumberv1.Route)
			if v6route.Dst != nil {
				route.Dst = (*v6route.Dst).String()
			}
			if v6route.Gw != nil {
				route.Gw = (v6route.Gw).String()
			}
			if v6route.Src != nil {
				route.Src = (v6route.Src).String()
			}
			route.Dev = linkName
			currV6Routes = append(currV6Routes, route)
		}
	}

	if len(currV4Routes) > 0 {
		currRoutes.V4Routes = currV4Routes
	}
	if len(currV6Routes) > 0 {
		currRoutes.V6Routes = currV6Routes
	}

	hni.currentStatus.Routes = currRoutes
	return nil
}

func (hni *HostNetworkInfo) addNetPciDevice(devicePath, ifName string) error {
	var ifStatus *plumberv1.InterfaceStatus = new(plumberv1.InterfaceStatus)

	ifStatus.PfName = ifName
	pciAddr := filepath.Base(devicePath)
	ifStatus.PciAddr = pciAddr

	fd, err := os.Open(filepath.Join(devicePath, "vendor"))
	defer fd.Close()
	if err != nil {
		hni.log.Infof("No vendor file for devicePath: %s\n", devicePath)
		return err
	}
	var vendorId string
	_, err = fmt.Fscanf(fd, "0x%s\n", &vendorId)
	if err != nil {
		hni.log.Infow("Error parsing vendor ID", "ifName", ifName, "err", err)
		return err
	}
	ifStatus.VendorId = vendorId

	fd, err = os.Open(filepath.Join(devicePath, "device"))
	defer fd.Close()
	if err != nil {
		hni.log.Infof("No device file for devicePath: %s\n", devicePath)
		return err
	}
	var deviceId string
	_, err = fmt.Fscanf(fd, "0x%s\n", &deviceId)
	if err != nil {
		hni.log.Infow("Error parsing device ID", "ifName", ifName, "err", err)
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

	ipv4Addrs, err := iputils.GetIpv4Cidr(ifName)
	if err != nil || len(*ipv4Addrs) == 0 {
		hni.log.Infow("Error getting IPv4 for interface", "err", err, "ifName", ifName, "ipv4Addrs", *ipv4Addrs)
	} else {
		ifStatus.IPv4 = new(plumberv1.IPv4Info)
		ifStatus.IPv4.Address = *ipv4Addrs
	}

	ipv6Addrs, err := iputils.GetIpv6Cidr(ifName)
	if err != nil || len(*ipv6Addrs) == 0 {
		hni.log.Infow("Error getting IPv6 for interface", "err", err, "ifName", ifName, "ipv6Addrs", *ipv6Addrs)
	} else {
		ifStatus.IPv6 = new(plumberv1.IPv6Info)
		ifStatus.IPv6.Address = *ipv6Addrs
	}

	var totalVfs int
	totalVfs = sriovutils.GetTotalVfs(devicePath)
	if totalVfs > 0 {
		ifStatus.SriovEnabled = true
		ifStatus.SriovStatus = new(plumberv1.SriovStatus)
		ifStatus.SriovStatus.TotalVfs = totalVfs
		ifStatus.SriovStatus.NumVfs = sriovutils.GetNumVfs(devicePath)
		if err := hni.populateVfInfo(ifStatus.SriovStatus, devicePath, ifName); err != nil {
			hni.log.Infof("Failed to retrieve VF details: %s", err)
			// Ignore error, don't populate VF info
		}
	} else {
		hni.log.Infof("SRIOV disabled for %s", ifName)
		ifStatus.SriovEnabled = false
	}

	hni.currentStatus.InterfaceStatus = append(hni.currentStatus.InterfaceStatus, ifStatus)

	return nil
}

func (hni *HostNetworkInfo) discoverInterfaceStatus() error {
	err := filepath.Walk(consts.SysClassNet, func(path string, info os.FileInfo, err error) error {
		ifName := info.Name()
		ifPath, _ := filepath.EvalSymlinks(path)
		if ifPath == consts.SysClassNet {
			return nil
		}
		if ifName == "lo" {
			return nil
		}
		devicePathLink := filepath.Join(ifPath, "device")
		devicePath, err := filepath.EvalSymlinks(devicePathLink)
		if err != nil {
			hni.log.Infow("Skipping IF with no device link", "ifName", ifName, "ifPath", ifPath, "err", err)
			return nil
		}

		if _, isVf := sriovutils.GetPfDeviceForVf(devicePath); isVf == true {
			// VF info populated later, skip
			return nil
		}

		hni.log.Infow("Adding physical interface", "ifName", ifName, "ifPath", ifPath)
		if err := hni.addNetPciDevice(devicePath, ifName); err != nil {
			hni.log.Infow("Error processing interface", "ifName", ifName, "err", err)
			return nil
		}
		return nil
	})
	if err != nil {
		hni.log.Infow("failed to traverse sys/class/net", "err", err)
		return err
	}
	return nil
}

func (hni *HostNetworkInfo) populateVfInfo(info *plumberv1.SriovStatus, devicePath, pfName string) error {
	linkInfo, err := netlink.LinkByName(pfName)
	if err != nil {
		hni.log.Infow("netlink failed to get PF link", "pfName", pfName, "err", err)
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
