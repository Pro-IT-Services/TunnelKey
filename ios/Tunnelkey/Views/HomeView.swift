import SwiftUI
import UniformTypeIdentifiers

struct HomeView: View {
    @EnvironmentObject private var store: ProfileStore
    @EnvironmentObject private var vpn: VPNController
    @EnvironmentObject private var files: FileRouter
    @AppStorage("selectedProfile") private var selectedRaw = ""

    @State private var showImporter = false
    @State private var showScanner = false
    @State private var editor: EditorForm?
    @State private var signIn: SignInRequest?
    @State private var showLog = false
    @State private var showAbout = false
    @State private var importError: String?

    /// Normal-mode profiles; a provisioned one is never listed here.
    private var profiles: [Profile] { store.profiles.filter { !$0.isManaged } }

    private var selected: Profile? {
        profiles.first { $0.id.uuidString == selectedRaw } ?? profiles.first
    }

    private var heroProfile: Profile? {
        vpn.phase.isActive ? (store.profile(vpn.profileID) ?? selected) : selected
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 10) {
                    StatusHeroView(
                        phase: vpn.phase,
                        profileName: heroProfile?.name,
                        connectedAt: vpn.connectedAt,
                        vpnAddress: vpn.vpnAddress,
                        bytesIn: vpn.bytesIn,
                        bytesOut: vpn.bytesOut,
                        errorMessage: vpn.errorMessage
                    )
                    .padding(.vertical, 16)

                    if profiles.isEmpty {
                        emptyState
                    } else {
                        Text("PROFILES")
                            .font(.caption2.weight(.semibold))
                            .tracking(1.2)
                            .foregroundStyle(Palette.muted)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .padding(.top, 12)
                            .padding(.leading, 4)

                        ForEach(profiles) { profile in
                            ProfileRow(
                                profile: profile,
                                selected: profile.id == selected?.id,
                                live: profile.id == vpn.profileID && vpn.phase == .connected,
                                onSelect: { selectedRaw = profile.id.uuidString },
                                onEdit: { editor = EditorForm(profile: profile) }
                            )
                            .disabled(vpn.phase.isActive)
                        }
                    }
                }
                .padding(.horizontal, 20)
                .padding(.bottom, 16)
            }
            .background(Palette.canvas.ignoresSafeArea())
            .navigationTitle("Tunnelkey")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .primaryAction) {
                    Button { showScanner = true } label: {
                        Image(systemName: "qrcode.viewfinder")
                    }
                    .accessibilityLabel("Scan setup code")
                }
                ToolbarItem(placement: .primaryAction) {
                    Button { showImporter = true } label: {
                        Image(systemName: "plus")
                    }
                    .accessibilityLabel("Import profile")
                }
                ToolbarItem(placement: .secondaryAction) {
                    Button { showLog = true } label: { Label("Connection Log", systemImage: "text.alignleft") }
                }
                ToolbarItem(placement: .secondaryAction) {
                    Button { showAbout = true } label: { Label("About", systemImage: "info.circle") }
                }
            }
            .safeAreaInset(edge: .bottom) {
                if !profiles.isEmpty { connectBar }
            }
        }
        .fileImporter(isPresented: $showImporter, allowedContentTypes: [.ovpnProfile, .tunnelkeySetup, .data]) { result in
            // Setup files go to the setup flow, everything else to the profile editor.
            if case let .success(url) = result { files.open(url, acceptsProfiles: true) }
        }
        .onAppear(perform: takeOpenedProfile)
        .onChange(of: files.profileURL) { _ in takeOpenedProfile() }
        .sheet(item: $editor) { form in
            EditorView(form: form, onSave: save, onDelete: delete)
        }
        .sheet(item: $signIn) { request in
            SignInSheet(request: request) { password, code in
                let pw = request.needsPassword ? password : Keychain.get(request.profile.id)
                Task { await vpn.connect(profile: request.profile, password: pw, code: code.isEmpty ? nil : code) }
            }
        }
        .sheet(isPresented: $showLog) { LogView() }
        .fullScreenCover(isPresented: $showScanner) { SetupFlowView() }
        .alert("Couldn't import profile", isPresented: Binding(get: { importError != nil }, set: { if !$0 { importError = nil } }), presenting: importError) { _ in
            Button("OK") { importError = nil }
        } message: { Text($0) }
        .sheet(isPresented: $showAbout) { AboutView() }
    }

    // MARK: - Pieces

    private var emptyState: some View {
        VStack(spacing: 6) {
            Text("No profiles yet").font(.headline)
            Text("Import an .ovpn file from your VPN provider or administrator to get started.")
                .font(.subheadline)
                .foregroundStyle(Palette.muted)
                .multilineTextAlignment(.center)
            Button {
                showImporter = true
            } label: {
                Label("Import profile", systemImage: "plus")
            }
            .buttonStyle(PrimaryButtonStyle())
            .padding(.top, 14)
            Button {
                showScanner = true
            } label: {
                Label("Scan setup code", systemImage: "qrcode.viewfinder")
            }
            .buttonStyle(PrimaryButtonStyle(prominent: false))
            Button {
                showImporter = true
            } label: {
                Label("Open setup file", systemImage: "doc.badge.gearshape")
            }
            .buttonStyle(PrimaryButtonStyle(prominent: false))
        }
        .padding(.top, 8)
    }

    private var connectBar: some View {
        VStack(spacing: 0) {
            Divider().overlay(Palette.line)
            Group {
                switch vpn.phase {
                case .connected:
                    Button("Disconnect") { vpn.disconnect() }
                        .buttonStyle(PrimaryButtonStyle(prominent: false))
                case .connecting, .reconnecting:
                    Button("Cancel") { vpn.disconnect() }
                        .buttonStyle(PrimaryButtonStyle(prominent: false))
                case .disconnecting:
                    Button("Disconnecting…") {}
                        .buttonStyle(PrimaryButtonStyle(prominent: false))
                        .disabled(true)
                case .disconnected, .failed:
                    Button("Connect") { if let selected { requestConnect(selected) } }
                        .buttonStyle(PrimaryButtonStyle())
                        .disabled(selected == nil)
                }
            }
            .padding(.horizontal, 20)
            .padding(.vertical, 12)
        }
        .background(Palette.canvas)
    }

    // MARK: - Actions

    private func requestConnect(_ profile: Profile) {
        let saved = profile.rememberPassword ? Keychain.get(profile.id) : nil
        let needsPassword = profile.needsCredentials && saved == nil
        let needsCode = profile.needsCredentials && profile.twoFactor
        guard needsPassword || needsCode else {
            Task { await vpn.connect(profile: profile, password: saved, code: nil) }
            return
        }
        let usesStaticChallenge = store.config(for: profile.id)?
            .split(whereSeparator: \.isNewline)
            .contains { $0.trimmingCharacters(in: .whitespaces).hasPrefix("static-challenge") } ?? false
        signIn = SignInRequest(
            profile: profile,
            needsPassword: needsPassword,
            needsCode: needsCode,
            usesStaticChallenge: usesStaticChallenge
        )
    }

    /// An .ovpn file opened from another app or picked in the importer.
    private func takeOpenedProfile() {
        guard let url = files.profileURL else { return }
        files.profileURL = nil
        importProfile(url)
    }

    private func importProfile(_ url: URL) {
        do {
            editor = EditorForm(draft: try store.draft(from: url))
        } catch {
            importError = error.localizedDescription
        }
    }

    private func save(_ form: EditorForm) {
        let saved = store.save(form.profile, content: form.content, password: form.rememberPassword ? form.password : nil)
        selectedRaw = saved.id.uuidString
    }

    private func delete(_ id: UUID) {
        store.delete(id)
        Task { await vpn.forget(profileID: id) }
        if selectedRaw == id.uuidString { selectedRaw = "" }
    }
}

private struct ProfileRow: View {
    let profile: Profile
    let selected: Bool
    let live: Bool
    let onSelect: () -> Void
    let onEdit: () -> Void

    var body: some View {
        HStack(spacing: 12) {
            Button(action: onSelect) {
                HStack(spacing: 12) {
                    Image(systemName: selected ? "largecircle.fill.circle" : "circle")
                        .font(.title3)
                        .foregroundStyle(selected ? Palette.brass : Palette.muted)
                    VStack(alignment: .leading, spacing: 2) {
                        HStack(spacing: 8) {
                            Text(profile.name)
                                .font(.headline)
                                .foregroundStyle(Palette.ink)
                                .lineLimit(1)
                            if profile.twoFactor {
                                Label("2FA", systemImage: "key.fill")
                                    .labelStyle(.titleAndIcon)
                                    .font(.caption2.weight(.semibold))
                                    .foregroundStyle(Palette.onBrassContainer)
                                    .padding(.horizontal, 6)
                                    .padding(.vertical, 2)
                                    .background(Palette.brassContainer, in: RoundedRectangle(cornerRadius: 6))
                            }
                        }
                        if !profile.remote.isEmpty {
                            Text(profile.remote)
                                .font(.caption.monospaced())
                                .foregroundStyle(live ? Palette.secure : Palette.muted)
                                .lineLimit(1)
                        }
                    }
                    Spacer(minLength: 0)
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityAddTraits(selected ? [.isSelected] : [])

            Button(action: onEdit) {
                Image(systemName: "slider.horizontal.3")
                    .foregroundStyle(Palette.muted)
                    .frame(width: 44, height: 44)
            }
            .accessibilityLabel("Edit \(profile.name)")
        }
        .padding(.leading, 16)
        .padding(.trailing, 6)
        .frame(minHeight: 72)
        .background(Palette.raised, in: RoundedRectangle(cornerRadius: 16, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 16, style: .continuous)
                .strokeBorder(selected ? Palette.brass : Palette.line, lineWidth: selected ? 1.5 : 1)
        )
    }
}

extension UTType {
    static let ovpnProfile = UTType(importedAs: "net.openvpn.formats.ovpn", conformingTo: .data)
}
