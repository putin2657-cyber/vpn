import SwiftUI

struct ContentView: View {
    @StateObject private var vpn = VPNManager()
    @State private var serverAddress = "127.0.0.1:8443"
    @State private var token = ""
    @State private var showError = false
    @State private var errorMessage = ""

    var body: some View {
        NavigationStack {
            Form {
                Section("Status") {
                    LabeledContent("Connection", value: vpn.statusMessage)
                    LabeledContent("Assigned IP", value: vpn.assignedIP)
                }

                Section("Server") {
                    TextField("Server (host:port)", text: $serverAddress)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
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
                            Task {
                                await vpn.connect(server: serverAddress, token: token)
                            }
                        }
                        .disabled(token.isEmpty || serverAddress.isEmpty)
                    }
                }
            }
            .navigationTitle("VPN Client")
            .task { await vpn.loadProfile() }
            .alert("Error", isPresented: $showError) {
                Button("OK") {}
            } message: {
                Text(errorMessage)
            }
        }
    }
}

#Preview {
    ContentView()
}
