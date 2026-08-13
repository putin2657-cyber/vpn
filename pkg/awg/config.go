package awg

import (
	"fmt"
	"net"
	"strings"
)

// DefaultListenPort is AmneziaWG's usual UDP port.
const DefaultListenPort = 51820

// DefaultMTU stays under common mobile and CGNAT ceilings. 1420 still
// fragments on some LTE paths; 1280 is the safe floor that IPv6 also requires.
const DefaultMTU = 1280

// Server describes the interface running on the VPS.
type Server struct {
	PrivateKey Key
	PublicKey  Key

	// Endpoint is how clients reach the server, as host:port.
	Endpoint   string
	ListenPort int

	// Address is the server's own address inside the tunnel.
	Address net.IP
	Subnet  *net.IPNet
	MTU     int
	DNS     []string

	// ExternalInterface is the NIC that faces the internet, used for NAT.
	ExternalInterface string

	Obfuscation Obfuscation
}

// Peer is one client device.
type Peer struct {
	Name         string
	PrivateKey   Key
	PublicKey    Key
	PresharedKey Key
	Address      net.IP
}

func (s *Server) Validate() error {
	if s.PrivateKey.IsZero() {
		return fmt.Errorf("server private key is not set")
	}
	if s.Endpoint == "" {
		return fmt.Errorf("server endpoint is not set: clients would have nowhere to connect")
	}
	if s.Address == nil || s.Address.To4() == nil {
		return fmt.Errorf("server tunnel address is not a valid IPv4 address")
	}
	if s.Subnet == nil {
		return fmt.Errorf("VPN subnet is not set")
	}
	if s.ExternalInterface == "" {
		return fmt.Errorf("external interface is not set: NAT rules cannot be written")
	}
	return s.Obfuscation.Validate()
}

// ServerConfig renders awg0.conf, including the NAT rules that awg-quick runs
// when the interface comes up.
func (s *Server) ServerConfig(peers []Peer) (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}

	ones, _ := s.Subnet.Mask.Size()
	nic := s.ExternalInterface

	var b strings.Builder
	b.WriteString("[Interface]\n")
	fmt.Fprintf(&b, "PrivateKey = %s\n", s.PrivateKey)
	fmt.Fprintf(&b, "Address = %s/%d\n", s.Address, ones)
	fmt.Fprintf(&b, "ListenPort = %d\n", s.listenPort())
	fmt.Fprintf(&b, "MTU = %d\n", s.mtu())

	b.WriteString("\n")
	b.WriteString(s.Obfuscation.render())

	// Masquerading is what actually gives clients the internet; forwarding
	// rules let packets cross between the tunnel and the outside world.
	// awg0→awg0 covers peer-to-peer; the extra MASQUERADE for traffic that
	// hairpins back to our public address is what Telegram P2P needs when
	// two phones share one exit IP.
	b.WriteString("\n")
	fmt.Fprintf(&b, "PostUp = sysctl -w net.ipv4.ip_forward=1\n")
	fmt.Fprintf(&b, "PostUp = sysctl -w net.ipv4.conf.all.rp_filter=2\n")
	fmt.Fprintf(&b, "PostUp = iptables -t nat -A POSTROUTING -s %s -o %s -j MASQUERADE\n", s.Subnet, nic)
	fmt.Fprintf(&b, "PostUp = iptables -t nat -A POSTROUTING -s %s -d %s -j MASQUERADE\n", s.Subnet, s.Subnet)
	fmt.Fprintf(&b, "PostUp = iptables -A FORWARD -i %%i -o %%i -j ACCEPT\n")
	fmt.Fprintf(&b, "PostUp = iptables -A FORWARD -i %%i -o %s -j ACCEPT\n", nic)
	fmt.Fprintf(&b, "PostUp = iptables -A FORWARD -i %s -o %%i -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT\n", nic)
	fmt.Fprintf(&b, "PostDown = iptables -t nat -D POSTROUTING -s %s -o %s -j MASQUERADE\n", s.Subnet, nic)
	fmt.Fprintf(&b, "PostDown = iptables -t nat -D POSTROUTING -s %s -d %s -j MASQUERADE\n", s.Subnet, s.Subnet)
	fmt.Fprintf(&b, "PostDown = iptables -D FORWARD -i %%i -o %%i -j ACCEPT\n")
	fmt.Fprintf(&b, "PostDown = iptables -D FORWARD -i %%i -o %s -j ACCEPT\n", nic)
	fmt.Fprintf(&b, "PostDown = iptables -D FORWARD -i %s -o %%i -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT\n", nic)

	for _, peer := range peers {
		b.WriteString("\n")
		fmt.Fprintf(&b, "# %s\n", peer.Name)
		b.WriteString("[Peer]\n")
		fmt.Fprintf(&b, "PublicKey = %s\n", peer.PublicKey)
		if !peer.PresharedKey.IsZero() {
			fmt.Fprintf(&b, "PresharedKey = %s\n", peer.PresharedKey)
		}
		fmt.Fprintf(&b, "AllowedIPs = %s/32\n", peer.Address)
	}

	return b.String(), nil
}

// ClientConfig renders the file a user imports into their AmneziaWG client.
// mode defaults to TunnelSmart when empty.
func (s *Server) ClientConfig(peer Peer, mode TunnelMode) (string, error) {
	if mode == "" {
		mode = DefaultTunnelMode
	}
	if mode != TunnelSmart && mode != TunnelFull {
		return "", fmt.Errorf("unknown tunnel mode %q", mode)
	}
	if err := s.Validate(); err != nil {
		return "", err
	}
	if peer.PrivateKey.IsZero() {
		return "", fmt.Errorf("peer %q has no private key", peer.Name)
	}
	if peer.Address == nil || peer.Address.To4() == nil {
		return "", fmt.Errorf("peer %q has no valid tunnel address", peer.Name)
	}

	var b strings.Builder
	b.WriteString("[Interface]\n")
	fmt.Fprintf(&b, "PrivateKey = %s\n", peer.PrivateKey)
	fmt.Fprintf(&b, "Address = %s/32\n", peer.Address)
	// Smart mode also sets DNS: Apple/FaceTime hostnames must resolve via a
	// clean resolver when the ISP path is filtered; AllowedIPs then sends
	// Apple (17.0.0.0/8) through the tunnel.
	if len(s.DNS) > 0 {
		fmt.Fprintf(&b, "DNS = %s\n", strings.Join(s.DNS, ", "))
	}
	fmt.Fprintf(&b, "MTU = %d\n", s.mtu())

	b.WriteString("\n")
	b.WriteString(s.Obfuscation.render())

	b.WriteString("\n[Peer]\n")
	fmt.Fprintf(&b, "PublicKey = %s\n", s.PublicKey)
	if !peer.PresharedKey.IsZero() {
		fmt.Fprintf(&b, "PresharedKey = %s\n", peer.PresharedKey)
	}
	fmt.Fprintf(&b, "Endpoint = %s\n", s.Endpoint)
	fmt.Fprintf(&b, "AllowedIPs = %s\n", s.clientAllowedIPs(mode))
	// Keeps NAT mappings on mobile networks from expiring.
	b.WriteString("PersistentKeepalive = 25\n")

	return b.String(), nil
}

func (s *Server) clientAllowedIPs(mode TunnelMode) string {
	switch mode {
	case TunnelSmart:
		return strings.Join(SmartAllowedIPs(s.DNS), ", ")
	default:
		return "0.0.0.0/0, ::/0"
	}
}

func (s *Server) listenPort() int {
	if s.ListenPort == 0 {
		return DefaultListenPort
	}
	return s.ListenPort
}

func (s *Server) mtu() int {
	if s.MTU == 0 {
		return DefaultMTU
	}
	return s.MTU
}

// render writes the obfuscation block shared by both sides of the tunnel.
func (o Obfuscation) render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Jc = %d\n", o.JunkPacketCount)
	fmt.Fprintf(&b, "Jmin = %d\n", o.JunkPacketMinSize)
	fmt.Fprintf(&b, "Jmax = %d\n", o.JunkPacketMaxSize)
	fmt.Fprintf(&b, "S1 = %d\n", o.InitPacketJunkSize)
	fmt.Fprintf(&b, "S2 = %d\n", o.ResponsePacketJunkSize)
	fmt.Fprintf(&b, "H1 = %d\n", o.InitPacketMagic)
	fmt.Fprintf(&b, "H2 = %d\n", o.ResponsePacketMagic)
	fmt.Fprintf(&b, "H3 = %d\n", o.UnderloadPacketMagic)
	fmt.Fprintf(&b, "H4 = %d\n", o.TransportPacketMagic)
	return b.String()
}
