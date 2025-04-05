package libmcp

import (
	"context"
	"net"
)

// GetIPAddressInfo retrieves Network interface information of the host
func GetIPAddressInfo(ctx context.Context) (map[string]interface{}, error) {
	// Array to store information of all interfaces
	interfaceInfos := []map[string]interface{}{}

	// Get list of network interfaces
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range interfaces {
		// Get list of addresses for this interface
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		// Process only interfaces with one or more addresses
		if len(addrs) > 0 {
			ipAddresses := []string{}
			for _, addr := range addrs {
				// Extract only IP address from CIDR notation
				ip, _, err := net.ParseCIDR(addr.String())
				if err == nil {
					ipAddresses = append(ipAddresses, ip.String())
				} else {
					// If not in CIDR format, use as is
					ipAddresses = append(ipAddresses, addr.String())
				}
			}

			// Store interface information in map
			interfaceInfo := map[string]interface{}{
				"name":         iface.Name,
				"mac_address":  iface.HardwareAddr.String(),
				"ip_addresses": ipAddresses,
				"mtu":          iface.MTU,
				"flags":        iface.Flags.String(),
			}
			interfaceInfos = append(interfaceInfos, interfaceInfo)
		}
	}

	// Return the result
	result := map[string]interface{}{
		"interfaces": interfaceInfos,
		"count":      len(interfaceInfos),
	}

	return result, nil
}
