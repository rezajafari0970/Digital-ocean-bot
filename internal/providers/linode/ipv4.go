package linode

import "net/netip"

// These special-use ranges must not authorize SSH/provisioning readiness.
// Be conservative about special-purpose sub-allocations: Linode instance
// addresses must be ordinary public unicast addresses.
var nonPublicIPv4 = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
}

func publicIPv4(raw string) string {
	ip, err := netip.ParseAddr(raw)
	if err != nil || !ip.Is4() {
		return ""
	}
	for _, prefix := range nonPublicIPv4 {
		if prefix.Contains(ip) {
			return ""
		}
	}
	return ip.String()
}
