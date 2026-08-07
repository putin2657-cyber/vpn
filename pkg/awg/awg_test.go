package awg

import (
	"encoding/hex"
	"net"
	"strings"
	"testing"
)

func TestGenerateKeypairIsUsable(t *testing.T) {
	pair, err := GenerateKeypair()
	if err != nil {
		t.Fatalf("GenerateKeypair: %v", err)
	}

	if pair.Private.IsZero() || pair.Public.IsZero() {
		t.Fatal("keypair contains a zero key")
	}
	if pair.Private == pair.Public {
		t.Fatal("private and public keys are identical")
	}

	// wg-format keys are 32 bytes, which is 44 base64 characters.
	if got := len(pair.Public.String()); got != 44 {
		t.Fatalf("encoded key is %d characters, want 44", got)
	}
}

func TestPrivateKeyIsClamped(t *testing.T) {
	for range 20 {
		key, err := GeneratePrivateKey()
		if err != nil {
			t.Fatalf("GeneratePrivateKey: %v", err)
		}
		if key[0]&7 != 0 {
			t.Fatalf("low three bits of byte 0 are set: %08b", key[0])
		}
		if key[31]&128 != 0 {
			t.Fatalf("high bit of byte 31 is set: %08b", key[31])
		}
		if key[31]&64 == 0 {
			t.Fatalf("second-highest bit of byte 31 is clear: %08b", key[31])
		}
	}
}

func TestPublicKeyIsDeterministic(t *testing.T) {
	private, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}

	first, err := private.PublicKey()
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}
	second, err := private.PublicKey()
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}
	if first != second {
		t.Fatal("the same private key produced two different public keys")
	}
}

// Checks our derivation against the X25519 test vector in RFC 7748 §6.1, which
// is the same curve operation `wg pubkey` performs. If this passes, keys we
// generate are accepted by a stock WireGuard peer.
func TestPublicKeyMatchesRFC7748Vector(t *testing.T) {
	privateHex := "77076d0a7318a57d3c16c17251b26645df4c2f87ebc0992ab177fba51db92c2a"
	wantPublicHex := "8520f0098930a754748b7ddcb43ef75a0dbf3a0d26381af4eba4a98eaa9b4e6a"

	raw, err := hex.DecodeString(privateHex)
	if err != nil {
		t.Fatalf("decode private key: %v", err)
	}
	var private Key
	copy(private[:], raw)

	public, err := private.PublicKey()
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}
	if got := hex.EncodeToString(public[:]); got != wantPublicHex {
		t.Fatalf("derived %s, want %s", got, wantPublicHex)
	}
}

func TestKeysAreDistinctAcrossCalls(t *testing.T) {
	seen := make(map[string]bool)
	for range 50 {
		pair, err := GenerateKeypair()
		if err != nil {
			t.Fatalf("GenerateKeypair: %v", err)
		}
		if seen[pair.Private.String()] {
			t.Fatal("the same private key was generated twice")
		}
		seen[pair.Private.String()] = true
	}
}

func TestParseKeyRoundTrip(t *testing.T) {
	pair, err := GenerateKeypair()
	if err != nil {
		t.Fatalf("GenerateKeypair: %v", err)
	}

	parsed, err := ParseKey(pair.Public.String())
	if err != nil {
		t.Fatalf("ParseKey: %v", err)
	}
	if parsed != pair.Public {
		t.Fatal("key changed when encoded and parsed back")
	}
}

func TestParseKeyRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"not base64":   "!!!not-base64!!!",
		"wrong length": "c2hvcnQ=",
		"empty":        "",
	}
	for name, encoded := range cases {
		if _, err := ParseKey(encoded); err == nil {
			t.Errorf("%s: expected an error for %q", name, encoded)
		}
	}
}

func TestNewObfuscationIsValid(t *testing.T) {
	for range 50 {
		obfuscation, err := NewObfuscation()
		if err != nil {
			t.Fatalf("NewObfuscation: %v", err)
		}
		if err := obfuscation.Validate(); err != nil {
			t.Fatalf("generated set is invalid: %v\n%+v", err, obfuscation)
		}
	}
}

func TestObfuscationValidateCatchesBadParameters(t *testing.T) {
	valid, err := NewObfuscation()
	if err != nil {
		t.Fatalf("NewObfuscation: %v", err)
	}

	cases := map[string]func(*Obfuscation){
		"junk count too low":      func(o *Obfuscation) { o.JunkPacketCount = 0 },
		"junk count too high":     func(o *Obfuscation) { o.JunkPacketCount = 129 },
		"min above max":           func(o *Obfuscation) { o.JunkPacketMinSize = o.JunkPacketMaxSize + 1 },
		"max beyond MTU":          func(o *Obfuscation) { o.JunkPacketMaxSize = 5000 },
		"colliding sizes":         func(o *Obfuscation) { o.ResponsePacketJunkSize = o.InitPacketJunkSize + 56 },
		"magic in reserved range": func(o *Obfuscation) { o.InitPacketMagic = 3 },
		"duplicate magics":        func(o *Obfuscation) { o.ResponsePacketMagic = o.InitPacketMagic },
	}

	for name, breakIt := range cases {
		broken := valid
		breakIt(&broken)
		if err := broken.Validate(); err == nil {
			t.Errorf("%s: expected Validate to reject the set", name)
		}
	}
}

func testServer(t *testing.T) (*Server, Peer) {
	t.Helper()

	serverKeys, err := GenerateKeypair()
	if err != nil {
		t.Fatalf("server keypair: %v", err)
	}
	peerKeys, err := GenerateKeypair()
	if err != nil {
		t.Fatalf("peer keypair: %v", err)
	}
	psk, err := GeneratePresharedKey()
	if err != nil {
		t.Fatalf("preshared key: %v", err)
	}
	obfuscation, err := NewObfuscation()
	if err != nil {
		t.Fatalf("obfuscation: %v", err)
	}

	_, subnet, err := net.ParseCIDR("10.8.0.0/24")
	if err != nil {
		t.Fatalf("parse subnet: %v", err)
	}

	server := &Server{
		PrivateKey:        serverKeys.Private,
		PublicKey:         serverKeys.Public,
		Endpoint:          "vpn.example.com:51820",
		Address:           net.ParseIP("10.8.0.1"),
		Subnet:            subnet,
		DNS:               []string{"1.1.1.1", "1.0.0.1"},
		ExternalInterface: "eth0",
		Obfuscation:       obfuscation,
	}
	peer := Peer{
		Name:         "alice-phone",
		PrivateKey:   peerKeys.Private,
		PublicKey:    peerKeys.Public,
		PresharedKey: psk,
		Address:      net.ParseIP("10.8.0.2"),
	}
	return server, peer
}

func TestServerConfigContainsPeersAndNAT(t *testing.T) {
	server, peer := testServer(t)

	config, err := server.ServerConfig([]Peer{peer})
	if err != nil {
		t.Fatalf("ServerConfig: %v", err)
	}

	mustContain(t, config, []string{
		"[Interface]",
		"PrivateKey = " + server.PrivateKey.String(),
		"Address = 10.8.0.1/24",
		"ListenPort = 51820",
		"MASQUERADE",
		"net.ipv4.ip_forward=1",
		"[Peer]",
		"PublicKey = " + peer.PublicKey.String(),
		"PresharedKey = " + peer.PresharedKey.String(),
		"AllowedIPs = 10.8.0.2/32",
	})

	// The server's own private key belongs there, but a peer's never does.
	if strings.Contains(config, peer.PrivateKey.String()) {
		t.Fatal("server config leaks a peer private key")
	}
}

func TestClientConfigFullModeRoutesEverything(t *testing.T) {
	server, peer := testServer(t)

	config, err := server.ClientConfig(peer, TunnelFull)
	if err != nil {
		t.Fatalf("ClientConfig: %v", err)
	}

	mustContain(t, config, []string{
		"PrivateKey = " + peer.PrivateKey.String(),
		"Address = 10.8.0.2/32",
		"DNS = 1.1.1.1, 1.0.0.1",
		"MTU = 1280",
		"PublicKey = " + server.PublicKey.String(),
		"Endpoint = vpn.example.com:51820",
		"AllowedIPs = 0.0.0.0/0, ::/0",
		"PersistentKeepalive = 25",
	})

	if strings.Contains(config, server.PrivateKey.String()) {
		t.Fatal("client config leaks the server private key")
	}
}

func TestClientConfigSmartModeKeepsLocalServicesDirect(t *testing.T) {
	server, peer := testServer(t)

	config, err := server.ClientConfig(peer, TunnelSmart)
	if err != nil {
		t.Fatalf("ClientConfig: %v", err)
	}

	if strings.Contains(config, "0.0.0.0/0") || strings.Contains(config, "::/0") {
		t.Fatal("smart mode must not capture the whole internet")
	}
	mustContain(t, config, []string{
		"1.1.1.1/32",
		"1.0.0.1/32",
		"142.250.0.0/15",
		"157.240.0.0/16",
		"PersistentKeepalive = 25",
	})
}

func TestSmartRoutesAreValidCIDRs(t *testing.T) {
	if err := ValidateSmartRoutes(); err != nil {
		t.Fatal(err)
	}
}

func TestParseTunnelMode(t *testing.T) {
	smart, err := ParseTunnelMode("")
	if err != nil || smart != TunnelSmart {
		t.Fatalf("empty mode: got %q, %v", smart, err)
	}
	full, err := ParseTunnelMode("full")
	if err != nil || full != TunnelFull {
		t.Fatalf("full mode: got %q, %v", full, err)
	}
	if _, err := ParseTunnelMode("bogus"); err == nil {
		t.Fatal("expected error for unknown mode")
	}
}

// Both ends must agree on every obfuscation value or the handshake fails.
func TestObfuscationMatchesOnBothSides(t *testing.T) {
	server, peer := testServer(t)

	serverConfig, err := server.ServerConfig([]Peer{peer})
	if err != nil {
		t.Fatalf("ServerConfig: %v", err)
	}
	clientConfig, err := server.ClientConfig(peer, TunnelSmart)
	if err != nil {
		t.Fatalf("ClientConfig: %v", err)
	}

	for _, line := range strings.Split(server.Obfuscation.render(), "\n") {
		if line == "" {
			continue
		}
		if !strings.Contains(serverConfig, line) {
			t.Errorf("server config is missing %q", line)
		}
		if !strings.Contains(clientConfig, line) {
			t.Errorf("client config is missing %q", line)
		}
	}
}

func TestServerConfigRejectsIncompleteServer(t *testing.T) {
	cases := map[string]func(*Server){
		"no endpoint":           func(s *Server) { s.Endpoint = "" },
		"no external interface": func(s *Server) { s.ExternalInterface = "" },
		"no private key":        func(s *Server) { s.PrivateKey = Key{} },
		"no address":            func(s *Server) { s.Address = nil },
		"bad obfuscation":       func(s *Server) { s.Obfuscation.JunkPacketCount = 0 },
	}

	for name, breakIt := range cases {
		server, peer := testServer(t)
		breakIt(server)
		if _, err := server.ServerConfig([]Peer{peer}); err == nil {
			t.Errorf("%s: expected ServerConfig to fail", name)
		}
	}
}

func mustContain(t *testing.T, config string, wanted []string) {
	t.Helper()
	for _, want := range wanted {
		if !strings.Contains(config, want) {
			t.Errorf("config is missing %q\n---\n%s", want, config)
		}
	}
}
