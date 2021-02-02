package ip

import (
	"github.com/vishvananda/netlink"
)

func GetIpv4Cidr(ifName string) (*[]string, error) {
	var ipList *[]string = new([]string)
	link, _ := netlink.LinkByName(ifName)
	ipAddrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
	if err != nil {
		return nil, err
	} else {
		for _, addr := range ipAddrs {
			var ipAddr string

			ipAddr = addr.IPNet.String()
			*ipList = append(*ipList, ipAddr)
		}
	}
	return ipList, nil
}

func SetIpv4Cidr(ifName string, ipCidr string) error {
	ifLink, err := netlink.LinkByName(ifName)
	if err != nil {
		return err
	}
	addr, err := netlink.ParseAddr(ipCidr)
	if err != nil {
		return err
	}
	if err := netlink.AddrReplace(ifLink, addr); err != nil {
		return nil
	}
	return nil
}

func DelIpv4Cidr(ifName string, ipCidr string) error {
	ifLink, err := netlink.LinkByName(ifName)
	if err != nil {
		return err
	}
	addr, err := netlink.ParseAddr(ipCidr)
	if err != nil {
		return err
	}
	if err := netlink.AddrDel(ifLink, addr); err != nil {
		return nil
	}
	return nil
}

func GetIpv6Cidr(ifName string) (*[]string, error) {
	var ipList *[]string = new([]string)
	link, _ := netlink.LinkByName(ifName)
	ipAddrs, err := netlink.AddrList(link, netlink.FAMILY_V6)
	if err != nil {
		return nil, err
	} else {
		for _, addr := range ipAddrs {
			var ipAddr string
			ipAddr = addr.IPNet.String()
			*ipList = append(*ipList, ipAddr)
		}
	}
	return ipList, nil
}

// TODO: This is identical to the v4 function for now
// Keep incase v4 vs v6 differences arise, or remove?
func SetIpv6Cidr(ifName string, ipCidr string) error {
	ifLink, err := netlink.LinkByName(ifName)
	if err != nil {
		return err
	}
	addr, err := netlink.ParseAddr(ipCidr)
	if err != nil {
		return err
	}
	if err := netlink.AddrReplace(ifLink, addr); err != nil {
		return nil
	}
	return nil
}

func DelIpv6Cidr(ifName string, ipCidr string) error {
	ifLink, err := netlink.LinkByName(ifName)
	if err != nil {
		return err
	}
	addr, err := netlink.ParseAddr(ipCidr)
	if err != nil {
		return err
	}
	if err := netlink.AddrDel(ifLink, addr); err != nil {
		return nil
	}
	return nil
}
