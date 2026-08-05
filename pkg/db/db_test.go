package db

import (
	"errors"
	"net"
	"path/filepath"
	"testing"

	"vpn-server/pkg/awg"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()

	database, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func testServer(t *testing.T, cidr string) *awg.Server {
	t.Helper()

	keys, err := awg.GenerateKeypair()
	if err != nil {
		t.Fatalf("GenerateKeypair: %v", err)
	}
	obfuscation, err := awg.NewObfuscation()
	if err != nil {
		t.Fatalf("NewObfuscation: %v", err)
	}
	_, subnet, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatalf("ParseCIDR: %v", err)
	}

	gateway := subnet.IP.To4()
	address := net.IPv4(gateway[0], gateway[1], gateway[2], gateway[3]+1)

	return &awg.Server{
		PrivateKey:        keys.Private,
		PublicKey:         keys.Public,
		Endpoint:          "vpn.example.com:51820",
		ListenPort:        51820,
		Address:           address,
		Subnet:            subnet,
		MTU:               1420,
		DNS:               []string{"1.1.1.1", "1.0.0.1"},
		ExternalInterface: "eth0",
		Obfuscation:       obfuscation,
	}
}

func TestServerReportsNotInitialised(t *testing.T) {
	database := openTestDB(t)

	if _, err := database.Server(); !errors.Is(err, ErrNotInitialised) {
		t.Fatalf("got %v, want ErrNotInitialised", err)
	}
}

// The obfuscation values and keys must survive a restart, or every config
// already handed out stops working.
func TestServerSurvivesRoundTrip(t *testing.T) {
	database := openTestDB(t)
	want := testServer(t, "10.8.0.0/24")

	if err := database.SaveServer(want); err != nil {
		t.Fatalf("SaveServer: %v", err)
	}
	got, err := database.Server()
	if err != nil {
		t.Fatalf("Server: %v", err)
	}

	if got.PrivateKey != want.PrivateKey || got.PublicKey != want.PublicKey {
		t.Error("keys changed across the round trip")
	}
	if got.Endpoint != want.Endpoint || got.ListenPort != want.ListenPort {
		t.Errorf("endpoint is %s:%d, want %s:%d",
			got.Endpoint, got.ListenPort, want.Endpoint, want.ListenPort)
	}
	if got.Subnet.String() != want.Subnet.String() {
		t.Errorf("subnet is %s, want %s", got.Subnet, want.Subnet)
	}
	if !got.Address.Equal(want.Address) {
		t.Errorf("address is %s, want %s", got.Address, want.Address)
	}
	if got.Obfuscation != want.Obfuscation {
		t.Errorf("obfuscation changed:\n got %+v\nwant %+v", got.Obfuscation, want.Obfuscation)
	}
	if len(got.DNS) != 2 || got.DNS[0] != "1.1.1.1" {
		t.Errorf("DNS is %v, want [1.1.1.1 1.0.0.1]", got.DNS)
	}
}

func TestAddPeerAssignsSequentialAddresses(t *testing.T) {
	database := openTestDB(t)
	if err := database.SaveServer(testServer(t, "10.8.0.0/24")); err != nil {
		t.Fatalf("SaveServer: %v", err)
	}

	// .1 belongs to the server, so peers start at .2.
	want := []string{"10.8.0.2", "10.8.0.3", "10.8.0.4"}
	for i, name := range []string{"alice", "bob", "carol"} {
		peer, err := database.AddPeer(name)
		if err != nil {
			t.Fatalf("AddPeer(%s): %v", name, err)
		}
		if peer.Address.String() != want[i] {
			t.Errorf("%s got %s, want %s", name, peer.Address, want[i])
		}
		if peer.PrivateKey.IsZero() || peer.PublicKey.IsZero() || peer.PresharedKey.IsZero() {
			t.Errorf("%s is missing one of its keys", name)
		}
	}
}

func TestAddPeerRejectsDuplicateName(t *testing.T) {
	database := openTestDB(t)
	if err := database.SaveServer(testServer(t, "10.8.0.0/24")); err != nil {
		t.Fatalf("SaveServer: %v", err)
	}

	if _, err := database.AddPeer("alice"); err != nil {
		t.Fatalf("AddPeer: %v", err)
	}
	if _, err := database.AddPeer("alice"); err == nil {
		t.Fatal("expected the second alice to be rejected")
	}
}

func TestAddPeerRequiresInitialisedServer(t *testing.T) {
	database := openTestDB(t)

	if _, err := database.AddPeer("alice"); !errors.Is(err, ErrNotInitialised) {
		t.Fatalf("got %v, want ErrNotInitialised", err)
	}
}

func TestRemovePeerFreesItsAddress(t *testing.T) {
	database := openTestDB(t)
	if err := database.SaveServer(testServer(t, "10.8.0.0/24")); err != nil {
		t.Fatalf("SaveServer: %v", err)
	}

	for _, name := range []string{"alice", "bob", "carol"} {
		if _, err := database.AddPeer(name); err != nil {
			t.Fatalf("AddPeer(%s): %v", name, err)
		}
	}
	if err := database.RemovePeer("bob"); err != nil {
		t.Fatalf("RemovePeer: %v", err)
	}

	reused, err := database.AddPeer("dave")
	if err != nil {
		t.Fatalf("AddPeer(dave): %v", err)
	}
	if got := reused.Address.String(); got != "10.8.0.3" {
		t.Fatalf("dave got %s, want bob's freed 10.8.0.3", got)
	}
}

func TestRemoveAndGetReportMissingPeer(t *testing.T) {
	database := openTestDB(t)
	if err := database.SaveServer(testServer(t, "10.8.0.0/24")); err != nil {
		t.Fatalf("SaveServer: %v", err)
	}

	if err := database.RemovePeer("ghost"); !errors.Is(err, ErrPeerNotFound) {
		t.Errorf("RemovePeer: got %v, want ErrPeerNotFound", err)
	}
	if _, err := database.GetPeer("ghost"); !errors.Is(err, ErrPeerNotFound) {
		t.Errorf("GetPeer: got %v, want ErrPeerNotFound", err)
	}
}

func TestGetPeerReturnsStoredKeys(t *testing.T) {
	database := openTestDB(t)
	if err := database.SaveServer(testServer(t, "10.8.0.0/24")); err != nil {
		t.Fatalf("SaveServer: %v", err)
	}

	added, err := database.AddPeer("alice")
	if err != nil {
		t.Fatalf("AddPeer: %v", err)
	}
	loaded, err := database.GetPeer("alice")
	if err != nil {
		t.Fatalf("GetPeer: %v", err)
	}

	if loaded.PrivateKey != added.PrivateKey || loaded.PublicKey != added.PublicKey {
		t.Error("keys changed between adding and loading")
	}
	if !loaded.Address.Equal(added.Address) {
		t.Errorf("address is %s, want %s", loaded.Address, added.Address)
	}
}

// A /29 leaves .1 for the server and .2 to .6 for peers.
func TestSubnetExhaustionIsReported(t *testing.T) {
	database := openTestDB(t)
	if err := database.SaveServer(testServer(t, "10.9.0.0/29")); err != nil {
		t.Fatalf("SaveServer: %v", err)
	}

	for i := range 5 {
		if _, err := database.AddPeer(string(rune('a' + i))); err != nil {
			t.Fatalf("peer %d should fit: %v", i, err)
		}
	}
	if _, err := database.AddPeer("one-too-many"); err == nil {
		t.Fatal("expected the subnet to be reported as full")
	}
}

func TestAWGPeersMirrorsStoredPeers(t *testing.T) {
	database := openTestDB(t)
	if err := database.SaveServer(testServer(t, "10.8.0.0/24")); err != nil {
		t.Fatalf("SaveServer: %v", err)
	}
	if _, err := database.AddPeer("alice"); err != nil {
		t.Fatalf("AddPeer: %v", err)
	}

	peers, err := database.AWGPeers()
	if err != nil {
		t.Fatalf("AWGPeers: %v", err)
	}
	if len(peers) != 1 {
		t.Fatalf("got %d peers, want 1", len(peers))
	}
	if peers[0].Name != "alice" || peers[0].PublicKey.IsZero() {
		t.Fatalf("unexpected peer %+v", peers[0])
	}
}
