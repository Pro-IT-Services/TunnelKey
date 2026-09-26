import Foundation

/// A picked .ovpn file that has not been saved yet.
struct ImportDraft: Identifiable {
    let id = UUID()
    var suggestedName: String
    var content: String
    var summary: OvpnSummary
}

enum ImportError: LocalizedError {
    case unreadable
    case tooLarge
    case notAProfile

    var errorDescription: String? {
        switch self {
        case .unreadable: return "The file could not be opened."
        case .tooLarge: return "This file is too large to be an OpenVPN profile."
        case .notAProfile: return "This doesn't look like an OpenVPN client profile (no “remote” found)."
        }
    }
}

/// Profiles live in the App Group container (shared with the tunnel
/// extension): one `<id>.ovpn` per profile plus a JSON index with the user's
/// settings. Excluded from backups.
@MainActor
final class ProfileStore: ObservableObject {
    @Published private(set) var profiles: [Profile] = []

    private let directory: URL
    private var indexURL: URL { directory.appendingPathComponent("index.json") }

    init() {
        directory = Shared.profilesDirectory
        try? FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        var dir = directory
        var values = URLResourceValues()
        values.isExcludedFromBackup = true
        try? dir.setResourceValues(values)
        load()
    }

    func profile(_ id: UUID?) -> Profile? {
        profiles.first { $0.id == id }
    }

    func config(for id: UUID) -> String? {
        try? String(contentsOf: Shared.profileURL(id.uuidString), encoding: .utf8)
    }

    /// Reads a file picked in the document picker or opened from another app.
    func draft(from url: URL) throws -> ImportDraft {
        let scoped = url.startAccessingSecurityScopedResource()
        defer { if scoped { url.stopAccessingSecurityScopedResource() } }

        guard let data = try? Data(contentsOf: url) else { throw ImportError.unreadable }
        guard data.count <= OvpnInspector.maxProfileBytes else { throw ImportError.tooLarge }
        guard var content = String(data: data, encoding: .utf8) ?? String(data: data, encoding: .isoLatin1) else {
            throw ImportError.unreadable
        }
        if content.hasPrefix("\u{FEFF}") { content.removeFirst() }
        let summary = OvpnInspector.inspect(content)
        guard summary.isValid else { throw ImportError.notAProfile }
        let name = url.deletingPathExtension().lastPathComponent
        return ImportDraft(suggestedName: name.isEmpty ? "Profile" : name, content: content, summary: summary)
    }

    /// Saves a new profile (when `content` is given) or updates an existing one.
    @discardableResult
    func save(_ profile: Profile, content: String?, password: String?) -> Profile {
        if let content {
            // Readable after first unlock so the tunnel can start while the phone is locked.
            try? Data(content.utf8).write(
                to: Shared.profileURL(profile.id.uuidString),
                options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication]
            )
        }
        if !profile.rememberPassword {
            Keychain.delete(profile.id)
        } else if let password, !password.isEmpty {
            Keychain.set(password, for: profile.id)
        }

        var list = profiles.filter { $0.id != profile.id }
        list.append(profile)
        list.sort { $0.importedAt < $1.importedAt }
        write(list)
        return profile
    }

    func delete(_ id: UUID) {
        try? FileManager.default.removeItem(at: Shared.profileURL(id.uuidString))
        Keychain.delete(id)
        write(profiles.filter { $0.id != id })
    }

    private func load() {
        guard let data = try? Data(contentsOf: indexURL),
              let list = try? JSONDecoder().decode([Profile].self, from: data) else { return }
        profiles = list
    }

    private func write(_ list: [Profile]) {
        if let data = try? JSONEncoder().encode(list) {
            try? data.write(to: indexURL, options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
        }
        profiles = list
    }
}
