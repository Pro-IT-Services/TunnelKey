import Foundation

/// Values shared by the app and the packet tunnel extension.
enum Shared {
    /// Must match the App Group in both entitlements files.
    static let appGroup = "group.app.tunnelkey"

    /// Profiles are stored in the App Group container so the tunnel extension
    /// can read them; the VPN preferences only carry the profile ID.
    static var profilesDirectory: URL {
        let base = FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: appGroup)
            ?? FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
        return base.appendingPathComponent("Profiles", isDirectory: true)
    }

    static func profileURL(_ id: String) -> URL {
        profilesDirectory.appendingPathComponent("\(id).ovpn")
    }

    /// Keys of `NETunnelProviderProtocol.providerConfiguration`.
    enum Config {
        static let profileID = "profileID"
        static let username = "username"
        static let twoFactor = "twoFactor"
        static let codePosition = "codePosition"
        static let codeLength = "codeLength"
    }

    /// Keys of the `options` dictionary passed to `startVPNTunnel(options:)`.
    /// These are held in memory only and never written to disk.
    enum StartOption {
        static let password = "password"
        static let code = "code"
    }

    enum TunnelErrorCode: Int {
        case missingProfile = 1
        case needsSignIn
        case authFailed
        case authFailedTwoFactor
        case profileInvalid
        case other
    }

    static let tunnelErrorDomain = "app.tunnelkey.tunnel"
}

/// Where the one-time code goes relative to the password.
enum CodePosition: String, Codable, CaseIterable, Identifiable {
    case afterPassword
    case beforePassword

    var id: String { rawValue }
}

enum Credentials {
    /// Builds the password that servers with a TOTP plugin expect: the static
    /// password and the current authenticator code joined together, e.g.
    /// `hunter2` + `123456` → `hunter2123456`.
    static func combine(password: String, code: String, position: CodePosition) -> String {
        switch position {
        case .afterPassword: return password + code
        case .beforePassword: return code + password
        }
    }

    static func isValidCode(_ code: String, length: Int) -> Bool {
        code.count == length && code.allSatisfy { ("0"..."9").contains($0) }
    }
}

/// Small append-only log in the App Group container, written by the tunnel
/// extension and read by the app's log screen.
final class SharedLog {
    static let shared = SharedLog()

    private let queue = DispatchQueue(label: "app.tunnelkey.log")
    private let maxBytes = 256 * 1024
    private let formatter: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "HH:mm:ss"
        f.locale = Locale(identifier: "en_US_POSIX")
        return f
    }()

    var fileURL: URL? {
        FileManager.default
            .containerURL(forSecurityApplicationGroupIdentifier: Shared.appGroup)?
            .appendingPathComponent("tunnel.log")
    }

    func append(_ message: String) {
        queue.async { [self] in
            guard let url = fileURL else { return }
            let stamp = formatter.string(from: Date())
            let lines = message
                .split(whereSeparator: \.isNewline)
                .map { "\(stamp) \($0)\n" }
                .joined()
            guard let data = lines.data(using: .utf8) else { return }

            if let size = (try? FileManager.default.attributesOfItem(atPath: url.path)[.size]) as? Int,
               size > maxBytes,
               let existing = try? Data(contentsOf: url) {
                // Keep the newest half.
                try? existing.suffix(maxBytes / 2).write(to: url)
            }
            if let handle = try? FileHandle(forWritingTo: url) {
                handle.seekToEndOfFile()
                handle.write(data)
                try? handle.close()
            } else {
                try? data.write(to: url)
            }
        }
    }

    func read() -> [String] {
        guard let url = fileURL, let text = try? String(contentsOf: url, encoding: .utf8) else { return [] }
        return text.split(whereSeparator: \.isNewline).map(String.init)
    }

    func clear() {
        queue.async { [self] in
            if let url = fileURL { try? FileManager.default.removeItem(at: url) }
        }
    }
}
