import SwiftUI

@main
struct TunnelkeyApp: App {
    @StateObject private var store = ProfileStore()
    @StateObject private var vpn = VPNController()
    @StateObject private var managed = ManagedController()
    @StateObject private var files = FileRouter()
    @Environment(\.scenePhase) private var scenePhase
    @State private var backgroundedAt: Date?

    var body: some Scene {
        WindowGroup {
            RootView()
                .environmentObject(store)
                .environmentObject(vpn)
                .environmentObject(managed)
                .environmentObject(files)
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

/// Picks the lock screen, single-config home or the normal profile list,
/// and receives files opened from other apps.
struct RootView: View {
    @EnvironmentObject private var managed: ManagedController
    @EnvironmentObject private var files: FileRouter

    var body: some View {
        Group {
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
        .onOpenURL { url in files.open(url, acceptsProfiles: managed.config == nil) }
        // A setup file opened while locked waits until the app is unlocked.
        .fullScreenCover(item: Binding(
            get: { managed.needsUnlock ? nil : files.setupFile },
            set: { files.setupFile = $0 }
        )) { request in
            SetupFlowView(file: request)
        }
        .alert("Couldn't open the file", isPresented: Binding(get: { files.error != nil }, set: { if !$0 { files.error = nil } })) {
            Button("OK") { files.error = nil }
        } message: { Text(files.error ?? "") }
    }
}
