import Foundation

struct Profile: Codable, Identifiable, Hashable {
    var id: UUID
    var name: String
    /// "host:port/proto" of the first remote, for display only.
    var remote: String
    var username: String = ""
    /// Profile contains `auth-user-pass`.
    var needsCredentials: Bool = true
    /// Server expects an authenticator (TOTP) code together with the password.
    var twoFactor: Bool = false
    var codePosition: CodePosition = .afterPassword
    var codeLength: Int = 6
    /// Password lives in the Keychain. The code never is stored.
    var rememberPassword: Bool = false
    var importedAt: Date = Date()
    /// Installed from a setup code; hidden from the profile list. Optional so
    /// profiles saved by older versions still decode.
    var managed: Bool? = nil

    var isManaged: Bool { managed == true }
}

/// Lightweight look at an .ovpn file, used to pre-fill the import form. The
/// OpenVPN core does the authoritative parsing when connecting.
struct OvpnSummary: Equatable {
    var remote: String
    var needsCredentials: Bool
    var staticChallenge: String?
    /// Directives that point at files outside the profile.
    var externalFiles: [String]
    var isValid: Bool
}

enum OvpnInspector {
    static let maxProfileBytes = 256 * 1024

    private static let fileDirectives: Set<String> = [
        "ca", "cert", "key", "tls-auth", "tls-crypt", "tls-crypt-v2", "pkcs12", "extra-certs", "crl-verify",
    ]

    static func inspect(_ content: String) -> OvpnSummary {
        var remote: String?
        var needsCredentials = false
        var staticChallenge: String?
        var sawClient = false
        var external: [String] = []
        var inlineBlock: String?

        for raw in content.split(whereSeparator: \.isNewline) {
            let line = raw.trimmingCharacters(in: .whitespaces)
            if line.isEmpty || line.hasPrefix("#") || line.hasPrefix(";") { continue }

            if let block = inlineBlock {
                if line.lowercased() == "</\(block)>" { inlineBlock = nil }
                continue
            }
            if line.hasPrefix("<") && line.hasSuffix(">") {
                let tag = line.trimmingCharacters(in: CharacterSet(charactersIn: "</>")).lowercased()
                // <connection> blocks hold ordinary directives (e.g. remote); read through them.
                if tag != "connection" && !line.hasPrefix("</") { inlineBlock = tag }
                if tag == "auth-user-pass" { needsCredentials = true }
                continue
            }

            let parts = tokenize(line)
            guard let first = parts.first else { continue }
            switch first.lowercased() {
            case "client", "tls-client":
                sawClient = true
            case "remote" where remote == nil && parts.count >= 2:
                let port = parts.count > 2 ? parts[2] : "1194"
                remote = parts.count > 3 ? "\(parts[1]):\(port)/\(parts[3])" : "\(parts[1]):\(port)"
            case "auth-user-pass":
                needsCredentials = true
            case "static-challenge":
                staticChallenge = parts.count > 1 ? parts[1] : ""
            case let d where fileDirectives.contains(d) && parts.count >= 2:
                external.append(parts[1])
            default:
                break
            }
        }

        return OvpnSummary(
            remote: remote ?? "",
            needsCredentials: needsCredentials,
            staticChallenge: staticChallenge,
            externalFiles: external,
            isValid: remote != nil && (sawClient || content.range(of: "<ca>", options: .caseInsensitive) != nil)
        )
    }

    /// Splits a directive line, honouring double quotes.
    static func tokenize(_ line: String) -> [String] {
        var out: [String] = []
        var current = ""
        var quoted = false
        var escaped = false
        for c in line {
            if escaped {
                current.append(c)
                escaped = false
            } else if c == "\\" && quoted {
                escaped = true
            } else if c == "\"" {
                quoted.toggle()
            } else if c.isWhitespace && !quoted {
                if !current.isEmpty { out.append(current); current = "" }
            } else {
                current.append(c)
            }
        }
        if !current.isEmpty { out.append(current) }
        return out
    }
}
