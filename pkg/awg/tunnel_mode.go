package awg

import (
	"fmt"
	"strings"
)

// TunnelMode controls which destinations a client sends through the VPN.
type TunnelMode string

const (
	// TunnelSmart is a full tunnel minus a curated Russian-service exclude
	// list (marketplaces, Yandex apps, banks, gov/medical portals, …).
	// Those destinations stay on the ISP path; everything else uses the VPN.
	TunnelSmart TunnelMode = "smart"
	// TunnelFull captures all IPv4 and IPv6 traffic.
	TunnelFull TunnelMode = "full"
)

// DefaultTunnelMode is what vpn-admin and the API use when nothing is set.
const DefaultTunnelMode = TunnelSmart

// ParseTunnelMode accepts "smart", "full", or empty (defaults to smart).
func ParseTunnelMode(s string) (TunnelMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", string(TunnelSmart):
		return TunnelSmart, nil
	case string(TunnelFull):
		return TunnelFull, nil
	default:
		return "", fmt.Errorf("unknown tunnel mode %q (want smart or full)", s)
	}
}
