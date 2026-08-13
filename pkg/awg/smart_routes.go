package awg

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
)

var (
	smartAllowedOnce sync.Once
	smartAllowedIPv4 []string
	smartAllowedIPv6 []string
	smartAllowedErr  error
)

func computeSmartAllowed() {
	var err error
	smartAllowedIPv4, err = subtractCIDRs("0.0.0.0/0", smartExcludeCIDRs)
	if err != nil {
		smartAllowedErr = err
		return
	}
	smartAllowedIPv4 = collapseCIDRStrings(smartAllowedIPv4)
	smartAllowedIPv6, err = subtractCIDRs("::/0", smartExcludeIPv6CIDRs)
	if err != nil {
		smartAllowedErr = err
		return
	}
	smartAllowedIPv6 = collapseCIDRStrings(smartAllowedIPv6)
}

// SmartAllowedIPs returns AllowedIPs for TunnelSmart: full tunnel minus the
// curated Russian-service exclude list (banks, marketplaces, Yandex, gov…).
//
// Order matters for flaky mobile clients that truncate long AllowedIPs:
//  1. DNS /32|/128
//  2. Google/YouTube force prefixes
//  3. Remaining routes, broadest prefix first
func SmartAllowedIPs(dns []string) []string {
	smartAllowedOnce.Do(computeSmartAllowed)
	if smartAllowedErr != nil {
		// Fail open to full tunnel rather than break clients.
		return []string{"0.0.0.0/0", "::/0"}
	}

	seen := make(map[string]struct{}, len(smartAllowedIPv4)+len(smartAllowedIPv6)+len(dns)+len(smartForceVPN)+8)
	var head, body []string
	addHead := func(cidr string) {
		if _, ok := seen[cidr]; ok {
			return
		}
		seen[cidr] = struct{}{}
		head = append(head, cidr)
	}
	addBody := func(cidr string) {
		if _, ok := seen[cidr]; ok {
			return
		}
		seen[cidr] = struct{}{}
		body = append(body, cidr)
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
			addHead(v4.String() + "/32")
			continue
		}
		addHead(ip.String() + "/128")
	}
	for _, c := range smartForceVPN {
		addHead(c)
	}
	for _, c := range smartAllowedIPv4 {
		addBody(c)
	}
	for _, c := range smartAllowedIPv6 {
		addBody(c)
	}
	sort.SliceStable(body, func(i, j int) bool {
		return prefixLen(body[i]) < prefixLen(body[j])
	})

	out := make([]string, 0, len(head)+len(body))
	out = append(out, head...)
	out = append(out, body...)
	return out
}

func prefixLen(cidr string) int {
	_, n, err := net.ParseCIDR(cidr)
	if err != nil {
		return 128
	}
	ones, _ := n.Mask.Size()
	return ones
}

// ValidateSmartRoutes checks exclude CIDRs parse and the complement computes.
func ValidateSmartRoutes() error {
	for _, cidr := range smartExcludeCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("exclude route %q: %w", cidr, err)
		}
	}
	for _, cidr := range smartForceVPN {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("force vpn route %q: %w", cidr, err)
		}
	}
	for _, cidr := range smartExcludeIPv6CIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("exclude ipv6 route %q: %w", cidr, err)
		}
	}
	// Reset once so validation (and tests) see fresh computation after edits.
	smartAllowedOnce = sync.Once{}
	smartAllowedErr = nil
	smartAllowedOnce.Do(computeSmartAllowed)
	if smartAllowedErr != nil {
		return smartAllowedErr
	}
	if len(smartAllowedIPv4) < 10 {
		return fmt.Errorf("smart allowed IPv4 list unexpectedly small: %d", len(smartAllowedIPv4))
	}
	// Mobile clients misbehave with enormous route tables.
	if len(smartAllowedIPv4) > 700 {
		return fmt.Errorf("smart allowed IPv4 list too large for mobile: %d", len(smartAllowedIPv4))
	}
	return nil
}
