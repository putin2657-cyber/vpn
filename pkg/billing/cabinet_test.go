package billing

import (
	"net"
	"path/filepath"
	"testing"

	"vpn-server/pkg/awg"
	"vpn-server/pkg/db"
)

func TestRegisterCreatesDisabledPeer(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	keys, err := awg.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	obf, err := awg.NewObfuscation()
	if err != nil {
		t.Fatal(err)
	}
	_, subnet, err := net.ParseCIDR("10.8.0.0/24")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SaveServer(&awg.Server{
		PrivateKey: keys.Private, PublicKey: keys.Public,
		Endpoint: "vpn.example.com:51820", ListenPort: 51820,
		Address: net.ParseIP("10.8.0.1"), Subnet: subnet, MTU: 1280,
		DNS: []string{"1.1.1.1"}, ExternalInterface: "eth0", Obfuscation: obf,
	}); err != nil {
		t.Fatal(err)
	}

	svc := &Service{DB: database, Config: Config{PublicBase: "https://example.com"}}
	user, session, err := svc.Register("mama@example.com", "password1", "Mama")
	if err != nil {
		t.Fatal(err)
	}
	if session.Token == "" || user.Email != "mama@example.com" {
		t.Fatalf("bad register result: %+v %+v", user, session)
	}
	member, err := database.GetMemberByUserID(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := database.GetPeer(member.PeerName)
	if err != nil {
		t.Fatal(err)
	}
	if peer.Enabled {
		t.Fatal("new self-registered peer must stay disabled until payment")
	}
	if IsOwnerPeer(member.PeerName) {
		t.Fatalf("allocated owner peer name %q", member.PeerName)
	}
}

func TestPeerNameFromEmail(t *testing.T) {
	if got := peerNameFromEmail("Anna.Ivanova@mail.ru"); got != "u-anna-ivanova" {
		t.Fatalf("got %q", got)
	}
}
