package protocol

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"testing"
)

func ipv4Packet(src, dst string) []byte {
	pkt := make([]byte, 20)
	pkt[0] = 0x45
	copy(pkt[12:16], net.ParseIP(src).To4())
	copy(pkt[16:20], net.ParseIP(dst).To4())
	return pkt
}

func TestPacketRoundTrip(t *testing.T) {
	want := ipv4Packet("10.8.0.2", "10.8.0.3")

	var buf bytes.Buffer
	if err := WritePacket(&buf, want); err != nil {
		t.Fatalf("WritePacket: %v", err)
	}

	got, err := ReadPacket(&buf)
	if err != nil {
		t.Fatalf("ReadPacket: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestKeepaliveReadsAsNilPacket(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteKeepalive(&buf); err != nil {
		t.Fatalf("WriteKeepalive: %v", err)
	}
	if buf.Len() != 4 {
		t.Fatalf("keepalive frame is %d bytes, want 4", buf.Len())
	}

	pkt, err := ReadPacket(&buf)
	if err != nil {
		t.Fatalf("ReadPacket: %v", err)
	}
	if pkt != nil {
		t.Fatalf("got packet %v, want nil for a keepalive", pkt)
	}
}

func TestReadPacketRejectsOversizedLength(t *testing.T) {
	var buf bytes.Buffer
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, MaxPacketSize+1)
	buf.Write(header)

	if _, err := ReadPacket(&buf); err == nil {
		t.Fatal("expected an error for a length above MaxPacketSize")
	}
}

// The server reads the token straight off the connection, so a packet that
// arrived in the same TCP segment must still be readable afterwards.
func TestReadAuthTokenDoesNotConsumeFollowingPacket(t *testing.T) {
	want := ipv4Packet("10.8.0.2", "10.8.0.3")

	var buf bytes.Buffer
	buf.WriteString("deadbeef\n")
	if err := WritePacket(&buf, want); err != nil {
		t.Fatalf("WritePacket: %v", err)
	}

	token, err := ReadAuthToken(&buf)
	if err != nil {
		t.Fatalf("ReadAuthToken: %v", err)
	}
	if token != "deadbeef" {
		t.Fatalf("got token %q, want %q", token, "deadbeef")
	}

	got, err := ReadPacket(&buf)
	if err != nil {
		t.Fatalf("ReadPacket after auth: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestReadAuthTokenRejectsUnboundedLine(t *testing.T) {
	r := strings.NewReader(strings.Repeat("a", maxAuthLineLen+10))
	if _, err := ReadAuthToken(r); err == nil {
		t.Fatal("expected an error for a line without a newline")
	}
}

func TestReadAuthTokenRejectsEmptyToken(t *testing.T) {
	if _, err := ReadAuthToken(strings.NewReader("   \n")); err == nil {
		t.Fatal("expected an error for a blank token")
	}
}

func TestReadAuthTokenOnClosedStream(t *testing.T) {
	if _, err := ReadAuthToken(bytes.NewReader(nil)); err != io.EOF {
		t.Fatalf("got %v, want io.EOF", err)
	}
}

func TestSrcAndDestIP(t *testing.T) {
	pkt := ipv4Packet("10.8.0.7", "10.8.0.9")

	src, ok := SrcIP(pkt)
	if !ok || src.String() != "10.8.0.7" {
		t.Fatalf("SrcIP = %v, %v; want 10.8.0.7, true", src, ok)
	}
	dst, ok := DestIP(pkt)
	if !ok || dst.String() != "10.8.0.9" {
		t.Fatalf("DestIP = %v, %v; want 10.8.0.9, true", dst, ok)
	}
}

func TestSrcAndDestIPRejectNonIPv4(t *testing.T) {
	short := make([]byte, 10)
	if _, ok := SrcIP(short); ok {
		t.Fatal("SrcIP accepted a truncated packet")
	}

	v6 := make([]byte, 20)
	v6[0] = 0x60
	if _, ok := DestIP(v6); ok {
		t.Fatal("DestIP accepted an IPv6 packet")
	}
}

func TestAuthConfigRoundTrip(t *testing.T) {
	want := Config{
		IP:      "10.8.0.4",
		Gateway: "10.8.0.1",
		Netmask: "255.255.255.0",
		MTU:     1400,
		DNS:     []string{"1.1.1.1", "1.0.0.1"},
	}

	var buf bytes.Buffer
	if err := WriteAuthOK(&buf, want); err != nil {
		t.Fatalf("WriteAuthOK: %v", err)
	}

	line, err := ReadAuthToken(&buf) // the response is also a single \n-terminated line
	if err != nil {
		t.Fatalf("read response line: %v", err)
	}
	got, err := ParseAuthResponse(line)
	if err != nil {
		t.Fatalf("ParseAuthResponse: %v", err)
	}

	if got.IP != want.IP || got.Gateway != want.Gateway || got.MTU != want.MTU {
		t.Fatalf("got %+v, want %+v", *got, want)
	}
	if got.Addr().String() != "10.8.0.4" {
		t.Fatalf("Addr() = %s, want 10.8.0.4", got.Addr())
	}
	if ones, _ := got.Mask().Size(); ones != 24 {
		t.Fatalf("Mask() is /%d, want /24", ones)
	}
	if len(got.DNS) != 2 {
		t.Fatalf("got %d DNS servers, want 2", len(got.DNS))
	}
}

func TestParseAuthResponseRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"auth failure":   "AUTH_FAIL",
		"no payload":     "OK",
		"not json":       "OK 10.8.0.4",
		"unknown status": "garbage",
		"bad ip":         `OK {"ip":"nope","gateway":"10.8.0.1","netmask":"255.255.255.0","mtu":1400}`,
		"bad gateway":    `OK {"ip":"10.8.0.4","gateway":"nope","netmask":"255.255.255.0","mtu":1400}`,
		"bad netmask":    `OK {"ip":"10.8.0.4","gateway":"10.8.0.1","netmask":"nope","mtu":1400}`,
		"missing mtu":    `OK {"ip":"10.8.0.4","gateway":"10.8.0.1","netmask":"255.255.255.0"}`,
		"absurd mtu":     `OK {"ip":"10.8.0.4","gateway":"10.8.0.1","netmask":"255.255.255.0","mtu":99999999}`,
		"truncated json": `OK {"ip":"10.8.0.4"`,
	}

	for name, raw := range cases {
		if _, err := ParseAuthResponse(raw); err == nil {
			t.Errorf("%s: expected an error for %q", name, raw)
		}
	}
}
