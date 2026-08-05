package main

import (
	"crypto/tls"
	"errors"
	"flag"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"vpn-server/pkg/db"
	"vpn-server/pkg/protocol"
	"vpn-server/pkg/routing"
)

var (
	addr    = flag.String("addr", ":8443", "server listen address")
	certPem = flag.String("cert", "server.crt", "TLS certificate file")
	keyPem  = flag.String("key", "server.key", "TLS key file")
	dbPath  = flag.String("db", "vpn.db", "SQLite database path")
)

const (
	// authTimeout bounds how long a fresh connection may stay silent before
	// sending its token.
	authTimeout = 10 * time.Second
	// idleTimeout drops peers that send nothing at all, not even a keepalive.
	idleTimeout = 90 * time.Second
	// keepaliveInterval must stay well below the peer's idleTimeout.
	keepaliveInterval = 30 * time.Second
	writeTimeout      = 10 * time.Second
)

// Client is one authenticated VPN session.
type Client struct {
	conn net.Conn
	ip   net.IP
	user *db.User

	writeMu sync.Mutex
}

// WritePacket serialises writes to the connection. The routing table hands the
// same client to every peer's goroutine, and interleaved frames from concurrent
// writers would desynchronise the length-prefixed stream.
func (c *Client) WritePacket(pkt []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return err
	}
	return protocol.WritePacket(c.conn, pkt)
}

func (c *Client) WriteKeepalive() error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return err
	}
	return protocol.WriteKeepalive(c.conn)
}

func (c *Client) Close() error {
	return c.conn.Close()
}

var (
	ipPool   = &ipAllocator{base: net.ParseIP("10.8.0.0"), next: ipPoolFirst}
	router   = routing.NewRoutingTable()
	database *db.DB
	sessions sync.WaitGroup
)

func main() {
	flag.Parse()
	log.Printf("Starting VPN server on %s", *addr)

	var err error
	database, err = db.Open(*dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	cert, err := tls.LoadX509KeyPair(*certPem, *keyPem)
	if err != nil {
		log.Fatalf("failed loading cert: %v", err)
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.NoClientCert,
	}

	ln, err := tls.Listen("tcp", *addr, tlsCfg)
	if err != nil {
		log.Fatalf("listen failed: %v", err)
	}
	defer ln.Close()

	log.Printf("Server listening on %s, database: %s", *addr, *dbPath)

	// Runs before the deferred database.Close(), so in-flight handlers can still
	// write their disconnect rows.
	defer waitForSessions(5 * time.Second)

	go awaitShutdown(ln)

	acceptLoop(ln)
}

func acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				log.Println("listener closed, server stopped")
				return
			}
			log.Printf("accept error: %v", err)
			continue
		}
		go handleConn(conn)
	}
}

// awaitShutdown stops accepting and hangs up on live clients so that their
// session rows get closed instead of being left open until the next restart.
func awaitShutdown(ln net.Listener) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Printf("shutdown requested, disconnecting %d client(s)", router.Count())
	ln.Close()
	router.CloseAll()
}

func waitForSessions(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		sessions.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		log.Println("timed out waiting for sessions to close")
	}
}

func handleConn(c net.Conn) {
	sessions.Add(1)
	defer sessions.Done()
	defer c.Close()

	remote := c.RemoteAddr().String()
	log.Printf("new conn from %s", remote)
	enableTCPKeepalive(c)

	if err := c.SetReadDeadline(time.Now().Add(authTimeout)); err != nil {
		log.Printf("set auth deadline for %s: %v", remote, err)
		return
	}
	token, err := protocol.ReadAuthToken(c)
	if err != nil {
		if !errors.Is(err, io.EOF) {
			log.Printf("read token error from %s: %v", remote, err)
		}
		return
	}

	_ = c.SetWriteDeadline(time.Now().Add(writeTimeout))

	user, err := database.GetUser(token)
	if err != nil {
		log.Printf("auth failed from %s: invalid token", remote)
		_ = protocol.WriteAuthFail(c)
		return
	}

	ip := ipPool.Allocate()
	if ip == nil {
		log.Printf("address pool exhausted, rejecting user=%s", user.Username)
		_ = protocol.WriteAuthFail(c)
		return
	}
	defer ipPool.Release(ip)

	// Send the assignment before registering, so no forwarded packet can land
	// in front of the auth line the client is still parsing.
	if err := protocol.WriteAuthOK(c, ip); err != nil {
		log.Printf("failed to send auth OK to %s: %v", user.Username, err)
		return
	}

	client := &Client{conn: c, ip: ip, user: user}
	router.RegisterClient(ip, client)
	defer func() {
		router.UnregisterClient(ip)
		log.Printf("client disconnected: user=%s ip=%s", user.Username, ip)
	}()

	sessionID, err := database.StartSession(user.ID, ip.String())
	if err != nil {
		log.Printf("failed to record session start for %s: %v", user.Username, err)
	}
	defer func() {
		if sessionID == 0 {
			return
		}
		if err := database.EndSession(sessionID); err != nil {
			log.Printf("failed to record session end for %s: %v", user.Username, err)
		}
	}()

	log.Printf("client authorized: user=%s assigned IP=%s", user.Username, ip)

	stop := make(chan struct{})
	defer close(stop)
	go keepaliveLoop(client, stop)

	client.forward()
}

// forward relays packets from this client to their destination inside the VPN
// subnet until the connection drops or goes idle.
func (c *Client) forward() {
	for {
		if err := c.conn.SetReadDeadline(time.Now().Add(idleTimeout)); err != nil {
			return
		}
		pkt, err := protocol.ReadPacket(c.conn)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Printf("read packet error from %s: %v", c.user.Username, err)
			}
			return
		}
		if pkt == nil {
			continue // keepalive
		}

		srcIP, ok := protocol.SrcIP(pkt)
		if !ok {
			continue
		}
		if !srcIP.Equal(c.ip) {
			log.Printf("dropping spoofed packet from %s: src=%s, assigned=%s",
				c.user.Username, srcIP, c.ip)
			continue
		}

		destIP, ok := protocol.DestIP(pkt)
		if !ok {
			continue
		}
		target := router.Lookup(destIP)
		if target == nil {
			continue
		}
		if err := target.WritePacket(pkt); err != nil {
			log.Printf("forward packet to %s failed: %v", destIP, err)
		}
	}
}

func keepaliveLoop(c *Client, stop <-chan struct{}) {
	ticker := time.NewTicker(keepaliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if err := c.WriteKeepalive(); err != nil {
				log.Printf("keepalive to %s failed: %v", c.user.Username, err)
				c.Close()
				return
			}
		}
	}
}

// enableTCPKeepalive lets the OS reclaim connections whose peer vanished
// without closing, which application deadlines alone would not notice.
func enableTCPKeepalive(c net.Conn) {
	tlsConn, ok := c.(*tls.Conn)
	if !ok {
		return
	}
	tcpConn, ok := tlsConn.NetConn().(*net.TCPConn)
	if !ok {
		return
	}
	_ = tcpConn.SetKeepAlive(true)
	_ = tcpConn.SetKeepAlivePeriod(keepaliveInterval)
}

// Host range of the VPN subnet: .0 is the network and .1 is reserved for the server.
const (
	ipPoolFirst = 2
	ipPoolLast  = 250
)

type ipAllocator struct {
	mu        sync.Mutex
	base      net.IP
	next      int
	allocated map[string]bool
}

// Allocate returns a free address from the pool, or nil once it is exhausted.
func (a *ipAllocator) Allocate() net.IP {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.allocated == nil {
		a.allocated = make(map[string]bool)
	}

	// net.ParseIP yields the 16-byte form, where index 3 is part of the
	// IPv4-in-IPv6 prefix rather than the last octet.
	base := a.base.To4()
	if base == nil {
		return nil
	}

	for range ipPoolLast - ipPoolFirst + 1 {
		if a.next < ipPoolFirst || a.next > ipPoolLast {
			a.next = ipPoolFirst
		}
		ip := make(net.IP, net.IPv4len)
		copy(ip, base)
		ip[3] = byte(a.next)
		a.next++

		key := ip.String()
		if !a.allocated[key] {
			a.allocated[key] = true
			return ip
		}
	}
	return nil
}

func (a *ipAllocator) Release(ip net.IP) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.allocated == nil {
		return
	}
	delete(a.allocated, ip.String())
}
