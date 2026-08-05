//go:build linux

package tun

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"strings"
)

var defaultHalves = []string{"0.0.0.0/1", "128.0.0.0/1"}

func redirect(dev *Device, serverIP net.IP, dns []string) (func(), error) {
	gateway, iface, err := defaultRoute()
	if err != nil {
		return nil, err
	}

	if err := run("ip", "route", "add", serverIP.String()+"/32",
		"via", gateway, "dev", iface); err != nil {
		return nil, fmt.Errorf("pin route to VPN server: %w", err)
	}
	undo := []func(){func() {
		_ = run("ip", "route", "del", serverIP.String()+"/32")
	}}

	rollback := func() {
		for i := len(undo) - 1; i >= 0; i-- {
			undo[i]()
		}
	}

	name := dev.Name()
	for _, half := range defaultHalves {
		if err := run("ip", "route", "add", half, "dev", name); err != nil {
			rollback()
			return nil, fmt.Errorf("route %s via %s: %w", half, name, err)
		}
		undo = append(undo, func() {
			_ = run("ip", "route", "del", half, "dev", name)
		})
	}

	if len(dns) > 0 {
		// resolv.conf is owned by systemd-resolved, NetworkManager or resolvconf
		// depending on the distribution, so leave it to the operator.
		log.Printf("set your resolver to %s manually; this client does not edit resolv.conf",
			strings.Join(dns, ", "))
	}

	log.Printf("all traffic redirected through %s, server reachable via %s", name, gateway)
	return rollback, nil
}

func defaultRoute() (gateway, iface string, err error) {
	out, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		return "", "", fmt.Errorf("look up default route: %w", err)
	}

	fields := strings.Fields(string(out))
	for i, field := range fields {
		switch field {
		case "via":
			if i+1 < len(fields) {
				gateway = fields[i+1]
			}
		case "dev":
			if i+1 < len(fields) {
				iface = fields[i+1]
			}
		}
	}
	if gateway == "" || iface == "" {
		return "", "", fmt.Errorf("no default route: this machine is offline")
	}
	return gateway, iface, nil
}
