package main

import (
	"bufio"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"vpn-server/pkg/protocol"
	"vpn-server/pkg/tun"
)

var (
	serverAddr = flag.String("server", "localhost:8443", "VPN server address")
	token      = flag.String("token", "", "authentication token")
	skipTUN    = flag.Bool("no-tun", false, "connect without creating a TUN device (testing mode)")
	fullTunnel = flag.Bool("full", true, "send all traffic through the VPN, not just the VPN subnet")
)

// Must mirror the server: keepalives have to arrive well inside its idle window.
const (
	keepaliveInterval = 30 * time.Second
	idleTimeout       = 90 * time.Second
	writeTimeout      = 10 * time.Second
)

var errNotConnected = errors.New("not connected")

type VPNClient struct {
	conn        net.Conn
	assignedIP  net.IP
	token       string
	serverAddr  string
	tunDev      *tun.Device
	restore     func()
	mu          sync.Mutex
	writeMu     sync.Mutex
	isConnected bool
}

// sendPacket serialises writes to the server: the TUN pump and the keepalive
// ticker share one connection. A nil packet is sent as a keepalive frame.
func (vc *VPNClient) sendPacket(pkt []byte) error {
	vc.writeMu.Lock()
	defer vc.writeMu.Unlock()

	vc.mu.Lock()
	conn := vc.conn
	vc.mu.Unlock()
	if conn == nil {
		return errNotConnected
	}

	if err := conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return err
	}
	if pkt == nil {
		return protocol.WriteKeepalive(conn)
	}
	return protocol.WritePacket(conn, pkt)
}

func (vc *VPNClient) connected() bool {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	return vc.isConnected
}

func (vc *VPNClient) keepaliveLoop() {
	ticker := time.NewTicker(keepaliveInterval)
	defer ticker.Stop()
	for range ticker.C {
		if !vc.connected() {
			return
		}
		if err := vc.sendPacket(nil); err != nil {
			if !errors.Is(err, errNotConnected) {
				log.Printf("keepalive failed: %v", err)
			}
			return
		}
	}
}

func main() {
	flag.Parse()

	if *token == "" {
		fmt.Println("Usage: sudo ./vpn-client -server <host:port> -token <token>")
		fmt.Println("       Add -no-tun to test TLS auth without TUN (no sudo required)")
		os.Exit(1)
	}

	client := &VPNClient{
		token:      *token,
		serverAddr: *serverAddr,
	}

	if err := client.Connect(); err != nil {
		log.Fatalf("Connection failed: %v", err)
	}
	defer client.Disconnect()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go client.keepaliveLoop()

	if client.tunDev != nil {
		go client.tunnelFromTUN()
		go client.tunnelToTUN()
		log.Printf("VPN active on %s via %s", client.assignedIP, client.tunDev.Name())
		log.Println("Press Ctrl+C to disconnect")
	} else {
		go client.testReceiveLoop()
		log.Println("Connected in test mode (-no-tun). Press Ctrl+C to exit.")
	}

	<-sigChan
}

func (vc *VPNClient) Connect() error {
	log.Printf("Connecting to VPN server at %s", vc.serverAddr)

	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	conn, err := tls.Dial("tcp", vc.serverAddr, tlsConfig)
	if err != nil {
		return fmt.Errorf("TLS dial failed: %w", err)
	}

	if _, err := conn.Write([]byte(vc.token + "\n")); err != nil {
		conn.Close()
		return fmt.Errorf("token write failed: %w", err)
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		conn.Close()
		return fmt.Errorf("auth response read failed: %w", err)
	}

	cfg, err := protocol.ParseAuthResponse(line)
	if err != nil {
		conn.Close()
		return err
	}

	vc.conn = conn
	vc.assignedIP = cfg.Addr()
	vc.isConnected = true
	log.Printf("Authenticated, assigned IP: %s (gateway %s, MTU %d)",
		cfg.IP, cfg.Gateway, cfg.MTU)

	if *skipTUN {
		return nil
	}

	dev, err := tun.OpenWithMTU(cfg.Addr(), cfg.Mask(), cfg.MTU)
	if err != nil {
		conn.Close()
		return fmt.Errorf("TUN setup failed (run with sudo): %w", err)
	}
	vc.tunDev = dev

	if *fullTunnel {
		serverIP := conn.RemoteAddr().(*net.TCPAddr).IP
		restore, err := tun.Redirect(dev, serverIP, cfg.DNS)
		if err != nil {
			dev.Close()
			conn.Close()
			return fmt.Errorf("could not redirect traffic through the tunnel: %w", err)
		}
		vc.restore = restore
	} else {
		log.Println("split mode: only VPN subnet traffic uses the tunnel")
	}

	return nil
}

func (vc *VPNClient) tunnelFromTUN() {
	for {
		vc.mu.Lock()
		if !vc.isConnected || vc.tunDev == nil {
			vc.mu.Unlock()
			return
		}
		dev := vc.tunDev
		vc.mu.Unlock()

		pkt, err := dev.Read()
		if err != nil {
			log.Printf("TUN read error: %v", err)
			return
		}
		if err := vc.sendPacket(pkt); err != nil {
			log.Printf("server write error: %v", err)
			return
		}
	}
}

// receivePacket reads one frame from the server, skipping keepalives.
func (vc *VPNClient) receivePacket() ([]byte, error) {
	for {
		vc.mu.Lock()
		conn := vc.conn
		live := vc.isConnected
		vc.mu.Unlock()
		if !live || conn == nil {
			return nil, errNotConnected
		}

		if err := conn.SetReadDeadline(time.Now().Add(idleTimeout)); err != nil {
			return nil, err
		}
		pkt, err := protocol.ReadPacket(conn)
		if err != nil {
			return nil, err
		}
		if pkt != nil {
			return pkt, nil
		}
	}
}

func (vc *VPNClient) tunnelToTUN() {
	for {
		vc.mu.Lock()
		dev := vc.tunDev
		vc.mu.Unlock()
		if dev == nil {
			return
		}

		pkt, err := vc.receivePacket()
		if err != nil {
			logReadError(err)
			return
		}
		if _, err := dev.Write(pkt); err != nil {
			log.Printf("TUN write error: %v", err)
			return
		}
	}
}

func (vc *VPNClient) testReceiveLoop() {
	for {
		pkt, err := vc.receivePacket()
		if err != nil {
			logReadError(err)
			return
		}
		dest, _ := protocol.DestIP(pkt)
		log.Printf("received packet: %d bytes, dest=%v", len(pkt), dest)
	}
}

func logReadError(err error) {
	if errors.Is(err, io.EOF) || errors.Is(err, errNotConnected) {
		return
	}
	log.Printf("server read error: %v", err)
}

func (vc *VPNClient) Disconnect() error {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	vc.isConnected = false

	// Restore routing and DNS before the interface disappears, otherwise the
	// machine is left without a working default route.
	if vc.restore != nil {
		vc.restore()
		vc.restore = nil
	}

	var err error
	if vc.tunDev != nil {
		err = vc.tunDev.Close()
		vc.tunDev = nil
	}
	if vc.conn != nil {
		if closeErr := vc.conn.Close(); err == nil {
			err = closeErr
		}
		vc.conn = nil
	}
	return err
}
