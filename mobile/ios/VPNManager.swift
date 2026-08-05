import Foundation
import NetworkExtension

@MainActor
final class VPNManager: ObservableObject {
    @Published var isConnected = false
    @Published var statusMessage = "Not connected"
    @Published var assignedIP = "—"

    private let manager = NETunnelProviderManager()

    func loadProfile() async {
        do {
            try await manager.loadFromPreferences()
            refreshStatus()
        } catch {
            statusMessage = "Profile load failed: \(error.localizedDescription)"
        }
    }

    func connect(server: String, token: String) async {
        do {
            let proto = NETunnelProviderProtocol()
            proto.providerBundleIdentifier = "com.vpn.client.PacketTunnel"
            proto.serverAddress = server
            proto.username = token
            proto.disconnectOnSleep = false

            manager.protocolConfiguration = proto
            manager.localizedDescription = "Custom VPN"
            manager.isEnabled = true

            try await manager.saveToPreferences()
            try await manager.loadFromPreferences()
            try manager.connection.startVPNTunnel()

            isConnected = true
            statusMessage = "Connecting…"
        } catch {
            statusMessage = error.localizedDescription
            isConnected = false
        }
    }

    func disconnect() {
        manager.connection.stopVPNTunnel()
        isConnected = false
        statusMessage = "Disconnected"
        assignedIP = "—"
    }

    func refreshStatus() {
        switch manager.connection.status {
        case .connected:
            isConnected = true
            statusMessage = "Connected"
        case .connecting, .reasserting:
            isConnected = true
            statusMessage = "Connecting…"
        case .disconnecting:
            statusMessage = "Disconnecting…"
        default:
            isConnected = false
            statusMessage = "Not connected"
        }
    }
}
