//go:build darwin

package nat

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
)

const (
	pfConfPath = "/etc/pf.conf"
	anchorName = "vpn"
	anchorPath = "/etc/pf.anchors/vpn"
	confMarker = "# added by vpn-server"
)

func enable(subnet *net.IPNet, tunName string) (Cleanup, error) {
	if err := run("sysctl", "-w", "net.inet.ip.forwarding=1"); err != nil {
		return nil, fmt.Errorf("enable ip forwarding: %w", err)
	}

	extIf, err := defaultInterface()
	if err != nil {
		return nil, err
	}
	log.Printf("NAT: masquerading %s out of %s", subnet, extIf)

	rules := fmt.Sprintf("nat on %s from %s to any -> (%s)\n", extIf, subnet, extIf)
	if err := os.WriteFile(anchorPath, []byte(rules), 0o644); err != nil {
		return nil, fmt.Errorf("write %s: %w", anchorPath, err)
	}

	if err := referenceAnchor(); err != nil {
		return nil, err
	}
	// Reload the main ruleset so it picks up the anchor reference.
	if err := run("pfctl", "-f", pfConfPath); err != nil {
		return nil, fmt.Errorf("reload %s: %w", pfConfPath, err)
	}

	token := enablePF()

	if err := run("pfctl", "-a", anchorName, "-f", anchorPath); err != nil {
		return nil, fmt.Errorf("load pf anchor %q: %w", anchorName, err)
	}

	return func() {
		if err := run("pfctl", "-a", anchorName, "-F", "nat"); err != nil {
			log.Printf("NAT: could not flush pf anchor: %v", err)
		}
		if token != "" {
			_ = run("pfctl", "-X", token)
		}
		_ = os.Remove(anchorPath)
	}, nil
}

// referenceAnchor adds our nat-anchor to /etc/pf.conf if it is not there yet.
// pf demands translation rules before filter rules, so the line goes directly
// after the last existing nat-anchor rather than at the end of the file.
func referenceAnchor() error {
	raw, err := os.ReadFile(pfConfPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", pfConfPath, err)
	}

	reference := fmt.Sprintf("nat-anchor %q", anchorName)
	if strings.Contains(string(raw), reference) {
		return nil
	}

	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	insertAt := 0
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "nat-anchor") {
			insertAt = i + 1
		}
	}

	updated := make([]string, 0, len(lines)+2)
	updated = append(updated, lines[:insertAt]...)
	updated = append(updated, confMarker, reference)
	updated = append(updated, lines[insertAt:]...)

	body := strings.Join(updated, "\n") + "\n"
	if err := os.WriteFile(pfConfPath, []byte(body), 0o644); err != nil {
		return fmt.Errorf("update %s: %w", pfConfPath, err)
	}
	log.Printf("NAT: added %s to %s", reference, pfConfPath)
	return nil
}

// enablePF bumps pf's reference count and returns the token needed to release
// it, so stopping the server does not disable pf for anything else using it.
func enablePF() string {
	out, err := exec.Command("pfctl", "-E").CombinedOutput()
	if err != nil {
		log.Printf("NAT: could not enable pf: %v: %s", err, out)
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if _, token, found := strings.Cut(line, "Token :"); found {
			return strings.TrimSpace(token)
		}
	}
	return ""
}

func defaultInterface() (string, error) {
	out, err := exec.Command("route", "-n", "get", "default").Output()
	if err != nil {
		return "", fmt.Errorf("look up default route: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if _, iface, found := strings.Cut(line, "interface:"); found {
			return strings.TrimSpace(iface), nil
		}
	}
	return "", fmt.Errorf("no default route: the server has no path to the internet")
}
