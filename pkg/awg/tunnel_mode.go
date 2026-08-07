package awg

import (
	"fmt"
	"strings"
)

// TunnelMode controls which destinations a client sends through the VPN.
type TunnelMode string

const (
	// TunnelSmart sends only curated foreign services through the tunnel.
	// Everything else (Yandex, Gosuslugi, banks, local apps) keeps the ISP IP.
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
