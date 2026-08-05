import Foundation
import Network

enum VPNProtocolError: LocalizedError {
    case invalidResponse(String)
    case authFailed
    case packetTooLarge
    case disconnected

    var errorDescription: String? {
        switch self {
        case .invalidResponse(let msg): return "Invalid response: \(msg)"
        case .authFailed: return "Authentication failed"
        case .packetTooLarge: return "Packet exceeds maximum size"
        case .disconnected: return "VPN connection closed"
        }
    }
}

enum VPNProtocol {
    static let maxPacketSize = 65535
    static let maxAuthLineLength = 512

    /// Must mirror the server: keepalives have to arrive inside its idle window.
    static let keepaliveInterval: TimeInterval = 30

    static func makeTLSConnection(host: String, port: UInt16) -> NWConnection {
        let endpoint = NWEndpoint.hostPort(
            host: NWEndpoint.Host(host),
            port: NWEndpoint.Port(rawValue: port)!
        )
        let tlsOptions = NWProtocolTLS.Options()
        sec_protocol_options_set_verify_block(
            tlsOptions.securityProtocolOptions,
            { _, _, complete in complete(true) },
            DispatchQueue.global()
        )
        let params = NWParameters(tls: tlsOptions)
        return NWConnection(to: endpoint, using: params)
    }

    static func sendAuthToken(_ token: String, on connection: NWConnection, completion: @escaping (Result<String, Error>) -> Void) {
        guard let data = (token + "\n").data(using: .utf8) else {
            completion(.failure(VPNProtocolError.invalidResponse("token encoding failed")))
            return
        }

        connection.send(content: data, completion: .contentProcessed { error in
            if let error {
                completion(.failure(error))
                return
            }
            receiveLine(on: connection, completion: completion)
        })
    }

    // Reads a byte at a time on purpose: a larger receive would swallow the
    // start of the first packet when it shares a segment with the auth line.
    private static func receiveLine(
        on connection: NWConnection,
        accumulated: Data = Data(),
        completion: @escaping (Result<String, Error>) -> Void
    ) {
        connection.receive(minimumIncompleteLength: 1, maximumLength: 1) { data, _, _, error in
            if let error {
                completion(.failure(error))
                return
            }
            guard let byte = data?.first else {
                completion(.failure(VPNProtocolError.invalidResponse("empty auth response")))
                return
            }
            if byte == UInt8(ascii: "\n") {
                let line = String(decoding: accumulated, as: UTF8.self)
                completion(.success(line.trimmingCharacters(in: .whitespacesAndNewlines)))
                return
            }

            var next = accumulated
            next.append(byte)
            guard next.count < maxAuthLineLength else {
                completion(.failure(VPNProtocolError.invalidResponse("auth line too long")))
                return
            }
            receiveLine(on: connection, accumulated: next, completion: completion)
        }
    }

    /// Tunnel settings handed over by the server after a successful login.
    struct TunnelConfig: Decodable {
        let ip: String
        let gateway: String
        let netmask: String
        let mtu: Int
        let dns: [String]
    }

    static func parseConfig(from response: String) throws -> TunnelConfig {
        if response.hasPrefix("AUTH_FAIL") {
            throw VPNProtocolError.authFailed
        }
        guard response.hasPrefix("OK "), let data = response.dropFirst(3).data(using: .utf8) else {
            throw VPNProtocolError.invalidResponse(response)
        }
        do {
            return try JSONDecoder().decode(TunnelConfig.self, from: data)
        } catch {
            throw VPNProtocolError.invalidResponse("malformed tunnel config: \(error)")
        }
    }

    static func writePacket(_ packet: Data, on connection: NWConnection, completion: @escaping (Error?) -> Void) {
        guard packet.count <= maxPacketSize else {
            completion(VPNProtocolError.packetTooLarge)
            return
        }
        var length = UInt32(packet.count).bigEndian
        var frame = Data(bytes: &length, count: 4)
        frame.append(packet)
        connection.send(content: frame, completion: .contentProcessed(completion))
    }

    /// Sends a zero-length frame that keeps an idle tunnel alive.
    static func writeKeepalive(on connection: NWConnection, completion: @escaping (Error?) -> Void) {
        var length = UInt32(0).bigEndian
        let frame = Data(bytes: &length, count: 4)
        connection.send(content: frame, completion: .contentProcessed(completion))
    }

    /// Yields nil when the peer sent a keepalive rather than a packet.
    static func readPacket(on connection: NWConnection, completion: @escaping (Result<Data?, Error>) -> Void) {
        connection.receive(minimumIncompleteLength: 4, maximumLength: 4) { header, _, _, error in
            if let error {
                completion(.failure(error))
                return
            }
            guard let header, header.count == 4 else {
                completion(.failure(VPNProtocolError.disconnected))
                return
            }
            let length = header.withUnsafeBytes { $0.load(as: UInt32.self).bigEndian }
            if length == 0 {
                completion(.success(nil))
                return
            }
            guard length <= maxPacketSize else {
                completion(.failure(VPNProtocolError.invalidResponse("bad packet length")))
                return
            }
            connection.receive(minimumIncompleteLength: Int(length), maximumLength: Int(length)) { body, _, _, error in
                if let error {
                    completion(.failure(error))
                    return
                }
                guard let body else {
                    completion(.failure(VPNProtocolError.disconnected))
                    return
                }
                completion(.success(body))
            }
        }
    }
}
