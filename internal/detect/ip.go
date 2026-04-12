package detect

import (
	"net"
)

// hasPublicIP returns true if any network interface on this machine
// has a globally routable (non-private, non-loopback) IP address.
func hasPublicIP() bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}

	for _, addr := range addrs {
		// addr is an net.Addr interface — the concrete type is *net.IPNet
		// (a CIDR block like 192.168.1.5/24). We only care about the IP part.
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipNet.IP

		// Skip addresses that are definitely not public.
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			continue
		}

		if isPrivate(ip) {
			continue
		}

		return true
	}
	return false
}

// isPrivate reports whether ip falls in any of the well-known private ranges.
// Go 1.17+ has ip.IsPrivate() but it misses some ranges; we check manually.
func isPrivate(ip net.IP) bool {
	private := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"100.64.0.0/10", // Carrier-grade NAT (RFC 6598)
		"fc00::/7",      // IPv6 unique local
	}

	for _, cidr := range private {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
