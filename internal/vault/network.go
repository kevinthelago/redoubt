package vault

import (
	"fmt"
	"net"
)

// privateRanges are the IPv4 RFC1918 private address ranges.
// Loopback and link-local are intentionally excluded: the vault must be
// accessible from other LAN nodes, not just the machine it runs on.
var privateRanges = func() []*net.IPNet {
	cidrs := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"fc00::/7", // IPv6 ULA (fd00::/8 subset used in practice)
	}
	nets := make([]*net.IPNet, len(cidrs))
	for i, c := range cidrs {
		_, ipnet, err := net.ParseCIDR(c)
		if err != nil {
			panic("invalid hardcoded CIDR: " + c)
		}
		nets[i] = ipnet
	}
	return nets
}()

// ValidateLANAddress returns nil if addr (host:port) is a private RFC1918 LAN address.
// It refuses:
//   - 0.0.0.0 (wildcard — would bind all interfaces including WAN)
//   - Public routable IPs
//   - Loopback addresses (127.x, ::1) — vault must be reachable over LAN
//   - Addresses with no port
//
// This is the enforcement point for Redoubt's no-WAN guarantee.
func ValidateLANAddress(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("%w: %s (must be host:port)", ErrPublicAddress, addr)
	}
	if port == "" {
		return fmt.Errorf("%w: %s (port is required)", ErrPublicAddress, addr)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		return fmt.Errorf("%w: %s (wildcard bind refused)", ErrPublicAddress, addr)
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("%w: %s (not a valid IP address)", ErrPublicAddress, addr)
	}

	// Refuse loopback: vault must be reachable over LAN, not just locally.
	if ip.IsLoopback() {
		return fmt.Errorf("%w: %s (loopback refused — vault must be bound to a LAN interface)", ErrPublicAddress, addr)
	}

	// Refuse link-local (169.254.x.x, fe80::/10)
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("%w: %s (link-local refused)", ErrPublicAddress, addr)
	}

	// Must be in a private range.
	for _, network := range privateRanges {
		if network.Contains(ip) {
			return nil
		}
	}

	return fmt.Errorf("%w: %s is a public address", ErrPublicAddress, addr)
}

// IsPrivateIP reports whether ip falls within an RFC1918 private range.
// Does not include loopback or link-local.
func IsPrivateIP(ip net.IP) bool {
	for _, network := range privateRanges {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// DiscoverPrivateInterfaces returns all local IPv4 addresses that fall within
// RFC1918 private ranges. Returns an error if no private interfaces are found.
func DiscoverPrivateInterfaces() ([]net.IP, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("enumerate network interfaces: %w", err)
	}

	var found []net.IP
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue
			}
			if IsPrivateIP(ip) {
				found = append(found, ip)
			}
		}
	}

	if len(found) == 0 {
		return nil, fmt.Errorf("no private (RFC1918) network interfaces found; ensure this machine has a LAN connection")
	}
	return found, nil
}
