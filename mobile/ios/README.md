# iOS VPN Client Setup

## Requirements

- Xcode 15+
- Apple Developer account (Network Extension on device)

## Steps

1. **New project** → iOS App, SwiftUI, bundle ID `com.vpn.client`

2. **Add target** → File → New → Target → Network Extension → Packet Tunnel Provider  
   Bundle ID: `com.vpn.client.PacketTunnel`

3. **Copy source files**
   | File | Target |
   |------|--------|
   | `VPNProtocol.swift` | App + Extension |
   | `VPNManager.swift` | App |
   | `ContentView.swift` | App (replace default) |
   | `PacketTunnel/PacketTunnelProvider.swift` | Extension (replace template) |

4. **Capabilities** (App + Extension): Signing & Capabilities → + → Personal VPN

5. **App entry point** — use `ContentView()` in `@main` App struct

6. **Bundle ID** — in `VPNManager.swift`, set:
   ```swift
   proto.providerBundleIdentifier = "com.vpn.client.PacketTunnel"
   ```

7. **Server address** on device: your Mac's LAN IP, not `127.0.0.1`

8. Run VPN server on Mac, create user token, connect from app.

## Troubleshooting

- **Connection refused**: check firewall, server running, correct IP:port
- **Auth failed**: token must match `vpn-admin -action list`
- **Extension not found**: bundle IDs must match exactly
