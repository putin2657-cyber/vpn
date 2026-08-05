# VPN Project

Custom TLS VPN with user management, Mac TUN client, iOS Network Extension, and Android VpnService.

## Architecture

```
┌─────────────┐     TLS + framed IP packets     ┌─────────────┐
│ vpn-client  │ ◄──────────────────────────────►│ vpn-server  │
│ (Mac/iOS/   │                                 │  :8443      │
│  Android)   │                                 └──────┬──────┘
└─────────────┘                                        │
                                                       │ SQLite
┌─────────────┐     REST :3000                         ▼
│ vpn-admin   │ ◄──────────────────────────────►  vpn.db
│ vpn-api     │
└─────────────┘
```

### Protocol

1. Client sends `token\n`
2. Server replies `OK 10.8.0.x\n` or `AUTH_FAIL\n`
3. All further traffic: `[4-byte BE length][raw IPv4 packet]`
4. A frame with length `0` is a keepalive and carries no payload

VPN subnet: `10.8.0.0/24`, addresses `.2`–`.250` (`.1` is reserved for the server).

Both sides send a keepalive every 30s and drop a peer that has been silent for
90s. A connection that does not send its token within 10s is closed.

The server forwards a packet only when its source address matches the address it
assigned, so one client cannot inject traffic on another's address.

## Mac (start here)

### Build

```bash
cd ~/CLionProjects/untitled3
./test_vpn.sh
```

### Tests

```bash
go test ./... -race
```

`cmd/server` carries end-to-end tests that start the server on a random port and
drive real TLS clients through it: address assignment, forwarding, keepalives,
rejection of spoofed sources, and session logging.

### Run server

```bash
./vpn-server -addr :8443 -cert server.crt -key server.key
```

### Create user

```bash
./vpn-admin -action add -user alice -email alice@test.com
./vpn-admin -action list   # copy token
```

### Connection history

```bash
./vpn-admin -action sessions -limit 20
```

### Connect (TUN, needs sudo)

```bash
sudo ./connect_mac.sh localhost:8443 <TOKEN>
```

Test auth without TUN:

```bash
./vpn-client -server localhost:8443 -token <TOKEN> -no-tun
```

### Ping between two Mac clients

1. Start server
2. Add two users, connect two terminals with different tokens
3. Each client gets `10.8.0.x` — ping the peer IP from another client

## iOS

Files are in `mobile/ios/`.

### Xcode setup

1. Create **App** project (SwiftUI), bundle id e.g. `com.vpn.client`
2. Add **Network Extension → Packet Tunnel Provider** target, bundle id `com.vpn.client.PacketTunnel`
3. Copy files:
   - `VPNProtocol.swift` → shared by App + Extension
   - `ContentView.swift`, `VPNManager.swift` → App target
   - `PacketTunnel/PacketTunnelProvider.swift` → Extension target
4. Enable **Personal VPN** capability on both targets
5. Set `providerBundleIdentifier` in `VPNManager.swift` to your extension bundle id
6. For device testing: use your Mac LAN IP instead of `127.0.0.1`

> Apple requires a paid Developer account for Network Extension on a physical device.

## Android

Project: `mobile/android/`

```bash
cd mobile/android
./gradlew assembleDebug
```

- Emulator → server: use `10.0.2.2:8443` (host machine)
- Physical device → use your Mac's LAN IP, e.g. `192.168.1.x:8443`

Grant VPN permission when prompted.

## REST API

```bash
./vpn-api -port 3000
curl http://localhost:3000/health
curl http://localhost:3000/api/users
```

## Security notes (dev only)

- TLS uses self-signed cert; clients skip verification, so the tunnel is
  trivially MITM-able on a hostile network
- Tokens are stored in plaintext in SQLite
- The REST API on `:3000` has no authentication and permits any origin, while
  exposing every user's token — do not bind it to a public interface
- Not suitable for production without hardening

## Project layout

| Path | Description |
|------|-------------|
| `cmd/server` | VPN server |
| `cmd/client-advanced` | Mac client with TUN |
| `cmd/admin` | User CLI |
| `cmd/api` | REST API |
| `pkg/protocol` | Wire protocol |
| `pkg/tun` | macOS/Linux TUN (water) |
| `mobile/ios` | SwiftUI + Packet Tunnel |
| `mobile/android` | Kotlin VpnService |
