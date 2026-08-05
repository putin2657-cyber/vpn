package protocol

import (
	"encoding/binary"
	"encoding/json"
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

// Config is the tunnel setup the server hands a client once it is authorised.
// It travels as one JSON line so fields can be added without a new handshake.
type Config struct {
	IP      string   `json:"ip"`
	Gateway string   `json:"gateway"`
	Netmask string   `json:"netmask"`
	MTU     int      `json:"mtu"`
	DNS     []string `json:"dns"`
}

// Addr returns the address assigned to the client.
func (c *Config) Addr() net.IP {
	return net.ParseIP(c.IP).To4()
}

// Mask returns the tunnel netmask.
func (c *Config) Mask() net.IPMask {
	return net.IPMask(net.ParseIP(c.Netmask).To4())
}

// GatewayAddr returns the server's address inside the tunnel.
func (c *Config) GatewayAddr() net.IP {
	return net.ParseIP(c.Gateway).To4()
}

// WriteAuthOK sends the tunnel configuration after successful authentication.
func WriteAuthOK(w io.Writer, cfg Config) error {
	body, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "OK %s\n", body)
	return err
}

// WriteAuthFail rejects authentication.
func WriteAuthFail(w io.Writer) error {
	_, err := io.WriteString(w, "AUTH_FAIL\n")
	return err
}

// ParseAuthResponse parses `OK {json}` or `AUTH_FAIL`.
func ParseAuthResponse(raw string) (*Config, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "AUTH_FAIL") {
		return nil, fmt.Errorf("authentication failed")
	}
	payload, ok := strings.CutPrefix(raw, "OK ")
	if !ok {
		return nil, fmt.Errorf("unexpected auth response: %q", raw)
	}

	var cfg Config
	if err := json.Unmarshal([]byte(payload), &cfg); err != nil {
		return nil, fmt.Errorf("malformed tunnel config %q: %w", payload, err)
	}
	if cfg.Addr() == nil {
		return nil, fmt.Errorf("invalid assigned IP: %q", cfg.IP)
	}
	if cfg.GatewayAddr() == nil {
		return nil, fmt.Errorf("invalid gateway: %q", cfg.Gateway)
	}
	if cfg.Mask() == nil {
		return nil, fmt.Errorf("invalid netmask: %q", cfg.Netmask)
	}
	if cfg.MTU < 576 || cfg.MTU > MaxPacketSize {
		return nil, fmt.Errorf("implausible MTU: %d", cfg.MTU)
	}
	return &cfg, nil
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
