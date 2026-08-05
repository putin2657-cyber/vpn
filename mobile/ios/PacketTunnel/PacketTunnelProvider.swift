import Foundation
import Network
import NetworkExtension

class PacketTunnelProvider: NEPacketTunnelProvider {
    private var connection: NWConnection?
    private var assignedIP: String = "10.8.0.0"
    private var isRunning = false
    private var keepaliveTimer: DispatchSourceTimer?

    override func startTunnel(options: [String: NSObject]?, completionHandler: @escaping (Error?) -> Void) {
        guard
            let proto = protocolConfiguration as? NETunnelProviderProtocol,
            let server = proto.serverAddress,
            let token = proto.username
        else {
            completionHandler(VPNProtocolError.invalidResponse("missing server or token"))
            return
        }

        let hostPort = server.split(separator: ":")
        let host = String(hostPort.first ?? "localhost")
        let port = UInt16(hostPort.count > 1 ? hostPort[1] : "8443") ?? 8443

        let conn = VPNProtocol.makeTLSConnection(host: host, port: port)
        self.connection = conn

        conn.stateUpdateHandler = { [weak self] state in
            switch state {
            case .ready:
                VPNProtocol.sendAuthToken(token, on: conn) { result in
                    switch result {
                    case .success(let response):
                        do {
                            let ip = try VPNProtocol.parseAssignedIP(from: response)
                            self?.assignedIP = ip
                            self?.applyTunnelSettings(ip: ip, completionHandler: completionHandler)
                        } catch {
                            completionHandler(error)
                        }
                    case .failure(let error):
                        completionHandler(error)
                    }
                }
            case .failed(let error):
                completionHandler(error)
            case .cancelled:
                completionHandler(VPNProtocolError.disconnected)
            default:
                break
            }
        }
        conn.start(queue: .global())
    }

    private func applyTunnelSettings(ip: String, completionHandler: @escaping (Error?) -> Void) {
        let settings = NEPacketTunnelNetworkSettings(tunnelRemoteAddress: ip)
        let ipv4 = NEIPv4Settings(addresses: [ip], subnetMasks: ["255.255.255.0"])
        ipv4.includedRoutes = [NEIPv4Route(destinationAddress: "10.8.0.0", subnetMask: "255.255.255.0")]
        settings.ipv4Settings = ipv4
        settings.mtu = 1500

        setTunnelNetworkSettings(settings) { [weak self] error in
            if let error {
                completionHandler(error)
                return
            }
            self?.isRunning = true
            self?.startPacketLoop()
            completionHandler(nil)
        }
    }

    private func startPacketLoop() {
        readFromTunnel()
        readFromServer()
        startKeepalive()
    }

    // The server hangs up on clients that stay silent past its idle window.
    private func startKeepalive() {
        guard let connection else { return }

        let timer = DispatchSource.makeTimerSource(queue: .global())
        timer.schedule(
            deadline: .now() + VPNProtocol.keepaliveInterval,
            repeating: VPNProtocol.keepaliveInterval
        )
        timer.setEventHandler { [weak self] in
            guard let self, self.isRunning else { return }
            VPNProtocol.writeKeepalive(on: connection) { _ in }
        }
        timer.resume()
        keepaliveTimer = timer
    }

    private func readFromTunnel() {
        guard isRunning, let connection else { return }
        packetFlow.readPackets { [weak self] packets, _ in
            guard let self, self.isRunning else { return }
            for packet in packets {
                VPNProtocol.writePacket(packet, on: connection) { _ in }
            }
            self.readFromTunnel()
        }
    }

    private func readFromServer() {
        guard isRunning, let connection else { return }
        VPNProtocol.readPacket(on: connection) { [weak self] result in
            guard let self, self.isRunning else { return }
            switch result {
            case .success(let packet):
                if let packet {
                    self.packetFlow.writePackets([packet], withProtocols: [AF_INET as NSNumber])
                }
                self.readFromServer()
            case .failure:
                self.cancelTunnelWithError(VPNProtocolError.disconnected)
            }
        }
    }

    override func stopTunnel(with reason: NEProviderStopReason, completionHandler: @escaping () -> Void) {
        isRunning = false
        keepaliveTimer?.cancel()
        keepaliveTimer = nil
        connection?.cancel()
        connection = nil
        completionHandler()
    }
}
