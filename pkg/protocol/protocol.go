package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
)

const (
	MaxPacketSize = 65535
	// maxAuthLineLen bounds the auth line so a peer cannot make us buffer
	// without limit before it sends a newline.
	maxAuthLineLen = 512
)

// keepaliveFrame is a zero-length frame: a no-op that refreshes idle deadlines.
var keepaliveFrame = []byte{0, 0, 0, 0}

// WritePacket sends a length-prefixed IP packet over the TLS stream.
func WritePacket(w io.Writer, pkt []byte) error {
	if len(pkt) == 0 {
		return fmt.Errorf("empty packet")
	}
	if len(pkt) > MaxPacketSize {
		return fmt.Errorf("packet too large: %d", len(pkt))
	}
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(pkt)))
	if _, err := w.Write(header); err != nil {
		return err
	}
	_, err := w.Write(pkt)
	return err
}

// WriteKeepalive sends a zero-length frame to keep an idle tunnel alive.
func WriteKeepalive(w io.Writer) error {
	_, err := w.Write(keepaliveFrame)
	return err
}

// ReadPacket reads a length-prefixed IP packet from the TLS stream.
// A keepalive frame yields a nil packet and a nil error.
func ReadPacket(r io.Reader) ([]byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(header)
	if length == 0 {
		return nil, nil
	}
	if length > MaxPacketSize {
		return nil, fmt.Errorf("invalid packet length: %d", length)
	}
	pkt := make([]byte, length)
	if _, err := io.ReadFull(r, pkt); err != nil {
		return nil, err
	}
	return pkt, nil
}

// ReadAuthToken reads the first-line authentication token. It reads one byte at
// a time on purpose: a buffered reader would swallow packet bytes that arrived
// in the same segment as the token line.
func ReadAuthToken(r io.Reader) (string, error) {
	line := make([]byte, 0, 64)
	buf := make([]byte, 1)
	for len(line) < maxAuthLineLen {
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		if buf[0] == '\n' {
			token := strings.TrimSpace(string(line))
			if token == "" {
				return "", fmt.Errorf("empty auth token")
			}
			return token, nil
		}
		line = append(line, buf[0])
	}
	return "", fmt.Errorf("auth token exceeds %d bytes", maxAuthLineLen)
}

// WriteAuthOK sends assigned IP after successful authentication.
func WriteAuthOK(w io.Writer, ip net.IP) error {
	_, err := fmt.Fprintf(w, "OK %s\n", ip.String())
	return err
}

// WriteAuthFail rejects authentication.
func WriteAuthFail(w io.Writer) error {
	_, err := io.WriteString(w, "AUTH_FAIL\n")
	return err
}

// ParseAuthResponse parses "OK 10.8.0.2" or "AUTH_FAIL".
func ParseAuthResponse(raw string) (net.IP, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "AUTH_FAIL") {
		return nil, fmt.Errorf("authentication failed")
	}
	if !strings.HasPrefix(raw, "OK") {
		return nil, fmt.Errorf("unexpected auth response: %q", raw)
	}
	parts := strings.Fields(raw)
	if len(parts) < 2 {
		return nil, fmt.Errorf("missing assigned IP in auth response")
	}
	ip := net.ParseIP(parts[1])
	if ip == nil {
		return nil, fmt.Errorf("invalid assigned IP: %q", parts[1])
	}
	return ip, nil
}

// SrcIP extracts the IPv4 source from a raw IP packet.
func SrcIP(pkt []byte) (net.IP, bool) {
	if !isIPv4(pkt) {
		return nil, false
	}
	return net.IPv4(pkt[12], pkt[13], pkt[14], pkt[15]), true
}

// DestIP extracts IPv4 destination from a raw IP packet.
func DestIP(pkt []byte) (net.IP, bool) {
	if !isIPv4(pkt) {
		return nil, false
	}
	return net.IPv4(pkt[16], pkt[17], pkt[18], pkt[19]), true
}

func isIPv4(pkt []byte) bool {
	return len(pkt) >= 20 && pkt[0]>>4 == 4
}
