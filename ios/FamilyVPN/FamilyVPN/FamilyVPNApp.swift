import SwiftUI

@main
struct FamilyVPNApp: App {
    @StateObject private var tunnel = TunnelController()

    var body: some Scene {
        WindowGroup {
            HomeView()
                .environmentObject(tunnel)
                .preferredColorScheme(.dark)
        }
    }
}
