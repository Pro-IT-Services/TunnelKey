import SwiftUI

/// Form state for a new import or an existing profile.
struct EditorForm: Identifiable {
    var id = UUID()
    var isNew = true
    var name = ""
    var remote = ""
    var username = ""
    var needsCredentials = true
    var twoFactor = false
    var codePosition: CodePosition = .afterPassword
    var codeLength = 6
    var rememberPassword = false
    var password = ""
    var hasSavedPassword = false
    var importedAt = Date()
    /// Set for new imports only.
    var content: String?
    var externalFiles: [String] = []
    var staticChallenge: String?

    init(draft: ImportDraft) {
        name = draft.suggestedName
        remote = draft.summary.remote
        needsCredentials = draft.summary.needsCredentials
        twoFactor = draft.summary.staticChallenge != nil
        content = draft.content
        externalFiles = draft.summary.externalFiles
        staticChallenge = draft.summary.staticChallenge
    }

    init(profile: Profile) {
        id = profile.id
        isNew = false
        name = profile.name
        remote = profile.remote
        username = profile.username
        needsCredentials = profile.needsCredentials
        twoFactor = profile.twoFactor
        codePosition = profile.codePosition
        codeLength = profile.codeLength
        rememberPassword = profile.rememberPassword
        hasSavedPassword = Keychain.has(profile.id)
        importedAt = profile.importedAt
    }

    var canSave: Bool {
        !name.trimmingCharacters(in: .whitespaces).isEmpty
            && (!needsCredentials || !username.trimmingCharacters(in: .whitespaces).isEmpty)
    }

    var profile: Profile {
        Profile(
            id: id,
            name: name.trimmingCharacters(in: .whitespaces),
            remote: remote,
            username: username.trimmingCharacters(in: .whitespaces),
            needsCredentials: needsCredentials,
            twoFactor: twoFactor,
            codePosition: codePosition,
            codeLength: codeLength,
            rememberPassword: rememberPassword && needsCredentials,
            importedAt: importedAt
        )
    }
}

struct EditorView: View {
    @State var form: EditorForm
    let onSave: (EditorForm) -> Void
    let onDelete: (UUID) -> Void

    @Environment(\.dismiss) private var dismiss
    @State private var confirmDelete = false

    var body: some View {
        NavigationStack {
            Form {
                if !form.remote.isEmpty {
                    Section {
                        LabeledContent("Server") {
                            Text(form.remote).font(.subheadline.monospaced())
                        }
                    }
                }

                if !form.externalFiles.isEmpty {
                    warning("This profile refers to files that aren't included: \(form.externalFiles.joined(separator: ", ")). Ask for a profile with the certificates embedded (inline).")
                }
                if form.isNew, let challenge = form.staticChallenge {
                    warning("The profile declares a static challenge (“\(challenge)”). 2FA is turned on for you; the code will be sent as the challenge response.")
                }

                Section {
                    TextField("Name", text: $form.name)
                    if form.needsCredentials {
                        TextField("Username", text: $form.username)
                            .textContentType(.username)
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                    }
                } footer: {
                    if !form.needsCredentials {
                        Text("This profile signs in with certificates only, so no password is needed.")
                    }
                }

                if form.needsCredentials {
                    twoFactorSection
                    passwordSection
                }

                if !form.isNew {
                    Section {
                        Button("Delete Profile", role: .destructive) { confirmDelete = true }
                    }
                }
            }
            .scrollContentBackground(.hidden)
            .background(Palette.canvas)
            .navigationTitle(form.isNew ? "New Profile" : "Edit Profile")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") {
                        onSave(form)
                        dismiss()
                    }
                    .disabled(!form.canSave)
                }
            }
            .confirmationDialog("Delete “\(form.name)”?", isPresented: $confirmDelete, titleVisibility: .visible) {
                Button("Delete Profile", role: .destructive) {
                    onDelete(form.id)
                    dismiss()
                }
            } message: {
                Text("The profile and any saved password will be removed from this device.")
            }
        }
    }

    private var twoFactorSection: some View {
        Section {
            Toggle("Server asks for an authenticator code", isOn: $form.twoFactor.animation())
            if form.twoFactor {
                Picker("Code placement", selection: $form.codePosition) {
                    Text("After password").tag(CodePosition.afterPassword)
                    Text("Before password").tag(CodePosition.beforePassword)
                }
                .pickerStyle(.segmented)
                .listRowSeparator(.hidden)

                Text(form.codePosition == .afterPassword ? "password123456" : "123456password")
                    .font(.footnote.monospaced())
                    .foregroundStyle(Palette.muted)

                Picker("Code length", selection: $form.codeLength) {
                    Text("6 digits").tag(6)
                    Text("8 digits").tag(8)
                }
            }
        } header: {
            Text("Two-factor authentication")
        } footer: {
            Text("The code is combined with your password and sent as one password, the way OpenVPN servers with TOTP plugins expect.")
        }
    }

    private var passwordSection: some View {
        Section {
            Toggle("Remember password", isOn: $form.rememberPassword.animation())
            if form.rememberPassword {
                SecureField(form.hasSavedPassword ? "Saved — leave empty to keep" : "Password", text: $form.password)
                    .textContentType(.password)
            }
        } header: {
            Text("Password")
        } footer: {
            Text("Stored in the iOS Keychain on this device only. Authenticator codes are never stored.")
        }
    }

    private func warning(_ text: String) -> some View {
        Section {
            Label(text, systemImage: "exclamationmark.triangle")
                .font(.subheadline)
                .foregroundStyle(Palette.onBrassContainer)
                .listRowBackground(Palette.brassContainer)
        }
    }
}
