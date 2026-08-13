package awg

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// DefaultInterface is the usual AmneziaWG interface name on the VPS.
const DefaultInterface = "awg0"

// PeerRuntime is live state from `awg show`.
type PeerRuntime struct {
	PublicKey      string
	Endpoint       string
	AllowedIPs     string
	LastHandshake  time.Time
	RxBytes        uint64
	TxBytes        uint64
	HandshakeHuman string
}

// AddPeerLive installs one peer into a running interface without restarting it.
func AddPeerLive(iface string, peer Peer) error {
	if iface == "" {
		iface = DefaultInterface
	}
	args := []string{"set", iface, "peer", peer.PublicKey.String(), "allowed-ips", peer.Address.String() + "/32"}
	if !peer.PresharedKey.IsZero() {
		tmp, err := os.CreateTemp("", "awg-psk-*")
		if err != nil {
			return err
		}
		path := tmp.Name()
		defer os.Remove(path)
		if _, err := tmp.WriteString(peer.PresharedKey.String() + "\n"); err != nil {
			tmp.Close()
			return err
		}
		if err := tmp.Close(); err != nil {
			return err
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return err
		}
		args = append(args, "preshared-key", path)
	}
	return runAWG(args...)
}

// RemovePeerLive drops a peer from a running interface by public key.
func RemovePeerLive(iface, publicKey string) error {
	if iface == "" {
		iface = DefaultInterface
	}
	return runAWG("set", iface, "peer", publicKey, "remove")
}

// ShowPeers returns runtime stats keyed by public key.
func ShowPeers(iface string) (map[string]PeerRuntime, error) {
	if iface == "" {
		iface = DefaultInterface
	}
	out, err := exec.Command("awg", "show", iface, "dump").Output()
	if err != nil {
		return nil, fmt.Errorf("awg show %s dump: %w", iface, err)
	}
	return parseShowDump(out)
}

func parseShowDump(raw []byte) (map[string]PeerRuntime, error) {
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	result := make(map[string]PeerRuntime)
	for _, line := range lines {
		fields := strings.Split(line, "\t")
		// AmneziaWG interface rows carry obfuscation columns and are long.
		// Peer rows are: pubkey, psk, endpoint, allowed-ips, handshake, rx, tx, keepalive.
		if len(fields) < 8 || len(fields) > 10 {
			continue
		}
		hs, _ := strconv.ParseInt(fields[4], 10, 64)
		rx, _ := strconv.ParseUint(fields[5], 10, 64)
		tx, _ := strconv.ParseUint(fields[6], 10, 64)
		var when time.Time
		human := "never"
		if hs > 0 {
			when = time.Unix(hs, 0)
			human = formatAgo(time.Since(when))
		}
		endpoint := fields[2]
		if endpoint == "(none)" {
			endpoint = ""
		}
		pk := fields[0]
		result[pk] = PeerRuntime{
			PublicKey:      pk,
			Endpoint:       endpoint,
			AllowedIPs:     fields[3],
			LastHandshake:  when,
			RxBytes:        rx,
			TxBytes:        tx,
			HandshakeHuman: human,
		}
	}
	return result, nil
}

func runAWG(args ...string) error {
	cmd := exec.Command("awg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("awg %s: %s", strings.Join(args, " "), msg)
	}
	return nil
}

// AWGAvailable reports whether the awg binary is on PATH.
func AWGAvailable() bool {
	_, err := exec.LookPath("awg")
	return err == nil
}

func formatAgo(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	sec := int(d.Seconds())
	switch {
	case sec < 60:
		return fmt.Sprintf("%ds ago", sec)
	case sec < 3600:
		return fmt.Sprintf("%dm ago", sec/60)
	case sec < 86400:
		return fmt.Sprintf("%dh %dm ago", sec/3600, (sec%3600)/60)
	default:
		return fmt.Sprintf("%dd ago", sec/86400)
	}
}

// FormatBytes is a short human size for transfer counters.
func FormatBytes(n uint64) string {
	const (
		kib = 1024
		mib = kib * 1024
		gib = mib * 1024
	)
	switch {
	case n >= gib:
		return fmt.Sprintf("%.2f GiB", float64(n)/float64(gib))
	case n >= mib:
		return fmt.Sprintf("%.2f MiB", float64(n)/float64(mib))
	case n >= kib:
		return fmt.Sprintf("%.1f KiB", float64(n)/float64(kib))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
