import Foundation
import NetworkExtension
import WireGuardKit

final class PacketTunnelProvider: NEPacketTunnelProvider {
    private lazy var adapter = WireGuardAdapter(with: self) { _, message in
        NSLog("[FamilyVPN] %@", message)
    }

    override func startTunnel(options: [String: NSObject]?, completionHandler: @escaping (Error?) -> Void) {
        guard
            let proto = protocolConfiguration as? NETunnelProviderProtocol,
            let data = proto.providerConfiguration?[AppConstants.providerConfigKey] as? Data,
            let conf = String(data: data, encoding: .utf8)
        else {
            completionHandler(TunnelError.missingConfig)
            return
        }

        do {
            let tunnelConfig = try TunnelConfiguration(fromWgQuickConfig: conf, called: "familyvpn")
            adapter.start(tunnelConfiguration: tunnelConfig) { error in
                if let error {
                    NSLog("[FamilyVPN] start failed: %@", String(describing: error))
                }
                completionHandler(error)
            }
        } catch {
            completionHandler(error)
        }
    }

    override func stopTunnel(with reason: NEProviderStopReason, completionHandler: @escaping () -> Void) {
        adapter.stop { _ in
            completionHandler()
        }
    }

    override func handleAppMessage(_ messageData: Data, completionHandler: ((Data?) -> Void)?) {
        completionHandler?(nil)
    }
}

private enum TunnelError: LocalizedError {
    case missingConfig

    var errorDescription: String? {
        switch self {
        case .missingConfig: return "Конфиг туннеля не передан"
        }
    }
}
