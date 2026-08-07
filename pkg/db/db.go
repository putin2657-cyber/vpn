// Package db stores the AmneziaWG deployment: the server's own identity and
// the peers allowed to connect.
package db

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strings"

	_ "github.com/mattn/go-sqlite3"

	"vpn-server/pkg/awg"
)

// ErrNotInitialised means no server identity exists yet; run the init action.
var ErrNotInitialised = errors.New("server is not initialised")

// ErrPeerNotFound is returned when no peer matches the requested name.
var ErrPeerNotFound = errors.New("peer not found")

// Peer is one client device together with the keys handed to it.
//
// Keeping a peer's private key on the server is a deliberate trade-off: it lets
// the admin regenerate a config or QR code at any time, at the cost of the
// database being enough to impersonate any peer. For a handful of people run by
// one admin that is the usual arrangement, but the file deserves care.
type Peer struct {
	ID           int
	Name         string
	PrivateKey   awg.Key
	PublicKey    awg.Key
	PresharedKey awg.Key
	Address      net.IP
	Enabled      bool
	Created      string
}

type DB struct {
	conn *sql.DB
}

func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	// The admin CLI and the API may both hold the file open.
	if _, err := conn.Exec("PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON;"); err != nil {
		conn.Close()
		return nil, err
	}

	db := &DB{conn: conn}
	if err := db.init(); err != nil {
		conn.Close()
		return nil, err
	}
	return db, nil
}

func (db *DB) init() error {
	schema := `
	CREATE TABLE IF NOT EXISTS server (
		id                 INTEGER PRIMARY KEY CHECK (id = 1),
		private_key        TEXT NOT NULL,
		public_key         TEXT NOT NULL,
		endpoint           TEXT NOT NULL,
		listen_port        INTEGER NOT NULL,
		address            TEXT NOT NULL,
		subnet             TEXT NOT NULL,
		mtu                INTEGER NOT NULL,
		dns                TEXT NOT NULL,
		external_interface TEXT NOT NULL,
		jc   INTEGER NOT NULL, jmin INTEGER NOT NULL, jmax INTEGER NOT NULL,
		s1   INTEGER NOT NULL, s2   INTEGER NOT NULL,
		h1   INTEGER NOT NULL, h2   INTEGER NOT NULL,
		h3   INTEGER NOT NULL, h4   INTEGER NOT NULL,
		created DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS peers (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		name          TEXT UNIQUE NOT NULL,
		private_key   TEXT NOT NULL,
		public_key    TEXT UNIQUE NOT NULL,
		preshared_key TEXT NOT NULL,
		address       TEXT UNIQUE NOT NULL,
		enabled       INTEGER NOT NULL DEFAULT 1,
		created       DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS members (
		id                 INTEGER PRIMARY KEY AUTOINCREMENT,
		display_name       TEXT NOT NULL,
		peer_name          TEXT NOT NULL UNIQUE,
		token              TEXT NOT NULL UNIQUE,
		payment_method_id  TEXT NOT NULL DEFAULT '',
		status             TEXT NOT NULL DEFAULT 'invited',
		paid_until         TEXT NOT NULL DEFAULT '',
		created            DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(peer_name) REFERENCES peers(name) ON DELETE CASCADE
	);
	CREATE TABLE IF NOT EXISTS payments (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		member_id     INTEGER NOT NULL,
		yk_payment_id TEXT NOT NULL UNIQUE,
		amount_value  TEXT NOT NULL,
		status        TEXT NOT NULL,
		kind          TEXT NOT NULL,
		created       DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(member_id) REFERENCES members(id) ON DELETE CASCADE
	);
	`
	if _, err := db.conn.Exec(schema); err != nil {
		return err
	}
	return db.ensurePeerEnabledColumn()
}

func (db *DB) ensurePeerEnabledColumn() error {
	rows, err := db.conn.Query(`PRAGMA table_info(peers)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	hasEnabled := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == "enabled" {
			hasEnabled = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if hasEnabled {
		return nil
	}
	_, err = db.conn.Exec(`ALTER TABLE peers ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1`)
	return err
}

// SaveServer stores the server identity. There is only ever one row: replacing
// it would invalidate every client config already handed out.
func (db *DB) SaveServer(s *awg.Server) error {
	ones, _ := s.Subnet.Mask.Size()
	o := s.Obfuscation

	_, err := db.conn.Exec(`
		INSERT OR REPLACE INTO server (
			id, private_key, public_key, endpoint, listen_port, address, subnet,
			mtu, dns, external_interface, jc, jmin, jmax, s1, s2, h1, h2, h3, h4
		) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.PrivateKey.String(), s.PublicKey.String(), s.Endpoint, s.ListenPort,
		s.Address.String(), fmt.Sprintf("%s/%d", s.Subnet.IP, ones),
		s.MTU, strings.Join(s.DNS, ","), s.ExternalInterface,
		o.JunkPacketCount, o.JunkPacketMinSize, o.JunkPacketMaxSize,
		o.InitPacketJunkSize, o.ResponsePacketJunkSize,
		o.InitPacketMagic, o.ResponsePacketMagic,
		o.UnderloadPacketMagic, o.TransportPacketMagic,
	)
	return err
}

// Server loads the stored identity, or ErrNotInitialised if there is none.
func (db *DB) Server() (*awg.Server, error) {
	var (
		privateKey, publicKey, endpoint string
		address, subnet, dns, external  string
		listenPort, mtu                 int
		o                               awg.Obfuscation
	)

	err := db.conn.QueryRow(`
		SELECT private_key, public_key, endpoint, listen_port, address, subnet,
		       mtu, dns, external_interface, jc, jmin, jmax, s1, s2, h1, h2, h3, h4
		FROM server WHERE id = 1`,
	).Scan(&privateKey, &publicKey, &endpoint, &listenPort, &address, &subnet,
		&mtu, &dns, &external,
		&o.JunkPacketCount, &o.JunkPacketMinSize, &o.JunkPacketMaxSize,
		&o.InitPacketJunkSize, &o.ResponsePacketJunkSize,
		&o.InitPacketMagic, &o.ResponsePacketMagic,
		&o.UnderloadPacketMagic, &o.TransportPacketMagic)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotInitialised
	}
	if err != nil {
		return nil, err
	}

	private, err := awg.ParseKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("stored server private key: %w", err)
	}
	public, err := awg.ParseKey(publicKey)
	if err != nil {
		return nil, fmt.Errorf("stored server public key: %w", err)
	}
	_, network, err := net.ParseCIDR(subnet)
	if err != nil {
		return nil, fmt.Errorf("stored subnet %q: %w", subnet, err)
	}

	return &awg.Server{
		PrivateKey:        private,
		PublicKey:         public,
		Endpoint:          endpoint,
		ListenPort:        listenPort,
		Address:           net.ParseIP(address),
		Subnet:            network,
		MTU:               mtu,
		DNS:               splitDNS(dns),
		ExternalInterface: external,
		Obfuscation:       o,
	}, nil
}

func splitDNS(joined string) []string {
	if joined == "" {
		return nil
	}
	parts := strings.Split(joined, ",")
	servers := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			servers = append(servers, trimmed)
		}
	}
	return servers
}

// AddPeer generates a fresh keypair, assigns the lowest free address in the
// subnet and stores the result.
func (db *DB) AddPeer(name string) (*Peer, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("peer name is required")
	}

	server, err := db.Server()
	if err != nil {
		return nil, err
	}

	address, err := db.nextFreeAddress(server)
	if err != nil {
		return nil, err
	}
	keys, err := awg.GenerateKeypair()
	if err != nil {
		return nil, err
	}
	preshared, err := awg.GeneratePresharedKey()
	if err != nil {
		return nil, err
	}

	result, err := db.conn.Exec(`
		INSERT INTO peers (name, private_key, public_key, preshared_key, address, enabled)
		VALUES (?, ?, ?, ?, ?, 1)`,
		name, keys.Private.String(), keys.Public.String(),
		preshared.String(), address.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("add peer %q: %w", name, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &Peer{
		ID:           int(id),
		Name:         name,
		PrivateKey:   keys.Private,
		PublicKey:    keys.Public,
		PresharedKey: preshared,
		Address:      address,
		Enabled:      true,
	}, nil
}

// nextFreeAddress returns the lowest host address in the subnet that is neither
// the server's nor already taken by a peer.
func (db *DB) nextFreeAddress(server *awg.Server) (net.IP, error) {
	taken := map[string]bool{server.Address.String(): true}

	rows, err := db.conn.Query("SELECT address FROM peers")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var address string
		if err := rows.Scan(&address); err != nil {
			return nil, err
		}
		taken[address] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	base := server.Subnet.IP.To4()
	if base == nil {
		return nil, fmt.Errorf("subnet %s is not IPv4", server.Subnet)
	}
	ones, bits := server.Subnet.Mask.Size()
	// The network and broadcast addresses are not usable hosts.
	hostCount := 1<<(bits-ones) - 2

	for offset := 1; offset <= hostCount; offset++ {
		candidate := make(net.IP, net.IPv4len)
		copy(candidate, base)
		if err := addOffset(candidate, offset); err != nil {
			return nil, err
		}
		if !taken[candidate.String()] {
			return candidate, nil
		}
	}
	return nil, fmt.Errorf("subnet %s is full", server.Subnet)
}

func addOffset(ip net.IP, offset int) error {
	value := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
	value += uint32(offset)
	ip[0] = byte(value >> 24)
	ip[1] = byte(value >> 16)
	ip[2] = byte(value >> 8)
	ip[3] = byte(value)
	return nil
}

func (db *DB) ListPeers() ([]*Peer, error) {
	rows, err := db.conn.Query(`
		SELECT id, name, private_key, public_key, preshared_key, address, enabled, created
		FROM peers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var peers []*Peer
	for rows.Next() {
		peer, err := scanPeer(rows)
		if err != nil {
			return nil, err
		}
		peers = append(peers, peer)
	}
	return peers, rows.Err()
}

func (db *DB) GetPeer(name string) (*Peer, error) {
	row := db.conn.QueryRow(`
		SELECT id, name, private_key, public_key, preshared_key, address, enabled, created
		FROM peers WHERE name = ?`, name)

	peer, err := scanPeer(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrPeerNotFound, name)
	}
	return peer, err
}

// SetPeerEnabled toggles whether a peer is written into awg0.conf.
func (db *DB) SetPeerEnabled(name string, enabled bool) error {
	flag := 0
	if enabled {
		flag = 1
	}
	result, err := db.conn.Exec(`UPDATE peers SET enabled = ? WHERE name = ?`, flag, name)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("%w: %s", ErrPeerNotFound, name)
	}
	return nil
}

// RemovePeer deletes a peer; its address becomes available again.
func (db *DB) RemovePeer(name string) error {
	result, err := db.conn.Exec("DELETE FROM peers WHERE name = ?", name)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("%w: %s", ErrPeerNotFound, name)
	}
	return nil
}

// AWGPeers returns enabled peers in the form the config renderer expects.
// Disabled (unpaid) peers are omitted so they cannot handshake.
func (db *DB) AWGPeers() ([]awg.Peer, error) {
	stored, err := db.ListPeers()
	if err != nil {
		return nil, err
	}

	peers := make([]awg.Peer, 0, len(stored))
	for _, p := range stored {
		if !p.Enabled {
			continue
		}
		peers = append(peers, awg.Peer{
			Name:         p.Name,
			PrivateKey:   p.PrivateKey,
			PublicKey:    p.PublicKey,
			PresharedKey: p.PresharedKey,
			Address:      p.Address,
		})
	}
	return peers, nil
}

// scanner covers both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanPeer(src scanner) (*Peer, error) {
	var (
		peer                             Peer
		private, public, preshared, addr string
		enabled                          int
	)
	err := src.Scan(&peer.ID, &peer.Name, &private, &public, &preshared, &addr, &enabled, &peer.Created)
	if err != nil {
		return nil, err
	}

	if peer.PrivateKey, err = awg.ParseKey(private); err != nil {
		return nil, fmt.Errorf("peer %q private key: %w", peer.Name, err)
	}
	if peer.PublicKey, err = awg.ParseKey(public); err != nil {
		return nil, fmt.Errorf("peer %q public key: %w", peer.Name, err)
	}
	if peer.PresharedKey, err = awg.ParseKey(preshared); err != nil {
		return nil, fmt.Errorf("peer %q preshared key: %w", peer.Name, err)
	}
	peer.Address = net.ParseIP(addr)
	if peer.Address == nil {
		return nil, fmt.Errorf("peer %q has an invalid address %q", peer.Name, addr)
	}
	peer.Enabled = enabled != 0
	return &peer, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}
