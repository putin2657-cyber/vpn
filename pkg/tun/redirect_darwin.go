//go:build darwin

package tun

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"strings"
)

// The two halves of the address space outrank the existing default route
// without deleting it, so the original default is still there to fall back on.
var defaultHalves = []string{"0.0.0.0/1", "128.0.0.0/1"}

func redirect(dev *Device, serverIP net.IP, dns []string) (func(), error) {
	gateway, iface, err := defaultRoute()
	if err != nil {
		return nil, err
	}

	// Without this the tunnel would try to carry its own carrier connection.
	if err := run("route", "-q", "add", "-host", serverIP.String(), gateway); err != nil {
		return nil, fmt.Errorf("pin route to VPN server: %w", err)
	}
	undo := []func(){func() {
		_ = run("route", "-q", "delete", "-host", serverIP.String())
	}}

	rollback := func() {
		for i := len(undo) - 1; i >= 0; i-- {
			undo[i]()
		}
	}

	name := dev.Name()
	for _, half := range defaultHalves {
		if err := run("route", "-q", "add", "-net", half, "-interface", name); err != nil {
			rollback()
			return nil, fmt.Errorf("route %s via %s: %w", half, name, err)
		}
		undo = append(undo, func() {
			_ = run("route", "-q", "delete", "-net", half, "-interface", name)
		})
	}

	if len(dns) > 0 {
		service, previous, err := currentDNS(iface)
		if err != nil {
			log.Printf("could not read DNS settings, leaving them alone: %v", err)
		} else if err := setDNS(service, dns); err != nil {
			log.Printf("could not set DNS servers: %v", err)
		} else {
			undo = append(undo, func() {
				if err := setDNS(service, previous); err != nil {
					log.Printf("could not restore DNS servers on %q: %v", service, err)
				}
			})
		}
	}

	log.Printf("all traffic redirected through %s, server reachable via %s", name, gateway)
	return rollback, nil
}

func defaultRoute() (gateway, iface string, err error) {
	out, err := exec.Command("route", "-n", "get", "default").Output()
	if err != nil {
		return "", "", fmt.Errorf("look up default route: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if _, value, found := strings.Cut(line, "gateway:"); found {
			gateway = strings.TrimSpace(value)
		}
		if _, value, found := strings.Cut(line, "interface:"); found {
			iface = strings.TrimSpace(value)
		}
	}
	if gateway == "" || iface == "" {
		return "", "", fmt.Errorf("no default route: this machine is offline")
	}
	return gateway, iface, nil
}

// currentDNS maps a BSD interface name onto the network service that owns it,
// which is what networksetup works with, and reads its current servers.
func currentDNS(iface string) (service string, servers []string, err error) {
	service, err = networkService(iface)
	if err != nil {
		return "", nil, err
	}

	out, err := exec.Command("networksetup", "-getdnsservers", service).Output()
	if err != nil {
		return "", nil, fmt.Errorf("read DNS servers for %q: %w", service, err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if net.ParseIP(line) != nil {
			servers = append(servers, line)
		}
	}
	return service, servers, nil
}

// setDNS applies servers to a service; an empty list restores DHCP defaults.
func setDNS(service string, servers []string) error {
	args := append([]string{"-setdnsservers", service}, servers...)
	if len(servers) == 0 {
		args = append(args, "empty")
	}
	return run("networksetup", args...)
}

// networkService finds the service name, such as "Wi-Fi", for a device name.
func networkService(iface string) (string, error) {
	out, err := exec.Command("networksetup", "-listnetworkserviceorder").Output()
	if err != nil {
		return "", fmt.Errorf("list network services: %w", err)
	}

	// Output alternates between "(1) Wi-Fi" and
	// "(Hardware Port: Wi-Fi, Device: en0)".
	var lastService string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if name, found := parseServiceHeading(line); found {
			lastService = name
			continue
		}
		if strings.Contains(line, "Device: "+iface+")") && lastService != "" {
			return lastService, nil
		}
	}
	return "", fmt.Errorf("no network service uses %s", iface)
}

func parseServiceHeading(line string) (string, bool) {
	if !strings.HasPrefix(line, "(") || strings.HasPrefix(line, "(Hardware Port:") {
		return "", false
	}
	_, rest, found := strings.Cut(line, ")")
	if !found {
		return "", false
	}
	name := strings.TrimSpace(rest)
	if name == "" {
		return "", false
	}
	return name, true
}
