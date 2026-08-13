import Combine
import Foundation
import NetworkExtension

enum VpnUiState: Equatable {
    case disconnected
    case connecting
    case connected
    case error
}

@MainActor
final class TunnelController: ObservableObject {
    @Published private(set) var state: VpnUiState = .disconnected
    @Published var message: String?
    @Published var draft: String = ""

    private let store = ConfigStore()
    private var manager: NETunnelProviderManager?
    private var statusObserver: NSObjectProtocol?

    init() {
        draft = store.configText
        Task { await refresh() }
        statusObserver = NotificationCenter.default.addObserver(
            forName: .NEVPNStatusDidChange,
            object: nil,
            queue: .main
        ) { [weak self] note in
            guard let session = note.object as? NEVPNConnection else { return }
            Task { @MainActor in
                self?.apply(status: session.status)
            }
        }
    }

    deinit {
        if let statusObserver {
            NotificationCenter.default.removeObserver(statusObserver)
        }
    }

    var hasConfig: Bool {
        store.looksLikeConfig(draft) || store.looksLikeConfig(store.configText)
    }

    func saveDraft() {
        guard store.looksLikeConfig(draft) else {
            message = "Это не похоже на AmneziaWG .conf"
            state = .error
            return
        }
        store.configText = draft
        message = "Конфиг сохранён"
    }

    func importText(_ text: String) {
        guard store.looksLikeConfig(text) else {
            message = "Файл не похож на AmneziaWG .conf"
            return
        }
        draft = text
        store.configText = text
        message = "Конфиг импортирован"
    }

    func toggle() async {
        message = nil
        switch state {
        case .connected, .connecting:
            await disconnect()
        case .disconnected, .error:
            await connect()
        }
    }

    func refresh() async {
        do {
            let managers = try await NETunnelProviderManager.loadAllFromPreferences()
            manager = managers.first ?? NETunnelProviderManager()
            if let status = manager?.connection.status {
                apply(status: status)
            }
        } catch {
            message = error.localizedDescription
        }
    }

    private func connect() async {
        if store.looksLikeConfig(draft) {
            store.configText = draft
        }
        let conf = store.configText
        guard store.looksLikeConfig(conf) else {
            state = .error
            message = "Сначала импортируйте .conf"
            return
        }

        state = .connecting
        do {
            let mgr = try await ensureManager(config: conf)
            manager = mgr
            try mgr.connection.startVPNTunnel()
        } catch {
            state = .error
            message = error.localizedDescription
        }
    }

    private func disconnect() async {
        manager?.connection.stopVPNTunnel()
        state = .disconnected
    }

    private func ensureManager(config: String) async throws -> NETunnelProviderManager {
        let managers = try await NETunnelProviderManager.loadAllFromPreferences()
        let mgr = managers.first ?? NETunnelProviderManager()

        let proto = NETunnelProviderProtocol()
        proto.providerBundleIdentifier = AppConstants.tunnelBundleId
        proto.serverAddress = endpointHost(from: config) ?? "Family VPN"
        proto.providerConfiguration = [
            AppConstants.providerConfigKey: config.data(using: .utf8) as Any,
        ]

        mgr.protocolConfiguration = proto
        mgr.localizedDescription = "Family VPN"
        mgr.isEnabled = true
        try await mgr.saveToPreferences()
        try await mgr.loadFromPreferences()
        return mgr
    }

    private func apply(status: NEVPNStatus) {
        switch status {
        case .connected:
            state = .connected
        case .connecting, .reasserting:
            state = .connecting
        case .disconnecting, .disconnected, .invalid:
            state = .disconnected
        @unknown default:
            state = .disconnected
        }
    }

    private func endpointHost(from config: String) -> String? {
        for line in config.split(whereSeparator: \.isNewline) {
            let s = line.trimmingCharacters(in: .whitespaces)
            if s.lowercased().hasPrefix("endpoint") {
                let value = s.split(separator: "=", maxSplits: 1).last?
                    .trimmingCharacters(in: .whitespaces) ?? ""
                return String(value.split(separator: ":").first ?? Substring(value))
            }
        }
        return nil
    }
}
