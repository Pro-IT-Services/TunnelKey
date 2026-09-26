import SwiftUI

// MARK: - PIN pad

/// Eight dots that fill as digits are typed; shakes on error.
struct PinDots: View {
    let filled: Int
    let error: Bool
    let shakeTrigger: Int
    @State private var offset: CGFloat = 0

    var body: some View {
        HStack(spacing: 12) {
            ForEach(0..<PinPolicy.length, id: \.self) { i in
                Circle()
                    .strokeBorder(error ? Palette.danger : Palette.line, lineWidth: i < filled ? 0 : 1.5)
                    .background(Circle().fill(i < filled ? (error ? Palette.danger : Palette.brass) : .clear))
                    .frame(width: 14, height: 14)
                if i == 3 { Spacer().frame(width: 6) }
            }
        }
        .offset(x: offset)
        .onChange(of: shakeTrigger) { _ in
            withAnimation(.interpolatingSpring(stiffness: 900, damping: 12)) { offset = -12 }
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.12) {
                withAnimation(.interpolatingSpring(stiffness: 600, damping: 10)) { offset = 0 }
            }
        }
        .accessibilityElement()
        .accessibilityLabel("\(filled) of \(PinPolicy.length) digits entered")
    }
}

/// Large numeric keypad for the thumb zone.
struct PinPad: View {
    let onDigit: (Character) -> Void
    let onDelete: () -> Void
    var biometric: (() -> Void)? = nil
    var enabled = true

    var body: some View {
        VStack(spacing: 14) {
            ForEach(["123", "456", "789"], id: \.self) { row in
                HStack(spacing: 22) {
                    ForEach(Array(row), id: \.self) { c in key(String(c)) { onDigit(c) } }
                }
            }
            HStack(spacing: 22) {
                if let biometric {
                    Button(action: biometric) {
                        Image(systemName: Vault.biometryName == "Touch ID" ? "touchid" : "faceid")
                            .font(.title2)
                            .frame(width: 76, height: 76)
                    }
                    .accessibilityLabel("Unlock with \(Vault.biometryName)")
                } else {
                    Color.clear.frame(width: 76, height: 76)
                }
                key("0") { onDigit("0") }
                Button(action: onDelete) {
                    Image(systemName: "delete.left").font(.title2).frame(width: 76, height: 76)
                }
                .accessibilityLabel("Delete digit")
            }
        }
        .foregroundStyle(Palette.ink)
        .disabled(!enabled)
        .opacity(enabled ? 1 : 0.4)
    }

    private func key(_ label: String, action: @escaping () -> Void) -> some View {
        Button {
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            action()
        } label: {
            Text(label)
                .font(.system(size: 28, weight: .medium, design: .monospaced))
                .frame(width: 76, height: 76)
                .background(Palette.high, in: Circle())
        }
    }
}

private struct PinScaffold: View {
    let title: String
    let subtitle: String
    let subtitleIsError: Bool
    let filled: Int
    let shake: Int
    let onDigit: (Character) -> Void
    let onDelete: () -> Void
    var biometric: (() -> Void)? = nil
    var enabled = true

    var body: some View {
        VStack(spacing: 0) {
            Spacer()
            Image(systemName: "lock").font(.title).foregroundStyle(Palette.brass)
            Text(title).font(.title2.weight(.semibold)).multilineTextAlignment(.center).padding(.top, 14)
            Text(subtitle)
                .font(.subheadline)
                .foregroundStyle(subtitleIsError ? Palette.danger : Palette.muted)
                .multilineTextAlignment(.center)
                .frame(minHeight: 40)
                .padding(.horizontal, 32)
                .padding(.top, 8)
            PinDots(filled: filled, error: subtitleIsError && filled == 0, shakeTrigger: shake)
                .padding(.top, 20)
            Spacer()
            PinPad(onDigit: onDigit, onDelete: onDelete, biometric: biometric, enabled: enabled)
                .padding(.bottom, 24)
        }
        .frame(maxWidth: .infinity)
        .background(Palette.canvas.ignoresSafeArea())
    }
}

// MARK: - PIN creation

/// Two-step PIN entry with the strength rules from `PinPolicy`.
struct PinCreateView: View {
    let onDone: (String) -> Void

    @State private var first: String?
    @State private var entry = ""
    @State private var error: String?
    @State private var shake = 0

    var body: some View {
        PinScaffold(
            title: first == nil ? "Choose an 8-digit PIN" : "Enter the PIN again",
            subtitle: error ?? (first == nil ? "Avoid repeats, sequences and dates." : ""),
            subtitleIsError: error != nil,
            filled: entry.count,
            shake: shake,
            onDigit: { c in
                guard entry.count < PinPolicy.length else { return }
                entry.append(c)
                if entry.count == PinPolicy.length { complete(entry) }
            },
            onDelete: { if !entry.isEmpty { entry.removeLast() } }
        )
    }

    private func complete(_ pin: String) {
        if let confirmed = first {
            if pin == confirmed {
                onDone(pin)
            } else {
                error = "The PINs didn't match. Start again."
                first = nil
                shake += 1
            }
        } else if let problem = PinPolicy.check(pin) {
            error = problem.message
            shake += 1
        } else {
            first = pin
            error = nil
        }
        entry = ""
    }
}

// MARK: - Lock screen

struct LockView: View {
    @EnvironmentObject private var managed: ManagedController
    @EnvironmentObject private var store: ProfileStore

    @State private var entry = ""
    @State private var error: String?
    @State private var shake = 0
    @State private var busy = false
    @State private var until: Date?
    @State private var now = Date()
    private let ticker = Timer.publish(every: 1, on: .main, in: .common).autoconnect()

    private var usesPin: Bool { managed.lockMethod == .pin }
    private var waiting: Bool { until.map { now < $0 } ?? false }

    var body: some View {
        Group {
            if usesPin {
                PinScaffold(
                    title: managed.config?.name ?? "Tunnelkey",
                    subtitle: waitText ?? error ?? "Enter your PIN",
                    subtitleIsError: waiting || error != nil,
                    filled: entry.count,
                    shake: shake,
                    onDigit: digit,
                    onDelete: { if !entry.isEmpty { entry.removeLast() } },
                    enabled: !busy && !waiting
                )
            } else {
                VStack(spacing: 16) {
                    Spacer()
                    Image(systemName: "lock").font(.largeTitle).foregroundStyle(Palette.brass)
                    Text(managed.config?.name ?? "Tunnelkey").font(.title2.weight(.semibold))
                    Text("Locked").foregroundStyle(Palette.muted)
                    if let error {
                        Text(error).foregroundStyle(Palette.danger).multilineTextAlignment(.center).padding(.horizontal, 24)
                    }
                    Spacer()
                    Button {
                        Task { await unlockBiometric() }
                    } label: {
                        Label("Unlock with \(Vault.biometryName)", systemImage: Vault.biometryName == "Touch ID" ? "touchid" : "faceid")
                    }
                    .buttonStyle(PrimaryButtonStyle())
                    .padding(24)
                }
                .frame(maxWidth: .infinity)
                .background(Palette.canvas.ignoresSafeArea())
                .task { await unlockBiometric() }
            }
        }
        .onAppear { until = managed.vault.lockedUntil }
        .onReceive(ticker) { now = $0 }
    }

    private var waitText: String? {
        guard let until, waiting else { return nil }
        let s = Int(until.timeIntervalSince(now)) + 1
        return "Too many attempts. Try again in " + (s >= 60 ? "\(s / 60) min \(s % 60) s." : "\(s) s.")
    }

    private func digit(_ c: Character) {
        guard entry.count < PinPolicy.length else { return }
        entry.append(c)
        guard entry.count == PinPolicy.length else { return }
        let pin = entry
        busy = true
        Task {
            switch await managed.unlockWithPin(pin, store: store) {
            case .unlocked: break
            case .wrong(let left, let lockedUntil):
                error = "Wrong PIN. \(left) attempts left before this configuration is erased."
                until = lockedUntil
                shake += 1
            case .lockedOut(let lockedUntil):
                until = lockedUntil
            case .wiped:
                error = "Too many wrong PINs. The configuration was erased — scan the setup code again."
            }
            entry = ""
            busy = false
        }
    }

    private func unlockBiometric() async {
        do {
            try await managed.unlockWithBiometric()
        } catch VaultError.cancelled {
            // user closed the sheet; they can tap the button again
        } catch VaultError.biometryChanged {
            error = VaultError.biometryChanged.errorDescription
            managed.remove(store: store)
        } catch {
            self.error = error.localizedDescription
        }
    }
}

// MARK: - Setup flow

/// Scan → choose lock → (PIN) → installed.
struct SetupFlowView: View {
    @EnvironmentObject private var managed: ManagedController
    @EnvironmentObject private var store: ProfileStore
    @EnvironmentObject private var vpn: VPNController
    @Environment(\.dismiss) private var dismiss

    @State private var payload: SetupPayload?
    @State private var choosingPin = false
    @State private var error: String?

    var body: some View {
        NavigationStack {
            Group {
                if let payload {
                    protectView(payload)
                } else {
                    ScanSetupView { payload = $0 }
                }
            }
            .navigationDestination(isPresented: $choosingPin) {
                PinCreateView { pin in finish(.pin(pin)) }
                    .navigationBarTitleDisplayMode(.inline)
            }
        }
        .alert("Couldn't save the configuration", isPresented: Binding(get: { error != nil }, set: { if !$0 { error = nil } })) {
            Button("OK") { error = nil }
        } message: { Text(error ?? "") }
    }

    private func protectView(_ p: SetupPayload) -> some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 12) {
                Text("This setup code contains:").font(.subheadline.weight(.semibold)).foregroundStyle(Palette.muted)
                included("lock.shield", "VPN profile")
                if !(p.password ?? "").isEmpty { included("key.horizontal", "Saved password") }
                if p.totp != nil { included("key", "2FA secret — codes are generated on this phone") }
                if !p.links.isEmpty { included("link", p.links.count == 1 ? "1 link" : "\(p.links.count) links") }

                Text(p.hasSecrets
                     ? "Because it holds secrets, the app has to be locked."
                     : "You can lock the app so only you can connect.")
                    .font(.subheadline)
                    .foregroundStyle(Palette.muted)
                    .padding(.top, 8)
                if managed.config != nil {
                    Text("This replaces the configuration currently on this phone.")
                        .font(.subheadline)
                        .foregroundStyle(Palette.danger)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(24)
        }
        .safeAreaInset(edge: .bottom) {
            VStack(spacing: 12) {
                if Vault.biometryAvailable {
                    Button {
                        finish(.biometric)
                    } label: {
                        Label("Use \(Vault.biometryName)", systemImage: Vault.biometryName == "Touch ID" ? "touchid" : "faceid")
                    }
                    .buttonStyle(PrimaryButtonStyle())
                }
                Button {
                    choosingPin = true
                } label: {
                    Label("Use an 8-digit PIN", systemImage: "circle.grid.3x3")
                }
                .buttonStyle(PrimaryButtonStyle(prominent: !Vault.biometryAvailable))
                if !p.hasSecrets {
                    Button("Don't lock") { finish(.none) }.frame(minHeight: 44)
                }
            }
            .padding(24)
            .background(Palette.canvas)
        }
        .background(Palette.canvas)
        .navigationTitle("Protect \(p.name)")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } }
        }
    }

    private func included(_ icon: String, _ text: String) -> some View {
        Label(text, systemImage: icon).foregroundStyle(Palette.ink)
    }

    private func finish(_ lock: LockChoice) {
        guard let payload else { return }
        Task {
            do {
                if vpn.phase.isActive { vpn.disconnect() }
                try await managed.install(payload, lock: lock, store: store)
                dismiss()
            } catch {
                self.error = error.localizedDescription
            }
        }
    }
}
