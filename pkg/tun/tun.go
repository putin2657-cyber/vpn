package tun

import (
	"fmt"
	"net"
	"os/exec"

	"github.com/songgao/water"
)

// DefaultMTU leaves headroom for the outer TCP/IP and TLS record headers, so a
// full-size tunnelled packet still fits in one segment of the carrier
// connection instead of being fragmented.
const DefaultMTU = 1400

// Device wraps a system TUN interface.
type Device struct {
	iface *water.Interface
	ip    net.IP
	mask  net.IPMask
	mtu   int
}

// Open creates and configures a TUN device with the given address.
// Requires root privileges on macOS and Linux.
func Open(ip net.IP, mask net.IPMask) (*Device, error) {
	return OpenWithMTU(ip, mask, DefaultMTU)
}

// OpenWithMTU is Open with an explicit MTU.
func OpenWithMTU(ip net.IP, mask net.IPMask, mtu int) (*Device, error) {
	v4 := ip.To4()
	if v4 == nil {
		return nil, fmt.Errorf("only IPv4 addresses are supported")
	}

	iface, err := water.New(newConfig())
	if err != nil {
		return nil, fmt.Errorf("create TUN (are you root?): %w", err)
	}

	d := &Device{iface: iface, ip: v4, mask: mask, mtu: mtu}
	if err := d.configure(); err != nil {
		iface.Close()
		return nil, err
	}
	return d, nil
}

func (d *Device) Name() string {
	return d.iface.Name()
}

func (d *Device) MTU() int {
	return d.mtu
}

// IP returns the address assigned to the interface.
func (d *Device) IP() net.IP {
	return d.ip
}

func (d *Device) Read() ([]byte, error) {
	buf := make([]byte, d.mtu)
	n, err := d.iface.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func (d *Device) Write(pkt []byte) (int, error) {
	return d.iface.Write(pkt)
}

// Redirect routes all traffic through the tunnel, except the connection to the
// VPN server itself. The returned function puts the previous routing and DNS
// configuration back.
func Redirect(dev *Device, serverIP net.IP, dns []string) (func(), error) {
	return redirect(dev, serverIP, dns)
}

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, out)
	}
	return nil
}

func (d *Device) Close() error {
	if d.iface == nil {
		return nil
	}
	name := d.iface.Name()
	err := d.iface.Close()
	bringDown(name)
	return err
}
