//go:build linux

package tun

import (
	"fmt"
	"os/exec"
	"strconv"

	"github.com/songgao/water"
)

func newConfig() water.Config {
	return water.Config{DeviceType: water.TUN}
}

func (d *Device) configure() error {
	name := d.iface.Name()
	ones, _ := d.mask.Size()

	addr := exec.Command(
		"ip", "addr", "add",
		fmt.Sprintf("%s/%d", d.ip, ones),
		"dev", name,
	)
	if out, err := addr.CombinedOutput(); err != nil {
		return fmt.Errorf("ip addr add on %s: %w: %s", name, err, out)
	}

	link := exec.Command(
		"ip", "link", "set", "dev", name,
		"mtu", strconv.Itoa(d.mtu),
		"up",
	)
	if out, err := link.CombinedOutput(); err != nil {
		return fmt.Errorf("ip link set %s: %w: %s", name, err, out)
	}

	return nil
}

func bringDown(name string) {
	_ = exec.Command("ip", "link", "set", "dev", name, "down").Run()
}
