package awg

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// Obfuscation holds the AmneziaWG parameters that stop the tunnel from looking
// like WireGuard on the wire. They are not a secret, but every peer has to use
// exactly the values the server uses or the handshake never completes.
type Obfuscation struct {
	// JunkPacketCount (Jc) is how many junk packets precede the handshake.
	JunkPacketCount int
	// JunkPacketMinSize (Jmin) and JunkPacketMaxSize (Jmax) bound their size.
	JunkPacketMinSize int
	JunkPacketMaxSize int
	// InitPacketJunkSize (S1) and ResponsePacketJunkSize (S2) pad the two
	// handshake packets so their length no longer identifies WireGuard.
	InitPacketJunkSize     int
	ResponsePacketJunkSize int
	// The four magics (H1..H4) replace WireGuard's fixed message type values.
	InitPacketMagic      uint32
	ResponsePacketMagic  uint32
	UnderloadPacketMagic uint32
	TransportPacketMagic uint32
}

// WireGuard uses message types 1 to 4, so the magics have to avoid that range
// or the traffic is trivially identified again.
const reservedMessageTypes = 4

// maxJunkSize keeps junk packets inside a normal MTU.
const maxJunkSize = 1280

// NewObfuscation produces a fresh random parameter set for one deployment.
func NewObfuscation() (Obfuscation, error) {
	junkCount, err := randomInt(3, 10)
	if err != nil {
		return Obfuscation{}, err
	}
	initJunk, err := randomInt(15, 150)
	if err != nil {
		return Obfuscation{}, err
	}
	responseJunk, err := randomInt(15, 150)
	if err != nil {
		return Obfuscation{}, err
	}
	// S1 + 56 == S2 would make the padded init packet the same length as a
	// response packet, which reintroduces a fingerprint.
	if initJunk+56 == responseJunk {
		responseJunk++
	}

	magics, err := randomMagics()
	if err != nil {
		return Obfuscation{}, err
	}

	obfuscation := Obfuscation{
		JunkPacketCount:        junkCount,
		JunkPacketMinSize:      8,
		JunkPacketMaxSize:      80,
		InitPacketJunkSize:     initJunk,
		ResponsePacketJunkSize: responseJunk,
		InitPacketMagic:        magics[0],
		ResponsePacketMagic:    magics[1],
		UnderloadPacketMagic:   magics[2],
		TransportPacketMagic:   magics[3],
	}
	if err := obfuscation.Validate(); err != nil {
		return Obfuscation{}, fmt.Errorf("generated an invalid parameter set: %w", err)
	}
	return obfuscation, nil
}

// randomMagics returns four distinct values outside WireGuard's reserved range.
func randomMagics() ([4]uint32, error) {
	var magics [4]uint32
	seen := make(map[uint32]bool, len(magics))

	for i := range magics {
		for {
			value, err := randomInt(reservedMessageTypes+1, 1<<31)
			if err != nil {
				return magics, err
			}
			candidate := uint32(value)
			if !seen[candidate] {
				seen[candidate] = true
				magics[i] = candidate
				break
			}
		}
	}
	return magics, nil
}

func (o Obfuscation) Validate() error {
	if o.JunkPacketCount < 1 || o.JunkPacketCount > 128 {
		return fmt.Errorf("Jc is %d, must be between 1 and 128", o.JunkPacketCount)
	}
	if o.JunkPacketMinSize < 1 || o.JunkPacketMinSize >= o.JunkPacketMaxSize {
		return fmt.Errorf("Jmin (%d) must be positive and below Jmax (%d)",
			o.JunkPacketMinSize, o.JunkPacketMaxSize)
	}
	if o.JunkPacketMaxSize > maxJunkSize {
		return fmt.Errorf("Jmax is %d, must not exceed %d", o.JunkPacketMaxSize, maxJunkSize)
	}
	if o.InitPacketJunkSize < 1 || o.InitPacketJunkSize > maxJunkSize {
		return fmt.Errorf("S1 is %d, must be between 1 and %d", o.InitPacketJunkSize, maxJunkSize)
	}
	if o.ResponsePacketJunkSize < 1 || o.ResponsePacketJunkSize > maxJunkSize {
		return fmt.Errorf("S2 is %d, must be between 1 and %d", o.ResponsePacketJunkSize, maxJunkSize)
	}
	if o.InitPacketJunkSize+56 == o.ResponsePacketJunkSize {
		return fmt.Errorf("S1 + 56 must not equal S2, otherwise the padded packets collide")
	}

	magics := map[string]uint32{
		"H1": o.InitPacketMagic,
		"H2": o.ResponsePacketMagic,
		"H3": o.UnderloadPacketMagic,
		"H4": o.TransportPacketMagic,
	}
	seen := make(map[uint32]string, len(magics))
	for name, value := range magics {
		if value <= reservedMessageTypes {
			return fmt.Errorf("%s is %d, must be above %d", name, value, reservedMessageTypes)
		}
		if other, duplicate := seen[value]; duplicate {
			return fmt.Errorf("%s and %s are both %d, all magics must differ", other, name, value)
		}
		seen[value] = name
	}
	return nil
}

// randomInt returns a value in [min, max].
func randomInt(min, max int) (int, error) {
	span := big.NewInt(int64(max - min + 1))
	offset, err := rand.Int(rand.Reader, span)
	if err != nil {
		return 0, fmt.Errorf("read random number: %w", err)
	}
	return min + int(offset.Int64()), nil
}
