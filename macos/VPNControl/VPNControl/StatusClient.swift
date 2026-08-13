import Foundation

struct PeerRow: Identifiable, Decodable {
    var id: String { name }
    let name: String
    let address: String
    let enabled: Bool
    let handshake: String
    let rx: String
    let tx: String
    let endpoint: String
    let online: Bool
}

struct StatusPayload: Decodable {
    let time: String
    let awgActive: Bool
    let peersTotal: Int
    let peersOnline: Int
    let peers: [PeerRow]
    let memory: String
    let disk: String
    let uptime: String

    enum CodingKeys: String, CodingKey {
        case time
        case awgActive = "awg_active"
        case peersTotal = "peers_total"
        case peersOnline = "peers_online"
        case peers, memory, disk, uptime
    }
}

struct StatusClient {
    /// Point this at the SSH tunnel or Caddy path that reaches statusd.
    /// Example local tunnel: ssh -L 8090:127.0.0.1:8090 root@VPS
    var baseURL = URL(string: ProcessInfo.processInfo.environment["VPN_STATUS_URL"] ?? "http://127.0.0.1:8090")!
    var token = ProcessInfo.processInfo.environment["STATUS_TOKEN"] ?? ""

    func fetch() async throws -> StatusPayload {
        var req = URLRequest(url: baseURL.appending(path: "/api/status"))
        if !token.isEmpty {
            req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        let (data, resp) = try await URLSession.shared.data(for: req)
        guard let http = resp as? HTTPURLResponse, http.statusCode == 200 else {
            throw URLError(.badServerResponse)
        }
        return try JSONDecoder().decode(StatusPayload.self, from: data)
    }
}
