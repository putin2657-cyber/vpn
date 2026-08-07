package awg

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

// smartServiceCIDRs is a compact allowlist for TunnelSmart. Russian services
// stay off this list on purpose so they use the device's normal ISP path.
//
// Keep this short enough that Amnezia QR import still works. Prefer provider
// aggregates used by the target apps over huge cloud / CDN blocks (those would
// pull unrelated sites, including some Russian ones, into the tunnel).
var smartServiceCIDRs = []string{
	// Google / YouTube
	"8.8.4.0/24",
	"8.8.8.0/24",
	"8.34.208.0/20",
	"8.35.192.0/20",
	"23.236.48.0/20",
	"23.251.128.0/19",
	"64.15.112.0/20",
	"64.233.160.0/19",
	"66.102.0.0/20",
	"66.249.64.0/19",
	"70.32.128.0/19",
	"72.14.192.0/18",
	"74.125.0.0/16",
	"108.177.0.0/17",
	"142.250.0.0/15",
	"172.217.0.0/16",
	"172.253.0.0/16",
	"173.194.0.0/16",
	"192.178.0.0/15",
	"199.36.154.0/23",
	"199.36.156.0/24",
	"207.223.160.0/20",
	"208.65.152.0/22",
	"208.117.224.0/19",
	"209.85.128.0/17",
	"216.58.192.0/19",
	"216.239.32.0/19",
	"2001:4860::/32",
	"2404:6800::/32",
	"2607:f8b0::/32",
	"2800:3f0::/32",
	"2a00:1450::/32",
	"2c0f:fb50::/32",

	// Meta: Instagram, Facebook, WhatsApp, Threads
	"31.13.24.0/21",
	"31.13.64.0/18",
	"31.13.96.0/19",
	"45.64.40.0/22",
	"66.220.144.0/20",
	"69.63.176.0/20",
	"69.171.224.0/19",
	"74.119.76.0/22",
	"102.132.96.0/20",
	"103.4.96.0/22",
	"129.134.0.0/16",
	"157.240.0.0/16",
	"163.70.128.0/17",
	"173.252.64.0/18",
	"179.60.192.0/22",
	"185.60.216.0/22",
	"204.15.20.0/22",
	"2a03:2880::/32",

	// Discord (own ranges; some voice/media still sits on cloud hosts)
	"66.22.192.0/18",
	"138.128.136.0/21",
	"162.159.128.0/19",
	"162.245.208.0/21",

	// X / Twitter
	"69.195.160.0/19",
	"104.244.42.0/24",
	"104.244.43.0/24",
	"104.244.44.0/24",
	"104.244.45.0/24",
	"104.244.46.0/24",
	"192.133.76.0/22",
	"196.201.220.0/23",
	"199.16.156.0/22",
	"199.59.148.0/22",
	"199.96.56.0/21",
	"202.160.128.0/22",
	"209.237.192.0/19",

	// Reddit (Fastly)
	"151.101.0.0/16",
	"199.232.0.0/16",

	// Twitch
	"52.223.192.0/18",
	"99.181.64.0/18",
	"185.42.204.0/22",

	// Netflix
	"23.246.0.0/18",
	"37.77.184.0/21",
	"45.57.0.0/17",
	"64.120.128.0/17",
	"66.197.128.0/17",
	"108.175.32.0/20",
	"185.2.220.0/22",
	"185.9.188.0/22",
	"192.173.64.0/18",
	"198.38.96.0/19",
	"198.45.48.0/20",
	"208.75.76.0/22",
}

// SmartAllowedIPs returns the TunnelSmart destination list, always including
// the DNS servers from the client config so resolution itself can use the VPN.
func SmartAllowedIPs(dns []string) []string {
	seen := make(map[string]struct{}, len(smartServiceCIDRs)+len(dns))
	out := make([]string, 0, len(smartServiceCIDRs)+len(dns))

	add := func(cidr string) {
		if _, ok := seen[cidr]; ok {
			return
		}
		seen[cidr] = struct{}{}
		out = append(out, cidr)
	}

	for _, cidr := range smartServiceCIDRs {
		add(cidr)
	}
	for _, server := range dns {
		server = strings.TrimSpace(server)
		if server == "" {
			continue
		}
		ip := net.ParseIP(server)
		if ip == nil {
			continue
		}
		if v4 := ip.To4(); v4 != nil {
			add(v4.String() + "/32")
			continue
		}
		add(ip.String() + "/128")
	}

	sort.Strings(out)
	return out
}

// ValidateSmartRoutes makes sure every baked-in CIDR parses. Called from tests.
func ValidateSmartRoutes() error {
	for _, cidr := range smartServiceCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("smart route %q: %w", cidr, err)
		}
	}
	return nil
}
