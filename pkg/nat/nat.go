// Package nat gives VPN clients a route to the internet: it turns on IPv4
// forwarding and masquerades traffic leaving the VPN subnet behind the
// server's own address.
package nat

import (
	"fmt"
	"net"
	"os/exec"
)

// Cleanup removes everything Enable installed.
type Cleanup func()

// Enable masquerades traffic from subnet out of the host's default interface.
// It needs root, and the returned Cleanup should run on shutdown so the host
// is not left with dangling firewall rules.
func Enable(subnet *net.IPNet, tunName string) (Cleanup, error) {
	return enable(subnet, tunName)
}

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, out)
	}
	return nil
}

func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
