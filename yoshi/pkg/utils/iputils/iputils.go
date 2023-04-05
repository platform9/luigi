package iputils

import (
	"fmt"
	"net/netip"
)

func AllocateIP(allocations map[string]string, cidrs ...string) (string, error) {
	for _, cidr := range cidrs {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return "", fmt.Errorf("Invalid CIDR: %s", cidr)
		}
		prefix = prefix.Masked()
		for addr := prefix.Addr(); prefix.Contains(addr); addr = addr.Next() {
			addrStr := addr.String()
			if _, exists := allocations[addrStr]; !exists {
				return addrStr, nil
			}
		}
	}
	return "", fmt.Errorf("No available IPs")
}
