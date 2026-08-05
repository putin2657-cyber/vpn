package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vpn-server/pkg/db"
	"vpn-server/pkg/protocol"
	"vpn-server/pkg/routing"
)

// startTestServer brings the server up on a random port with a throwaway
// database. Server state lives in package globals, so each test resets them.
func startTestServer(t *testing.T) string {
	t.Helper()

	var err error
	database, err = db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	ipPool = &ipAllocator{base: net.ParseIP("10.8.0.0"), next: ipPoolFirst}
	router = routing.NewRoutingTable()

	ln, err := tls.Listen("tcp", "127.0.0.1:0", testTLSConfig(t))
	if err != nil {
		t.Fatalf("tls.Listen: %v", err)
	}
	t.Cleanup(func() {
		ln.Close()
		router.CloseAll()
		// Handlers touch these globals in their defers; the next test reassigns
		// them, so let the current ones drain first.
		waitForSessions(5 * time.Second)
	})

	go acceptLoop(ln)
	return ln.Addr().String()
}

func testTLSConfig(t *testing.T) *tls.Config {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "vpn-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
	}
}

func addTestUser(t *testing.T, name string) string {
	t.Helper()
	token := "token-" + name
	if _, err := database.AddUser(name, token, name+"@test.local"); err != nil {
		t.Fatalf("AddUser(%s): %v", name, err)
	}
	return token
}

type testClient struct {
	conn net.Conn
	ip   net.IP
}

func dialClient(t *testing.T, addr, token string) *testClient {
	t.Helper()

	conn, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("tls.Dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	if _, err := conn.Write([]byte(token + "\n")); err != nil {
		t.Fatalf("write token: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	line, err := readLine(conn)
	if err != nil {
		t.Fatalf("read auth response: %v", err)
	}
	ip, err := protocol.ParseAuthResponse(line)
	if err != nil {
		t.Fatalf("auth as %s: %v", token, err)
	}
	return &testClient{conn: conn, ip: ip.To4()}
}

// readPacket returns the next real packet, skipping keepalive frames.
func (c *testClient) readPacket(t *testing.T) []byte {
	t.Helper()
	c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		pkt, err := protocol.ReadPacket(c.conn)
		if err != nil {
			t.Fatalf("ReadPacket: %v", err)
		}
		if pkt != nil {
			return pkt
		}
	}
}

func readLine(r io.Reader) (string, error) {
	var line []byte
	buf := make([]byte, 1)
	for {
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		if buf[0] == '\n' {
			return string(line), nil
		}
		line = append(line, buf[0])
	}
}

func ipv4Packet(src, dst net.IP, payload string) []byte {
	pkt := make([]byte, 20, 20+len(payload))
	pkt[0] = 0x45
	copy(pkt[12:16], src.To4())
	copy(pkt[16:20], dst.To4())
	return append(pkt, payload...)
}

func TestClientsGetDistinctSubnetAddresses(t *testing.T) {
	addr := startTestServer(t)

	alice := dialClient(t, addr, addTestUser(t, "alice"))
	bob := dialClient(t, addr, addTestUser(t, "bob"))

	if alice.ip.String() != "10.8.0.2" {
		t.Errorf("alice got %s, want 10.8.0.2", alice.ip)
	}
	if bob.ip.String() != "10.8.0.3" {
		t.Errorf("bob got %s, want 10.8.0.3", bob.ip)
	}
}

func TestServerForwardsPacketBetweenClients(t *testing.T) {
	addr := startTestServer(t)

	alice := dialClient(t, addr, addTestUser(t, "alice"))
	bob := dialClient(t, addr, addTestUser(t, "bob"))

	sent := ipv4Packet(alice.ip, bob.ip, "hello-bob")
	if err := protocol.WritePacket(alice.conn, sent); err != nil {
		t.Fatalf("WritePacket: %v", err)
	}

	got := bob.readPacket(t)
	if !bytes.Equal(got, sent) {
		t.Fatalf("bob received %v, want %v", got, sent)
	}
}

// A client must not be able to inject traffic on someone else's address.
func TestServerDropsSpoofedSourceAddress(t *testing.T) {
	addr := startTestServer(t)

	alice := dialClient(t, addr, addTestUser(t, "alice"))
	bob := dialClient(t, addr, addTestUser(t, "bob"))

	spoofed := ipv4Packet(net.ParseIP("192.0.2.55"), bob.ip, "spoofed")
	if err := protocol.WritePacket(alice.conn, spoofed); err != nil {
		t.Fatalf("WritePacket(spoofed): %v", err)
	}

	// Sent second, so if the spoofed packet were forwarded it would arrive first.
	legit := ipv4Packet(alice.ip, bob.ip, "legit")
	if err := protocol.WritePacket(alice.conn, legit); err != nil {
		t.Fatalf("WritePacket(legit): %v", err)
	}

	got := bob.readPacket(t)
	if bytes.Equal(got, spoofed) {
		t.Fatal("server forwarded a packet with a spoofed source address")
	}
	if !bytes.Equal(got, legit) {
		t.Fatalf("bob received %v, want %v", got, legit)
	}
}

// Internet-bound traffic belongs on the server's TUN, never on a peer's
// connection. With egress off there is no TUN, so the packet is simply dropped.
func TestInternetBoundTrafficIsNotSentToPeers(t *testing.T) {
	addr := startTestServer(t)

	alice := dialClient(t, addr, addTestUser(t, "alice"))
	bob := dialClient(t, addr, addTestUser(t, "bob"))

	external := ipv4Packet(alice.ip, net.ParseIP("8.8.8.8"), "to-the-internet")
	if err := protocol.WritePacket(alice.conn, external); err != nil {
		t.Fatalf("WritePacket(external): %v", err)
	}

	legit := ipv4Packet(alice.ip, bob.ip, "to-bob")
	if err := protocol.WritePacket(alice.conn, legit); err != nil {
		t.Fatalf("WritePacket(legit): %v", err)
	}

	got := bob.readPacket(t)
	if bytes.Equal(got, external) {
		t.Fatal("internet-bound packet was delivered to another client")
	}
	if !bytes.Equal(got, legit) {
		t.Fatalf("bob received %v, want %v", got, legit)
	}
}

func TestKeepaliveDoesNotDisturbTheStream(t *testing.T) {
	addr := startTestServer(t)

	alice := dialClient(t, addr, addTestUser(t, "alice"))
	bob := dialClient(t, addr, addTestUser(t, "bob"))

	if err := protocol.WriteKeepalive(alice.conn); err != nil {
		t.Fatalf("WriteKeepalive: %v", err)
	}

	sent := ipv4Packet(alice.ip, bob.ip, "after-keepalive")
	if err := protocol.WritePacket(alice.conn, sent); err != nil {
		t.Fatalf("WritePacket: %v", err)
	}

	if got := bob.readPacket(t); !bytes.Equal(got, sent) {
		t.Fatalf("bob received %v, want %v", got, sent)
	}
}

func TestUnknownTokenIsRejected(t *testing.T) {
	addr := startTestServer(t)

	conn, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("tls.Dial: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("not-a-real-token\n")); err != nil {
		t.Fatalf("write token: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	line, err := readLine(conn)
	if err != nil {
		t.Fatalf("read auth response: %v", err)
	}
	if !strings.HasPrefix(line, "AUTH_FAIL") {
		t.Fatalf("got %q, want AUTH_FAIL", line)
	}
}

// The token line and the first packet often share one TCP segment.
func TestPacketPipelinedWithAuthTokenSurvives(t *testing.T) {
	addr := startTestServer(t)

	bob := dialClient(t, addr, addTestUser(t, "bob"))
	aliceToken := addTestUser(t, "alice")

	conn, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("tls.Dial: %v", err)
	}
	defer conn.Close()

	// Alice is next in the pool, so she can address her packet before the
	// server has confirmed the assignment.
	aliceIP := net.ParseIP("10.8.0.3")
	sent := ipv4Packet(aliceIP, bob.ip, "pipelined")

	var frame bytes.Buffer
	frame.WriteString(aliceToken + "\n")
	if err := protocol.WritePacket(&frame, sent); err != nil {
		t.Fatalf("WritePacket: %v", err)
	}
	if _, err := conn.Write(frame.Bytes()); err != nil {
		t.Fatalf("write: %v", err)
	}

	if got := bob.readPacket(t); !bytes.Equal(got, sent) {
		t.Fatalf("bob received %v, want %v", got, sent)
	}
}

func TestSessionIsLoggedOnConnectAndDisconnect(t *testing.T) {
	addr := startTestServer(t)

	client := dialClient(t, addr, addTestUser(t, "alice"))

	session := awaitSession(t, func(s *db.Session) bool {
		return s.Action == "connect" && s.IPAssigned == client.ip.String()
	})
	if session.Username != "alice" {
		t.Fatalf("session logged for %q, want alice", session.Username)
	}

	client.conn.Close()

	awaitSession(t, func(s *db.Session) bool {
		return s.ID == session.ID && s.Action == "disconnect" && s.DisconnectedAt != ""
	})
}

// awaitSession polls vpn_logs, since the server writes it from the connection
// goroutine rather than inline with the handshake.
func awaitSession(t *testing.T, match func(*db.Session) bool) *db.Session {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		sessions, err := database.ListSessions(50)
		if err != nil {
			t.Fatalf("ListSessions: %v", err)
		}
		for _, s := range sessions {
			if match(s) {
				return s
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no matching session appeared in vpn_logs")
	return nil
}
