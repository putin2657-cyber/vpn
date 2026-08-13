import SwiftUI

struct ContentView: View {
    @StateObject private var model = StatusModel()

    var body: some View {
        NavigationSplitView {
            List {
                Section("Сервер") {
                    LabeledContent("awg0", value: model.status?.awgActive == true ? "active" : "down")
                    LabeledContent("Онлайн", value: "\(model.status?.peersOnline ?? 0)/\(model.status?.peersTotal ?? 0)")
                    LabeledContent("Uptime", value: model.status?.uptime ?? "—")
                }
                Section("Хост") {
                    Text(model.status?.memory ?? "—")
                        .font(.system(.caption, design: .monospaced))
                        .foregroundStyle(.secondary)
                    Text(model.status?.disk ?? "—")
                        .font(.system(.caption, design: .monospaced))
                        .foregroundStyle(.secondary)
                }
            }
            .navigationTitle("VPN Control")
            .listStyle(.sidebar)
        } detail: {
            VStack(alignment: .leading, spacing: 12) {
                HStack {
                    Text("Пиры")
                        .font(.title2.weight(.semibold))
                    Spacer()
                    Button("Обновить") { Task { await model.refresh() } }
                        .keyboardShortcut("r", modifiers: .command)
                }
                if let err = model.error {
                    Text(err).foregroundStyle(.red)
                }
                Table(model.status?.peers ?? []) {
                    TableColumn("Имя") { Text($0.name) }
                    TableColumn("IP") { Text($0.address) }
                    TableColumn("On") { Text($0.enabled ? "yes" : "no") }
                    TableColumn("Handshake") { Text($0.handshake) }
                    TableColumn("RX") { Text($0.rx) }
                    TableColumn("TX") { Text($0.tx) }
                    TableColumn("Endpoint") { Text($0.endpoint.isEmpty ? "—" : $0.endpoint) }
                }
                .tableStyle(.inset(alternatesRowBackgrounds: true))
            }
            .padding(16)
        }
        .task { await model.refresh() }
        .toolbar {
            ToolbarItem {
                Toggle("Авто", isOn: $model.autoRefresh)
            }
        }
        .onChange(of: model.autoRefresh) { _, on in
            model.setAutoRefresh(on)
        }
    }
}

@MainActor
final class StatusModel: ObservableObject {
    @Published var status: StatusPayload?
    @Published var error: String?
    @Published var autoRefresh = true

    private var timer: Timer?
    private let client = StatusClient()

    func refresh() async {
        do {
            status = try await client.fetch()
            error = nil
        } catch {
            self.error = error.localizedDescription
        }
    }

    func setAutoRefresh(_ on: Bool) {
        timer?.invalidate()
        guard on else { return }
        timer = Timer.scheduledTimer(withTimeInterval: 15, repeats: true) { [weak self] _ in
            Task { @MainActor in await self?.refresh() }
        }
    }
}
