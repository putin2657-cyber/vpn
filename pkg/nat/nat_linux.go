//go:build linux

package nat

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"strings"
)

const nftTable = "vpn"

func enable(subnet *net.IPNet, tunName string) (Cleanup, error) {
	if err := run("sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
		return nil, fmt.Errorf("enable ip forwarding: %w", err)
	}

	extIf, err := defaultInterface()
	if err != nil {
		return nil, err
	}
	log.Printf("NAT: masquerading %s out of %s", subnet, extIf)

	switch {
	case hasCommand("nft"):
		return enableNftables(subnet, tunName, extIf)
	case hasCommand("iptables"):
		return enableIptables(subnet, tunName, extIf)
	default:
		return nil, fmt.Errorf("neither nft nor iptables is installed")
	}
}

func enableNftables(subnet *net.IPNet, tunName, extIf string) (Cleanup, error) {
	// A previous run may have died without cleaning up.
	_ = run("nft", "delete", "table", "ip", nftTable)

	steps := [][]string{
		{"add", "table", "ip", nftTable},
		{"add", "chain", "ip", nftTable, "postrouting", "{ type nat hook postrouting priority 100 ; }"},
		{"add", "rule", "ip", nftTable, "postrouting", "ip", "saddr", subnet.String(), "oifname", extIf, "masquerade"},
		{"add", "chain", "ip", nftTable, "forward", "{ type filter hook forward priority 0 ; }"},
		{"add", "rule", "ip", nftTable, "forward", "iifname", tunName, "oifname", extIf, "accept"},
		{"add", "rule", "ip", nftTable, "forward", "iifname", extIf, "oifname", tunName, "ct", "state", "related,established", "accept"},
	}
	for _, step := range steps {
		if err := run("nft", step...); err != nil {
			_ = run("nft", "delete", "table", "ip", nftTable)
			return nil, fmt.Errorf("nft %s: %w", strings.Join(step, " "), err)
		}
	}

	return func() {
		if err := run("nft", "delete", "table", "ip", nftTable); err != nil {
			log.Printf("NAT: could not remove nft table %s: %v", nftTable, err)
		}
	}, nil
}

func enableIptables(subnet *net.IPNet, tunName, extIf string) (Cleanup, error) {
	rules := [][]string{
		{"-t", "nat", "-A", "POSTROUTING", "-s", subnet.String(), "-o", extIf, "-j", "MASQUERADE"},
		{"-A", "FORWARD", "-i", tunName, "-o", extIf, "-j", "ACCEPT"},
		{"-A", "FORWARD", "-i", extIf, "-o", tunName, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
	}

	var applied [][]string
	for _, rule := range rules {
		if err := run("iptables", rule...); err != nil {
			removeIptables(applied)
			return nil, fmt.Errorf("iptables %s: %w", strings.Join(rule, " "), err)
		}
		applied = append(applied, rule)
	}

	return func() { removeIptables(applied) }, nil
}

func removeIptables(rules [][]string) {
	for _, rule := range rules {
		del := make([]string, len(rule))
		copy(del, rule)
		for i, arg := range del {
			if arg == "-A" {
				del[i] = "-D"
			}
		}
		if err := run("iptables", del...); err != nil {
			log.Printf("NAT: could not remove iptables rule: %v", err)
		}
	}
}

func defaultInterface() (string, error) {
	out, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		return "", fmt.Errorf("look up default route: %w", err)
	}
	fields := strings.Fields(string(out))
	for i, field := range fields {
		if field == "dev" && i+1 < len(fields) {
			return fields[i+1], nil
		}
	}
	return "", fmt.Errorf("no default route: the server has no path to the internet")
}
