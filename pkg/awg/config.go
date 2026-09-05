package awg

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

// DefaultListenPort is AmneziaWG's usual UDP port.
const DefaultListenPort = 51820

// DefaultMTU stays under common mobile and CGNAT ceilings.
const DefaultMTU = 1280

// androidExcludedApps stay on the ISP path inside AmneziaWG/WireGuard on Android.
// Package names are ignored by clients that do not support the key.
var androidExcludedApps = []string{
	"com.wildberries.ru",
	"ru.ozon.app.android",
	"ru.sberbankmobile",
	"com.idamob.tinkoff.android",
	"ru.vtb24.mobilebanking.android",
	"ru.alfabank.mobile.android",
	"logo.com.mbanking",
	"ru.rostel",
	"ru.yandex.androidkeyboard",
	"ru.yandex.searchplugin",
	"com.yandex.browser",
	"ru.yandex.yandexmaps",
	"ru.yandex.taxi",
	"com.yandex.mobile.auth.sdk",
	"com.valvesoftware.android.steam.community",
}

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
	// QUIC (UDP/443) over WireGuard often works for ~30–60s then collapses on
	// mobile paths. Force HTTPS to TCP so YouTube/TikTok stay stable.
	fmt.Fprintf(&b, "PostUp = iptables -I FORWARD 1 -i %%i -p udp --dport 443 -j DROP\n")
	// Clamp MSS so TCP sessions do not silently stall after a few minutes on
	// mobile CGNAT / broken PMTUD paths (Telegram "connecting…", etc.).
	fmt.Fprintf(&b, "PostUp = iptables -t mangle -A FORWARD -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu\n")
	fmt.Fprintf(&b, "PostUp = tc qdisc replace dev %s root fq_codel\n", nic)
	fmt.Fprintf(&b, "PostDown = iptables -t nat -D POSTROUTING -s %s -o %s -j MASQUERADE\n", s.Subnet, nic)
	fmt.Fprintf(&b, "PostDown = iptables -t nat -D POSTROUTING -s %s -d %s -j MASQUERADE\n", s.Subnet, s.Subnet)
	fmt.Fprintf(&b, "PostDown = iptables -D FORWARD -i %%i -o %%i -j ACCEPT\n")
	fmt.Fprintf(&b, "PostDown = iptables -D FORWARD -i %%i -o %s -j ACCEPT\n", nic)
	fmt.Fprintf(&b, "PostDown = iptables -D FORWARD -i %s -o %%i -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT\n", nic)
	fmt.Fprintf(&b, "PostDown = iptables -D FORWARD -i %%i -p udp --dport 443 -j DROP\n")
	fmt.Fprintf(&b, "PostDown = iptables -t mangle -D FORWARD -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu\n")

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
	// Android AmneziaWG/WireGuard: keep marketplace & bank apps on the ISP
	// even if a destination IP is missing from the smart exclude list.
	fmt.Fprintf(&b, "ExcludedApplications = %s\n", strings.Join(androidExcludedApps, ", "))

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
	b.WriteString("PersistentKeepalive = 15\n")

	return b.String(), nil
}

func (s *Server) clientAllowedIPs(mode TunnelMode) string {
	switch mode {
	case TunnelSmart:
		// Full IPv4 tunnel minus a tiny Steam/Dota bypass list so games use the
		// ISP while Discord stays on the VPN. IPv6 stays off (no VPS egress).
		return steamBypassAllowedIPs(s.DNS)
	default:
		return fullIPv4AllowedIPs(s.DNS)
	}
}

func steamBypassAllowedIPs(dns []string) string {
	allowed, err := subtractCIDRs("0.0.0.0/0", steamBypassCIDRs)
	if err != nil || len(allowed) == 0 {
		return fullIPv4AllowedIPs(dns)
	}
	allowed = collapseCIDRStrings(allowed)
	seen := make(map[string]struct{}, len(allowed)+len(dns)+2)
	out := make([]string, 0, len(allowed)+len(dns)+2)
	add := func(cidr string) {
		if _, ok := seen[cidr]; ok {
			return
		}
		seen[cidr] = struct{}{}
		out = append(out, cidr)
	}
	for _, server := range dns {
		server = strings.TrimSpace(server)
		ip := net.ParseIP(server)
		if ip == nil || ip.To4() == nil {
			continue
		}
		add(ip.To4().String() + "/32")
	}
	// Broadest prefixes first so mobile truncation keeps most of the internet.
	sort.SliceStable(allowed, func(i, j int) bool {
		return prefixLen(allowed[i]) < prefixLen(allowed[j])
	})
	for _, c := range allowed {
		add(c)
	}
	return strings.Join(out, ", ")
}

func fullIPv4AllowedIPs(dns []string) string {
	seen := map[string]struct{}{"0.0.0.0/0": {}}
	out := []string{"0.0.0.0/0"}
	for _, server := range dns {
		server = strings.TrimSpace(server)
		ip := net.ParseIP(server)
		if ip == nil || ip.To4() == nil {
			continue
		}
		cidr := ip.To4().String() + "/32"
		if _, ok := seen[cidr]; ok {
			continue
		}
		seen[cidr] = struct{}{}
		out = append(out, cidr)
	}
	return strings.Join(out, ", ")
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
