import Foundation
import NetworkExtension

@MainActor
final class VPNManager: ObservableObject {
    @Published var isConnected = false
    @Published var isBusy = false
    @Published var statusMessage = "Not connected"
    @Published var errorMessage: String?

    /// Must match the bundle identifier of the Packet Tunnel extension target.
    private let providerBundleIdentifier = "com.vpn.client.PacketTunnel"

    private var manager: NETunnelProviderManager?
    private var statusObserver: NSObjectProtocol?

    init() {
        statusObserver = NotificationCenter.default.addObserver(
            forName: .NEVPNStatusDidChange,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            Task { @MainActor in self?.refreshStatus() }
        }
    }

    deinit {
        if let statusObserver {
            NotificationCenter.default.removeObserver(statusObserver)
        }
    }

    /// Reuses the profile already installed in Settings rather than creating a
    /// duplicate on every launch.
    func loadProfile() async {
        do {
            let existing = try await NETunnelProviderManager.loadAllFromPreferences()
            manager = existing.first ?? NETunnelProviderManager()
            if let saved = existing.first?.protocolConfiguration as? NETunnelProviderProtocol {
                savedServer = saved.serverAddress ?? ""
            }
            refreshStatus()
        } catch {
            errorMessage = "Could not load the VPN profile: \(error.localizedDescription)"
        }
    }

    /// Server address stored in the installed profile, used to prefill the form.
    private(set) var savedServer = ""

    func connect(server: String, token: String) async {
        errorMessage = nil
        isBusy = true
        defer { isBusy = false }

        if manager == nil {
            await loadProfile()
        }
        guard let manager else { return }

        let proto = NETunnelProviderProtocol()
        proto.providerBundleIdentifier = providerBundleIdentifier
        proto.serverAddress = server
        proto.username = token
        proto.disconnectOnSleep = false

        manager.protocolConfiguration = proto
        manager.localizedDescription = "Custom VPN"
        manager.isEnabled = true

        do {
            try await manager.saveToPreferences()
            // Reload so the connection object points at the profile we just saved.
            try await manager.loadFromPreferences()
            try manager.connection.startVPNTunnel()
        } catch {
            errorMessage = error.localizedDescription
        }
        refreshStatus()
    }

    func disconnect() {
        manager?.connection.stopVPNTunnel()
        refreshStatus()
    }

    func refreshStatus() {
        switch manager?.connection.status {
        case .connected:
            isConnected = true
            statusMessage = "Connected"
        case .connecting:
            isConnected = false
            statusMessage = "Connecting…"
        case .reasserting:
            isConnected = true
            statusMessage = "Reconnecting…"
        case .disconnecting:
            isConnected = false
            statusMessage = "Disconnecting…"
        case .invalid:
            isConnected = false
            statusMessage = "No profile installed"
        default:
            isConnected = false
            statusMessage = "Not connected"
        }
    }
}
