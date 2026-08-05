package tun

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"

	"github.com/songgao/water"
)

// Device wraps a system TUN interface.
type Device struct {
	iface *water.Interface
	ip    net.IP
	mask  net.IPMask
}

// Open creates and configures a TUN device with the given address.
// Requires root/administrator privileges on macOS and Linux.
func Open(ip net.IP, mask net.IPMask) (*Device, error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return nil, fmt.Errorf("TUN is supported only on macOS and Linux")
	}

	config := water.Config{
		DeviceType: water.TUN,
	}
	iface, err := water.New(config)
	if err != nil {
		return nil, fmt.Errorf("create TUN: %w", err)
	}

	d := &Device{
		iface: iface,
		ip:    ip.To4(),
		mask:  mask,
	}
	if d.ip == nil {
		iface.Close()
		return nil, fmt.Errorf("only IPv4 addresses are supported")
	}
	if err := d.configure(); err != nil {
		iface.Close()
		return nil, err
	}
	return d, nil
}

func (d *Device) configure() error {
	name := d.iface.Name()
	maskIP := net.IP(d.mask).String()

	ifconfig := exec.Command(
		"ifconfig", name,
		d.ip.String(), d.ip.String(),
		"netmask", maskIP,
		"mtu", "1500", "up",
	)
	if out, err := ifconfig.CombinedOutput(); err != nil {
		return fmt.Errorf("ifconfig %s: %w: %s", name, err, out)
	}

	ones, _ := d.mask.Size()
	network := d.ip.Mask(d.mask)
	route := exec.Command(
		"route", "add", "-net",
		fmt.Sprintf("%s/%d", network.String(), ones),
		"-interface", name,
	)
	if out, err := route.CombinedOutput(); err != nil {
		// Route may already exist during reconnect tests.
		_ = out
	}

	return nil
}

func (d *Device) Name() string {
	return d.iface.Name()
}

func (d *Device) Read() ([]byte, error) {
	buf := make([]byte, 1500)
	n, err := d.iface.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func (d *Device) Write(pkt []byte) (int, error) {
	return d.iface.Write(pkt)
}

func (d *Device) Close() error {
	if d.iface == nil {
		return nil
	}
	name := d.iface.Name()
	err := d.iface.Close()
	exec.Command("ifconfig", name, "down").Run()
	return err
}
