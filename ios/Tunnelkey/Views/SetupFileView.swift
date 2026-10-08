import SwiftUI

/// Asks for the password of an encrypted setup file and returns its payload.
struct SetupFilePasswordView: View {
    let request: SetupFileRequest
    let onOpened: (SetupPayload) -> Void

    @Environment(\.dismiss) private var dismiss
    @State private var password = ""
    @State private var showPassword = false
    @State private var busy = false
    @State private var error: String?
    @FocusState private var passwordFocused: Bool

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                Label(request.fileName, systemImage: "doc.badge.gearshape")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(Palette.ink)
                    .lineLimit(1)
                    .padding(.bottom, 8)
                Text("Enter the password for this setup file. Your administrator sends it separately from the file.")
                    .font(.subheadline)
                    .foregroundStyle(Palette.muted)
                    .padding(.bottom, 20)

                passwordField

                if busy {
                    HStack(spacing: 10) {
                        ProgressView()
                        Text("Unlocking the file…").font(.subheadline).foregroundStyle(Palette.muted)
                    }
                    .padding(.top, 14)
                } else if let error {
                    Text(error)
                        .font(.subheadline)
                        .foregroundStyle(Palette.danger)
                        .padding(.top, 14)
                }
            }
            .padding(.horizontal, 24)
            .padding(.top, 8)
        }
        .scrollDismissesKeyboard(.interactively)
        .safeAreaInset(edge: .bottom) {
            Button("Open", action: open)
                .buttonStyle(PrimaryButtonStyle())
                .disabled(busy || password.isEmpty)
                .padding(.horizontal, 24)
                .padding(.vertical, 12)
                .background(Palette.canvas)
        }
        .background(Palette.canvas)
        .navigationTitle("Open setup file")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } }
        }
        .onAppear { passwordFocused = true }
    }

    private var passwordField: some View {
        HStack {
            Group {
                if showPassword {
                    TextField("Password", text: $password)
                } else {
                    SecureField("Password", text: $password)
                }
            }
            .autocorrectionDisabled()
            .textInputAutocapitalization(.never)
            .focused($passwordFocused)
            .submitLabel(.go)
            .onSubmit(open)
            .disabled(busy)

            Button {
                showPassword.toggle()
            } label: {
                Image(systemName: showPassword ? "eye.slash" : "eye")
                    .foregroundStyle(Palette.muted)
                    .frame(width: 44, height: 44)
            }
            .accessibilityLabel(showPassword ? "Hide password" : "Show password")
        }
        .padding(.leading, 16)
        .frame(minHeight: 56)
        .background(
            RoundedRectangle(cornerRadius: 14, style: .continuous)
                .strokeBorder(error != nil ? Palette.danger : Palette.line)
        )
    }

    private func open() {
        guard !busy, !password.isEmpty else { return }
        busy = true
        error = nil
        let file = request.file
        let typed = password
        Task { @MainActor in
            do {
                // PBKDF2 with 600 000 rounds takes a noticeable moment; keep it off the main thread.
                let payload = try await Task.detached(priority: .userInitiated) {
                    try file.decrypt(password: typed)
                }.value
                busy = false
                onOpened(payload)
            } catch {
                self.error = error.localizedDescription
                busy = false
                passwordFocused = true
            }
        }
    }
}
