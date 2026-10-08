import SwiftUI

/// Single-configuration mode: the config name is the title, links sit under the tunnel.
struct ManagedHomeView: View {
    @EnvironmentObject private var managed: ManagedController
    @EnvironmentObject private var store: ProfileStore
    @EnvironmentObject private var vpn: VPNController
    @Environment(\.openURL) private var openURL

    @State private var signIn: SignInRequest?
    @State private var showLog = false
    @State private var showSecurity = false
    @State private var showAbout = false
    @State private var confirmRemove = false
    @State private var pendingLink: SetupLink?
    @State private var hint: String?
    @State private var missingApp: SetupLink?
    @State private var retriesLeft = 0
    @State private var retryTask: Task<Void, Never>?
    @State private var retryIn: Int?

    private static let maxAutoRetries = 2

    /// While waiting to retry with the next code, present it as reconnecting.
    private var phase: TunnelPhase { retryIn != nil ? .reconnecting : vpn.phase }

    /// Microsoft's Windows App (formerly Remote Desktop) on the App Store.
    private let rdpAppURL = URL(string: "https://apps.apple.com/app/id714464092")!

    var body: some View {
        let config = managed.config
        NavigationStack {
            ScrollView {
                VStack(spacing: 10) {
                    StatusHeroView(
                        phase: phase,
                        profileName: (config?.username ?? "").isEmpty ? nil : config?.username,
                        connectedAt: vpn.connectedAt,
                        vpnAddress: vpn.vpnAddress,
                        bytesIn: vpn.bytesIn,
                        bytesOut: vpn.bytesOut,
                        errorMessage: vpn.errorMessage
                    )
                    .padding(.vertical, 16)

                    if let hint {
                        Text(hint).font(.subheadline).foregroundStyle(Palette.muted).frame(maxWidth: .infinity, alignment: .leading)
                    }

                    if let links = config?.links, !links.isEmpty {
                        Text("LINKS")
                            .font(.caption2.weight(.semibold))
                            .tracking(1.2)
                            .foregroundStyle(Palette.muted)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .padding(.top, 12)
                            .padding(.leading, 4)
                        ForEach(Array(links.enumerated()), id: \.offset) { _, link in
                            LinkRow(link: link) { open(link) }
                        }
                    }
                }
                .padding(.horizontal, 20)
                .padding(.bottom, 16)
            }
            .background(Palette.canvas.ignoresSafeArea())
            .navigationTitle(config?.name ?? "Tunnelkey")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .secondaryAction) {
                    Button { showLog = true } label: { Label("Connection Log", systemImage: "text.alignleft") }
                }
                ToolbarItem(placement: .secondaryAction) {
                    Button { showSecurity = true } label: { Label("Security", systemImage: "lock") }
                }
                ToolbarItem(placement: .secondaryAction) {
                    Button { showAbout = true } label: { Label("About", systemImage: "info.circle") }
                }
                ToolbarItem(placement: .secondaryAction) {
                    Button(role: .destructive) { confirmRemove = true } label: { Label("Remove Configuration", systemImage: "trash") }
                }
            }
            .safeAreaInset(edge: .bottom) { connectBar }
        }
        .sheet(item: $signIn) { request in
            SignInSheet(request: request) { password, code in
                Task { await connect(typedPassword: password, typedCode: code) }
            }
        }
        .sheet(isPresented: $showLog) { LogView() }
        .sheet(isPresented: $showSecurity) { SecurityView() }
        .sheet(isPresented: $showAbout) { AboutView() }
        .confirmationDialog("Remove “\(config?.name ?? "")”?", isPresented: $confirmRemove, titleVisibility: .visible) {
            Button("Remove", role: .destructive) {
                vpn.disconnect()
                if let id = config?.profileID { Task { await vpn.forget(profileID: id) } }
                managed.remove(store: store)
            }
        } message: {
            Text("The VPN profile, saved password, 2FA secret and links are erased from this phone. You'll need a new setup code to use it again.")
        }
        .alert(missingApp?.title ?? "", isPresented: Binding(get: { missingApp != nil }, set: { if !$0 { missingApp = nil } })) {
            if missingApp?.kind == "rdp" {
                Button("Get Windows App") { openURL(rdpAppURL); missingApp = nil }
                Button("Cancel", role: .cancel) { missingApp = nil }
            } else {
                Button("OK") { missingApp = nil }
            }
        } message: {
            Text("No app on this phone can open “\(missingApp?.title ?? "")”.")
        }
        .onChange(of: vpn.phase) { phase in
            if phase == .connected { retriesLeft = 0 }
            guard let link = pendingLink else { return }
            if phase == .connected {
                pendingLink = nil
                hint = nil
                launch(link)
            } else if (phase == .failed || phase == .disconnected) && retryTask == nil {
                pendingLink = nil
                hint = nil
            }
        }
        .onChange(of: vpn.authRejections) { _ in scheduleRetryIfUseful() }
    }

    private var connectBar: some View {
        VStack(spacing: 0) {
            Divider().overlay(Palette.line)
            Group {
                switch phase {
                case .connected:
                    Button("Disconnect") { stop() }.buttonStyle(PrimaryButtonStyle(prominent: false))
                case .connecting, .reconnecting:
                    Button("Cancel") { stop() }.buttonStyle(PrimaryButtonStyle(prominent: false))
                case .disconnecting:
                    Button("Disconnecting…") {}.buttonStyle(PrimaryButtonStyle(prominent: false)).disabled(true)
                case .disconnected, .failed:
                    Button("Connect") { userConnect() }.buttonStyle(PrimaryButtonStyle())
                }
            }
            .padding(.horizontal, 20)
            .padding(.vertical, 12)
        }
        .background(Palette.canvas)
    }

    // MARK: Actions

    private func userConnect() {
        cancelRetry()
        retriesLeft = Self.maxAutoRetries
        Task { await connect() }
    }

    private func stop() {
        cancelRetry()
        retriesLeft = 0
        pendingLink = nil
        hint = nil
        vpn.disconnect()
    }

    /// A rejected sign-in with a generated code is usually timing (the code
    /// rolled over, or the server refuses a code it already saw). Retry once
    /// the next code is out, a limited number of times so a real problem
    /// doesn't trip the server's brute-force protection.
    private func scheduleRetryIfUseful() {
        guard retriesLeft > 0, retryTask == nil, let totp = managed.totp else { return }
        retriesLeft -= 1
        retryTask = Task {
            var left = totp.secondsLeft() + 2
            while left > 0 {
                retryIn = left
                hint = "Code rejected — trying again with the next code in \(left) s"
                try? await Task.sleep(nanoseconds: 1_000_000_000)
                if Task.isCancelled { return }
                left -= 1
            }
            retryIn = nil
            hint = pendingLink.map { "Connecting — \($0.title) opens when the VPN is up." }
            retryTask = nil
            await connect()
        }
    }

    private func cancelRetry() {
        retryTask?.cancel()
        retryTask = nil
        if retryIn != nil {
            retryIn = nil
            hint = nil
        }
    }

    /// Connects, generating the 2FA code from the stored secret when there is one.
    private func connect(typedPassword: String? = nil, typedCode: String? = nil) async {
        guard let cfg = managed.config, let profile = store.profile(cfg.profileID) else { return }
        let password = typedPassword.flatMap { $0.isEmpty ? nil : $0 } ?? managed.password
        let needsPassword = profile.needsCredentials && password == nil
        let needsCode = cfg.manualCode && (typedCode ?? "").isEmpty
        if needsPassword || needsCode {
            signIn = SignInRequest(
                profile: profile,
                needsPassword: needsPassword,
                needsCode: needsCode,
                usesStaticChallenge: store.config(for: profile.id)?.contains("static-challenge") ?? false
            )
            return
        }
        var code = typedCode.flatMap { $0.isEmpty ? nil : $0 }
        if code == nil, let totp = managed.totp {
            if totp.secondsLeft() <= 3 { hint = "Waiting for a fresh code…" }
            code = await managed.freshCode()
            if pendingLink == nil { hint = nil }
        }
        await vpn.connect(profile: profile, password: password, code: code)
    }

    private func open(_ link: SetupLink) {
        if vpn.phase == .connected {
            launch(link)
            return
        }
        pendingLink = link
        guard retryTask == nil else { return } // already reconnecting; the link opens once connected
        hint = "Connecting — \(link.title) opens when the VPN is up."
        if !vpn.phase.isActive { userConnect() }
    }

    private func launch(_ link: SetupLink) {
        guard let url = URL(string: link.uri) else { missingApp = link; return }
        UIApplication.shared.open(url) { ok in if !ok { missingApp = link } }
    }
}

private struct LinkRow: View {
    let link: SetupLink
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            HStack(spacing: 14) {
                Image(systemName: icon)
                    .font(.title3)
                    .foregroundStyle(Palette.onBrassContainer)
                    .frame(width: 40, height: 40)
                    .background(Palette.brassContainer, in: RoundedRectangle(cornerRadius: 10, style: .continuous))
                VStack(alignment: .leading, spacing: 2) {
                    Text(link.title).font(.headline).foregroundStyle(Palette.ink).lineLimit(1)
                    Text(detail).font(.caption.monospaced()).foregroundStyle(Palette.muted).lineLimit(1)
                }
                Spacer(minLength: 0)
                Image(systemName: "arrow.up.right").foregroundStyle(Palette.muted)
            }
            .padding(.horizontal, 16)
            .frame(minHeight: 68)
            .background(Palette.raised, in: RoundedRectangle(cornerRadius: 16, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: 16, style: .continuous).strokeBorder(Palette.line))
        }
        .buttonStyle(.plain)
        .accessibilityHint("Opens \(link.kind == "rdp" ? "Remote Desktop" : "the link")")
    }

    private var icon: String {
        switch link.kind {
        case "rdp": return "desktopcomputer"
        case "app": return "square.grid.2x2"
        default: return "globe"
        }
    }

    private var detail: String {
        switch link.kind {
        case "rdp":
            if let r = link.uri.range(of: "full%20address=s:") {
                let rest = link.uri[r.upperBound...]
                return String(rest.prefix { $0 != "&" }).removingPercentEncoding ?? "Remote Desktop"
            }
            return "Remote Desktop"
        case "web": return URL(string: link.uri)?.host ?? link.uri
        default: return String(link.uri.prefix { $0 != "?" })
        }
    }
}

struct SecurityView: View {
    @EnvironmentObject private var managed: ManagedController
    @Environment(\.dismiss) private var dismiss
    @State private var choosingPin = false
    @State private var error: String?

    var body: some View {
        NavigationStack {
            List {
                Section {
                    Text(current).font(.headline)
                } footer: {
                    if managed.config?.lockRequired == true {
                        Text("This configuration holds secrets, so the app has to stay locked.")
                    }
                }
                Section {
                    if Vault.biometryAvailable && managed.lockMethod != .biometric {
                        Button("Switch to \(Vault.biometryName)") { change(.biometric) }
                    }
                    Button(managed.lockMethod == .pin ? "Change PIN" : "Switch to a PIN") { choosingPin = true }
                }
                if let error {
                    Section { Text(error).foregroundStyle(Palette.danger) }
                }
            }
            .navigationTitle("Security")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) { Button("Done") { dismiss() } }
            }
            .navigationDestination(isPresented: $choosingPin) {
                PinCreateView { pin in
                    change(.pin(pin))
                    choosingPin = false
                }
            }
        }
    }

    private var current: String {
        switch managed.lockMethod {
        case .biometric: return "Unlocks with \(Vault.biometryName)"
        case .pin: return "Unlocks with an 8-digit PIN"
        case .none: return "Not locked"
        }
    }

    private func change(_ lock: LockChoice) {
        Task {
            do {
                try await managed.changeLock(to: lock)
                error = nil
            } catch {
                self.error = error.localizedDescription
            }
        }
    }
}
