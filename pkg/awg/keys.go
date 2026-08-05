// Package awg builds AmneziaWG server and client configuration. AmneziaWG is
// WireGuard with extra junk traffic and randomised header magics, which is what
// keeps the handshake from being recognised and throttled by DPI.
package awg

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/curve25519"
)

// KeyLen is the size of every WireGuard key: Curve25519 keys and preshared keys alike.
const KeyLen = 32

// Key is a WireGuard key in its raw form; it prints as standard base64.
type Key [KeyLen]byte

func (k Key) String() string {
	return base64.StdEncoding.EncodeToString(k[:])
}

// IsZero reports whether the key was never set, which matters because an unset
// preshared key must be left out of a config rather than written as zeroes.
func (k Key) IsZero() bool {
	return k == Key{}
}

func ParseKey(encoded string) (Key, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return Key{}, fmt.Errorf("key is not valid base64: %w", err)
	}
	if len(raw) != KeyLen {
		return Key{}, fmt.Errorf("key is %d bytes, want %d", len(raw), KeyLen)
	}

	var key Key
	copy(key[:], raw)
	return key, nil
}

// GeneratePrivateKey returns a clamped Curve25519 private key, matching what
// `wg genkey` produces.
func GeneratePrivateKey() (Key, error) {
	var key Key
	if _, err := rand.Read(key[:]); err != nil {
		return Key{}, fmt.Errorf("read random bytes: %w", err)
	}

	// Clamping required by Curve25519: clear the low three bits, clear the top
	// bit and set the second-highest.
	key[0] &= 248
	key[31] &= 127
	key[31] |= 64

	return key, nil
}

// PublicKey derives the public half of a private key.
func (k Key) PublicKey() (Key, error) {
	derived, err := curve25519.X25519(k[:], curve25519.Basepoint)
	if err != nil {
		return Key{}, fmt.Errorf("derive public key: %w", err)
	}

	var public Key
	copy(public[:], derived)
	return public, nil
}

// GeneratePresharedKey returns an extra symmetric secret shared by one peer and
// the server. It is optional in WireGuard but cheap, so every peer gets one.
func GeneratePresharedKey() (Key, error) {
	var key Key
	if _, err := rand.Read(key[:]); err != nil {
		return Key{}, fmt.Errorf("read random bytes: %w", err)
	}
	return key, nil
}

// Keypair is a private key together with its derived public key.
type Keypair struct {
	Private Key
	Public  Key
}

func GenerateKeypair() (Keypair, error) {
	private, err := GeneratePrivateKey()
	if err != nil {
		return Keypair{}, err
	}
	public, err := private.PublicKey()
	if err != nil {
		return Keypair{}, err
	}
	return Keypair{Private: private, Public: public}, nil
}
