// Command admin manages the AmneziaWG deployment: it creates the server
// identity, issues peers and prints the configs users import into their apps.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"github.com/skip2/go-qrcode"

	"vpn-server/pkg/awg"
	"vpn-server/pkg/db"
)

var (
	dbPath = flag.String("db", "vpn.db", "path to the SQLite database")
	action = flag.String("action", "list",
		"action: init, add, list, remove, config, qr, server-config")

	name     = flag.String("name", "", "peer name for add, remove, config and qr")
	endpoint = flag.String("endpoint", "", "public address clients connect to, host:port (init)")
	external = flag.String("interface", "eth0", "server network interface facing the internet (init)")
	subnet   = flag.String("subnet", "10.8.0.0/24", "VPN subnet (init)")
	dns      = flag.String("dns", "1.1.1.1,1.0.0.1", "DNS servers handed to clients (init)")
	port     = flag.Int("port", awg.DefaultListenPort, "UDP port the server listens on (init)")
	mtu      = flag.Int("mtu", awg.DefaultMTU, "tunnel MTU (init)")
	mode     = flag.String("mode", string(awg.DefaultTunnelMode),
		"client tunnel mode for add/config/qr: smart (default) or full")
	out = flag.String("out", "", "write the config to this file instead of stdout")
)

func main() {
	flag.Parse()
	log.SetFlags(0)

	database, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer database.Close()

	if err := run(database); err != nil {
		if errors.Is(err, db.ErrNotInitialised) {
			log.Fatalf("%v\n\nCreate one first:\n"+
				"  admin -action init -endpoint vpn.example.com:51820 -interface eth0", err)
		}
		log.Fatalf("%v", err)
	}
}

func run(database *db.DB) error {
	switch *action {
	case "init":
		return initServer(database)
	case "add":
		return addPeer(database)
	case "list":
		return listPeers(database)
	case "remove":
		return removePeer(database)
	case "config":
		return showConfig(database)
	case "qr":
		return showQR(database)
	case "server-config":
		return showServerConfig(database)
	default:
		return fmt.Errorf("unknown action %q", *action)
	}
}

func initServer(database *db.DB) error {
	if existing, err := database.Server(); err == nil {
		return fmt.Errorf("server is already initialised with endpoint %s.\n"+
			"Re-initialising would invalidate every config already handed out",
			existing.Endpoint)
	} else if !errors.Is(err, db.ErrNotInitialised) {
		return err
	}

	if *endpoint == "" {
		return fmt.Errorf("-endpoint is required, for example vpn.example.com:51820")
	}

	_, network, err := net.ParseCIDR(*subnet)
	if err != nil {
		return fmt.Errorf("parse subnet %q: %w", *subnet, err)
	}
	gateway := firstHost(network)
	if gateway == nil {
		return fmt.Errorf("subnet %s has no usable host addresses", network)
	}

	keys, err := awg.GenerateKeypair()
	if err != nil {
		return err
	}
	obfuscation, err := awg.NewObfuscation()
	if err != nil {
		return err
	}

	server := &awg.Server{
		PrivateKey:        keys.Private,
		PublicKey:         keys.Public,
		Endpoint:          *endpoint,
		ListenPort:        *port,
		Address:           gateway,
		Subnet:            network,
		MTU:               *mtu,
		DNS:               splitList(*dns),
		ExternalInterface: *external,
		Obfuscation:       obfuscation,
	}
	if err := server.Validate(); err != nil {
		return err
	}
	if err := database.SaveServer(server); err != nil {
		return err
	}

	fmt.Println("Server initialised.")
	fmt.Printf("  Endpoint:   %s\n", server.Endpoint)
	fmt.Printf("  Public key: %s\n", server.PublicKey)
	fmt.Printf("  Subnet:     %s, server at %s\n", server.Subnet, server.Address)
	fmt.Printf("  Interface:  %s\n", server.ExternalInterface)
	fmt.Println("\nNext: add a peer, then write the server config to the VPS:")
	fmt.Println("  admin -action add -name alice-phone")
	fmt.Println("  admin -action server-config -out /etc/amnezia/amneziawg/awg0.conf")
	return nil
}

func addPeer(database *db.DB) error {
	if *name == "" {
		return fmt.Errorf("-name is required")
	}

	tunnelMode, err := awg.ParseTunnelMode(*mode)
	if err != nil {
		return err
	}

	peer, err := database.AddPeer(*name)
	if err != nil {
		return err
	}
	server, err := database.Server()
	if err != nil {
		return err
	}

	config, err := server.ClientConfig(toAWGPeer(peer), tunnelMode)
	if err != nil {
		return err
	}

	fmt.Printf("Peer %q added with address %s (mode %s)\n\n", peer.Name, peer.Address, tunnelMode)
	if err := emit(config); err != nil {
		return err
	}
	fmt.Println("\nApply the new peer on the server:")
	fmt.Println("  admin -action server-config -out /etc/amnezia/amneziawg/awg0.conf")
	fmt.Println("  systemctl restart awg-quick@awg0")
	return nil
}

func listPeers(database *db.DB) error {
	peers, err := database.ListPeers()
	if err != nil {
		return err
	}
	if len(peers) == 0 {
		fmt.Println("No peers yet. Add one with: admin -action add -name alice-phone")
		return nil
	}

	fmt.Printf("%-4s %-24s %-14s %-8s %-20s\n", "ID", "Name", "Address", "Enabled", "Created")
	fmt.Println(strings.Repeat("-", 76))
	for _, peer := range peers {
		enabled := "yes"
		if !peer.Enabled {
			enabled = "no"
		}
		fmt.Printf("%-4d %-24s %-14s %-8s %-20s\n", peer.ID, peer.Name, peer.Address, enabled, peer.Created)
	}
	return nil
}

func removePeer(database *db.DB) error {
	if *name == "" {
		return fmt.Errorf("-name is required")
	}
	if err := database.RemovePeer(*name); err != nil {
		return err
	}

	fmt.Printf("Peer %q removed.\n", *name)
	fmt.Println("It keeps working until you update the server:")
	fmt.Println("  admin -action server-config -out /etc/amnezia/amneziawg/awg0.conf")
	fmt.Println("  systemctl restart awg-quick@awg0")
	return nil
}

func showConfig(database *db.DB) error {
	tunnelMode, err := awg.ParseTunnelMode(*mode)
	if err != nil {
		return err
	}
	server, peer, err := load(database)
	if err != nil {
		return err
	}
	config, err := server.ClientConfig(peer, tunnelMode)
	if err != nil {
		return err
	}
	return emit(config)
}

// showQR prints the config as a QR code, which is how the mobile apps import it.
func showQR(database *db.DB) error {
	tunnelMode, err := awg.ParseTunnelMode(*mode)
	if err != nil {
		return err
	}
	server, peer, err := load(database)
	if err != nil {
		return err
	}
	config, err := server.ClientConfig(peer, tunnelMode)
	if err != nil {
		return err
	}

	code, err := qrcode.New(config, qrcode.Medium)
	if err != nil {
		return fmt.Errorf("build QR code: %w (config may be too large; use -action config -out peer.conf instead)", err)
	}
	fmt.Printf("Scan with the Amnezia app to add %q (%s):\n\n", *name, tunnelMode)
	fmt.Println(code.ToSmallString(false))
	return nil
}

func showServerConfig(database *db.DB) error {
	server, err := database.Server()
	if err != nil {
		return err
	}
	peers, err := database.AWGPeers()
	if err != nil {
		return err
	}

	config, err := server.ServerConfig(peers)
	if err != nil {
		return err
	}
	return emit(config)
}

func load(database *db.DB) (*awg.Server, awg.Peer, error) {
	if *name == "" {
		return nil, awg.Peer{}, fmt.Errorf("-name is required")
	}
	server, err := database.Server()
	if err != nil {
		return nil, awg.Peer{}, err
	}
	peer, err := database.GetPeer(*name)
	if err != nil {
		return nil, awg.Peer{}, err
	}
	return server, toAWGPeer(peer), nil
}

func toAWGPeer(peer *db.Peer) awg.Peer {
	return awg.Peer{
		Name:         peer.Name,
		PrivateKey:   peer.PrivateKey,
		PublicKey:    peer.PublicKey,
		PresharedKey: peer.PresharedKey,
		Address:      peer.Address,
	}
}

// emit writes a config to stdout, or to -out with private-key-appropriate
// permissions.
func emit(config string) error {
	if *out == "" {
		fmt.Print(config)
		return nil
	}
	if err := os.WriteFile(*out, []byte(config), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", *out, err)
	}
	fmt.Printf("Written to %s\n", *out)
	return nil
}

func firstHost(network *net.IPNet) net.IP {
	base := network.IP.To4()
	if base == nil {
		return nil
	}
	host := make(net.IP, net.IPv4len)
	copy(host, base)
	host[3]++
	if !network.Contains(host) {
		return nil
	}
	return host
}

func splitList(joined string) []string {
	var items []string
	for _, part := range strings.Split(joined, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}
