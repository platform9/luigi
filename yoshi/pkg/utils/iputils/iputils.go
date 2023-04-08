package iputils

import (
	"crypto/rand"
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

func GenerateRandomMAC() (string, error) {
	MAC := make([]byte, 6, 6)
	n, err := rand.Read(MAC)
	if n != len(MAC) || err != nil {
		return "", fmt.Errorf("Failed to generate MAC address: %s", err)
	}

	// MAC needs to be local and unicast, set correct bits
	MAC[0] |= 0x02
	MAC[0] &= 0xfe

	MACStr := fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x\n", MAC[0], MAC[1], MAC[2], MAC[3], MAC[4], MAC[5])

	return MACStr, nil
}
