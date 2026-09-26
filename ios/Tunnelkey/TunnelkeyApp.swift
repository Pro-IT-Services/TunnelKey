import SwiftUI

@main
struct TunnelkeyApp: App {
    @StateObject private var store = ProfileStore()
    @StateObject private var vpn = VPNController()
    @StateObject private var managed = ManagedController()
    @Environment(\.scenePhase) private var scenePhase
    @State private var backgroundedAt: Date?

    var body: some Scene {
        WindowGroup {
            RootView()
                .environmentObject(store)
                .environmentObject(vpn)
                .environmentObject(managed)
                .tint(Palette.brass)
        }
        .onChange(of: scenePhase) { phase in
            switch phase {
            case .background:
                backgroundedAt = Date()
            case .active:
                // Re-lock after the app has been away for a while.
                if let at = backgroundedAt, Date().timeIntervalSince(at) > 30 { managed.lock() }
                backgroundedAt = nil
                managed.unlockIfUnprotected()
            default:
                break
            }
        }
    }
}

/// Picks the lock screen, single-config home or the normal profile list.
struct RootView: View {
    @EnvironmentObject private var managed: ManagedController

    var body: some View {
        if managed.config != nil {
            if managed.needsUnlock {
                LockView()
            } else {
                ManagedHomeView()
            }
        } else {
            HomeView()
        }
    }
}
