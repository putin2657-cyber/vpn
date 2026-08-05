import SwiftUI

struct ContentView: View {
    @StateObject private var vpn = VPNManager()
    @AppStorage("serverAddress") private var serverAddress = "127.0.0.1:8443"
    @AppStorage("token") private var token = ""

    private var canConnect: Bool {
        !serverAddress.isEmpty && !token.isEmpty && !vpn.isBusy
    }

    var body: some View {
        NavigationStack {
            Form {
                Section("Status") {
                    HStack {
                        Circle()
                            .fill(vpn.isConnected ? .green : .secondary)
                            .frame(width: 10, height: 10)
                        Text(vpn.statusMessage)
                        if vpn.isBusy {
                            Spacer()
                            ProgressView()
                        }
                    }
                }

                Section("Server") {
                    TextField("Server (host:port)", text: $serverAddress)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .keyboardType(.URL)
                        .disabled(vpn.isConnected)

                    SecureField("Token", text: $token)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .disabled(vpn.isConnected)
                }

                Section {
                    if vpn.isConnected {
                        Button("Disconnect", role: .destructive) {
                            vpn.disconnect()
                        }
                    } else {
                        Button("Connect") {
                            Task { await vpn.connect(server: serverAddress, token: token) }
                        }
                        .disabled(!canConnect)
                    }
                }

                if let errorMessage = vpn.errorMessage {
                    Section {
                        Text(errorMessage)
                            .foregroundStyle(.red)
                            .font(.footnote)
                    }
                }
            }
            .navigationTitle("VPN Client")
            .task {
                await vpn.loadProfile()
                if serverAddress.isEmpty, !vpn.savedServer.isEmpty {
                    serverAddress = vpn.savedServer
                }
            }
        }
    }
}

#Preview {
    ContentView()
}
