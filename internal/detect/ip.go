package detect

import "net"

// privateNets holds the well-known private/non-routable IP ranges parsed once
// at startup so isPrivate does not call net.ParseCIDR on every invocation.
var privateNets []*net.IPNet

func init() {
	for _, cidr := range []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"100.64.0.0/10", // Carrier-grade NAT (RFC 6598)
		"fc00::/7",      // IPv6 unique local
	} {
		_, network, _ := net.ParseCIDR(cidr)
		privateNets = append(privateNets, network)
	}
}

// hasPublicIP returns true if any network interface on this machine
// has a globally routable (non-private, non-loopback) IP address.
func hasPublicIP() bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipNet.IP
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
func isPrivate(ip net.IP) bool {
	for _, network := range privateNets {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
