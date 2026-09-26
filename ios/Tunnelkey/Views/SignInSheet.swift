import SwiftUI

/// What the sign-in sheet has to ask for before connecting.
struct SignInRequest: Identifiable {
    let profile: Profile
    let needsPassword: Bool
    let needsCode: Bool
    /// Profile uses OpenVPN's static-challenge instead of password+code.
    let usesStaticChallenge: Bool

    var id: UUID { profile.id }
}

struct SignInSheet: View {
    let request: SignInRequest
    let onSubmit: (_ password: String, _ code: String) -> Void

    @Environment(\.dismiss) private var dismiss
    @State private var password = ""
    @State private var code = ""
    @State private var showPassword = false
    @FocusState private var passwordFocused: Bool
    @FocusState private var codeFocused: Bool

    private var profile: Profile { request.profile }
    private var passwordOK: Bool { !request.needsPassword || !password.isEmpty }
    private var codeOK: Bool { !request.needsCode || Credentials.isValidCode(code, length: profile.codeLength) }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 0) {
                    if !profile.username.isEmpty {
                        Text("Signed in as \(profile.username)")
                            .font(.subheadline)
                            .foregroundStyle(Palette.muted)
                            .padding(.bottom, 20)
                    }

                    if request.needsPassword {
                        passwordField.padding(.bottom, 24)
                    }

                    if request.needsCode {
                        HStack {
                            Text("Authenticator code").font(.headline)
                            Spacer()
                            Button {
                                let digits = (UIPasteboard.general.string ?? "").filter { ("0"..."9").contains($0) }
                                if digits.count == profile.codeLength { code = digits }
                            } label: {
                                Label("Paste", systemImage: "doc.on.clipboard")
                                    .font(.subheadline.weight(.medium))
                            }
                            .frame(minHeight: 44)
                        }
                        .padding(.bottom, 8)

                        CodeField(code: $code, length: profile.codeLength, focused: $codeFocused)

                        Text(request.usesStaticChallenge
                             ? "This profile uses OpenVPN's challenge prompt, so the code is sent separately from the password."
                             : "Enter the \(profile.codeLength)-digit code from your authenticator app.")
                            .font(.footnote)
                            .foregroundStyle(Palette.muted)
                            .padding(.top, 10)

                        if !request.usesStaticChallenge {
                            combinedPreview.padding(.top, 14)
                        }
                    }
                }
                .padding(.horizontal, 24)
                .padding(.top, 8)
            }
            .scrollDismissesKeyboard(.interactively)
            .safeAreaInset(edge: .bottom) {
                Button("Connect", action: submit)
                    .buttonStyle(PrimaryButtonStyle())
                    .disabled(!(passwordOK && codeOK))
                    .padding(.horizontal, 24)
                    .padding(.vertical, 12)
                    .background(Palette.canvas)
            }
            .background(Palette.canvas)
            .navigationTitle("Sign in to \(profile.name)")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
            }
        }
        .presentationDetents([.large])
        .onAppear {
            if request.needsPassword { passwordFocused = true } else { codeFocused = true }
        }
        .onChange(of: code) { newValue in
            // Typing the last digit is the natural "go" moment.
            if newValue.count == profile.codeLength && passwordOK && codeOK { submit() }
        }
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
            .textContentType(.password)
            .autocorrectionDisabled()
            .textInputAutocapitalization(.never)
            .focused($passwordFocused)
            .submitLabel(request.needsCode ? .next : .go)
            .onSubmit {
                if request.needsCode { codeFocused = true } else { submit() }
            }

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
            RoundedRectangle(cornerRadius: 14, style: .continuous).strokeBorder(Palette.line)
        )
    }

    /// Shows how password and code are glued together, without revealing either.
    private var combinedPreview: some View {
        let pw = String(repeating: "•", count: request.needsPassword ? min(max(password.count, 4), 12) : 8)
        let otp = code.padding(toLength: profile.codeLength, withPad: "·", startingAt: 0)
        let pwText = Text(pw).foregroundColor(Palette.muted)
        let otpText = Text(otp).foregroundColor(Palette.brass)
        return HStack {
            Text("Sent to the server as")
                .font(.caption.weight(.medium))
                .foregroundStyle(Palette.muted)
            Spacer()
            (profile.codePosition == .afterPassword ? pwText + otpText : otpText + pwText)
                .font(.subheadline.monospaced())
                .lineLimit(1)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
        .background(Palette.high, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
    }

    private func submit() {
        guard passwordOK && codeOK else { return }
        onSubmit(password, code)
        dismiss()
    }
}
