//go:build darwin

package tun

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"

	"github.com/songgao/water"
)

func newConfig() water.Config {
	return water.Config{DeviceType: water.TUN}
}

// macOS utun interfaces are point-to-point, so ifconfig wants the address
// twice: once as the local end and once as the peer.
func (d *Device) configure() error {
	name := d.iface.Name()

	ifconfig := exec.Command(
		"ifconfig", name,
		d.ip.String(), d.ip.String(),
		"netmask", net.IP(d.mask).String(),
		"mtu", strconv.Itoa(d.mtu),
		"up",
	)
	if out, err := ifconfig.CombinedOutput(); err != nil {
		return fmt.Errorf("ifconfig %s: %w: %s", name, err, out)
	}

	ones, _ := d.mask.Size()
	network := d.ip.Mask(d.mask)
	route := exec.Command(
		"route", "-q", "add", "-net",
		fmt.Sprintf("%s/%d", network, ones),
		"-interface", name,
	)
	// The route survives a crashed process, so a reconnect finds it present.
	_ = route.Run()

	return nil
}

func bringDown(name string) {
	_ = exec.Command("ifconfig", name, "down").Run()
}
