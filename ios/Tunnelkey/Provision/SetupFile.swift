import Combine
import CommonCrypto
import CryptoKit
import Foundation
import UniformTypeIdentifiers

// Reader for password-encrypted setup files (.tunnelkey) made by the setup page.
// Format: docs/provisioning-format.md, "Setup file format (v1)"

enum SetupFileError: LocalizedError, Equatable {
    case notASetupFile, newerVersion, wrongPassword, damaged, tooLarge

    var errorDescription: String? {
        switch self {
        case .notASetupFile: return "This file isn't a Tunnelkey setup file."
        case .newerVersion: return "This setup file needs a newer version of Tunnelkey."
        case .wrongPassword: return "Wrong password, or the file was modified."
        case .damaged: return "The setup file is damaged."
        case .tooLarge: return "The setup file is too large."
        }
    }
}

/// A parsed setup file, still encrypted.
struct SetupFile {
    static let maxFileBytes = 1024 * 1024
    static let iterationRange = 100_000...10_000_000

    let iterations: Int
    let salt: Data
    let iv: Data
    /// Ciphertext with the 16-byte GCM tag appended.
    let sealed: Data
    /// Authenticated data; binds the KDF parameters to the ciphertext.
    let aad: Data

    private struct Envelope: Decodable {
        struct Kdf: Decodable {
            let alg: String
            let iter: Int
            let salt: String
        }
        struct Enc: Decodable {
            let alg: String
            let iv: String
        }
        let v: Int
        let kdf: Kdf
        let enc: Enc
        let data: String
    }

    private struct Magic: Decodable {
        let tunnelkey: String?
    }

    /// Throws `.notASetupFile` for anything that isn't a setup file (an .ovpn
    /// profile, other JSON, …), so callers can route the file elsewhere.
    init(data: Data) throws {
        guard let magic = try? JSONDecoder().decode(Magic.self, from: data), magic.tunnelkey == "setup-file" else {
            throw SetupFileError.notASetupFile
        }
        guard data.count <= Self.maxFileBytes else { throw SetupFileError.tooLarge }
        guard let env = try? JSONDecoder().decode(Envelope.self, from: data) else { throw SetupFileError.damaged }
        guard env.v == 1 else { throw env.v > 1 ? SetupFileError.newerVersion : SetupFileError.damaged }
        guard env.kdf.alg == "PBKDF2-SHA256", env.enc.alg == "A256GCM",
              Self.iterationRange.contains(env.kdf.iter),
              let salt = Self.base64URLDecode(env.kdf.salt), salt.count >= 16,
              let iv = Self.base64URLDecode(env.enc.iv), iv.count == 12,
              let sealed = Self.base64URLDecode(env.data), sealed.count >= 16 else {
            throw SetupFileError.damaged
        }
        iterations = env.kdf.iter
        self.salt = salt
        self.iv = iv
        self.sealed = sealed
        aad = Data("tunnelkey-setup-file:1:\(env.kdf.iter):\(env.kdf.salt):\(env.enc.iv)".utf8)
    }

    /// Slow on purpose (PBKDF2 with hundreds of thousands of rounds): call off the main thread.
    func decrypt(password: String) throws -> SetupPayload {
        guard !password.isEmpty else { throw SetupFileError.wrongPassword }
        let key = try Self.deriveKey(password: password, salt: salt, iterations: iterations)

        let plaintext: Data
        do {
            let box = try AES.GCM.SealedBox(
                nonce: try AES.GCM.Nonce(data: iv),
                ciphertext: Data(sealed.prefix(sealed.count - 16)),
                tag: Data(sealed.suffix(16))
            )
            plaintext = try AES.GCM.open(box, using: key, authenticating: aad)
        } catch {
            // A wrong password and a modified file both fail the tag check.
            throw SetupFileError.wrongPassword
        }

        do {
            return try SetupCodes.payload(fromZlib: plaintext)
        } catch SetupCodeError.newerVersion {
            throw SetupFileError.newerVersion
        } catch SetupCodeError.tooLarge {
            throw SetupFileError.tooLarge
        } catch {
            throw SetupFileError.damaged
        }
    }

    /// PBKDF2-HMAC-SHA256 over the NFC-normalised UTF-8 password, 32-byte key.
    static func deriveKey(password: String, salt: Data, iterations: Int) throws -> SymmetricKey {
        let pw = password.precomposedStringWithCanonicalMapping.utf8CString // NUL-terminated
        let saltBytes = [UInt8](salt)
        var derived = [UInt8](repeating: 0, count: 32)
        defer { for i in derived.indices { derived[i] = 0 } }
        let status = pw.withUnsafeBufferPointer { p in
            saltBytes.withUnsafeBufferPointer { s in
                derived.withUnsafeMutableBufferPointer { d in
                    CCKeyDerivationPBKDF(
                        CCPBKDFAlgorithm(kCCPBKDF2),
                        p.baseAddress, p.count - 1,
                        s.baseAddress, s.count,
                        CCPseudoRandomAlgorithm(kCCPRFHmacAlgSHA256),
                        UInt32(iterations),
                        d.baseAddress, d.count
                    )
                }
            }
        }
        guard status == kCCSuccess else { throw SetupFileError.damaged }
        return SymmetricKey(data: derived)
    }

    /// RFC 4648 §5 base64url without padding; nil for anything else.
    static func base64URLDecode(_ s: String) -> Data? {
        let allowed = Set("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_")
        guard !s.isEmpty, s.count % 4 != 1, s.allSatisfy(allowed.contains) else { return nil }
        var b64 = s.replacingOccurrences(of: "-", with: "+").replacingOccurrences(of: "_", with: "/")
        b64 += String(repeating: "=", count: (4 - b64.count % 4) % 4)
        return Data(base64Encoded: b64)
    }
}

/// A setup file waiting for its password.
struct SetupFileRequest: Identifiable {
    let id = UUID()
    let file: SetupFile
    let fileName: String
}

/// Files opened from other apps (Files, Mail, AirDrop, …) or picked in the
/// importer, routed by content: setup files to the setup flow, everything else
/// to the .ovpn importer.
@MainActor
final class FileRouter: ObservableObject {
    @Published var setupFile: SetupFileRequest?
    @Published var profileURL: URL?
    @Published var error: String?

    /// `acceptsProfiles` is false in single-config mode, where .ovpn profiles can't be added.
    func open(_ url: URL, acceptsProfiles: Bool) {
        let scoped = url.startAccessingSecurityScopedResource()
        defer { if scoped { url.stopAccessingSecurityScopedResource() } }

        let isSetupExtension = url.pathExtension.lowercased() == "tunnelkey"
        guard let data = try? Data(contentsOf: url, options: .mappedIfSafe) else {
            if isSetupExtension {
                error = "The file could not be opened."
            } else {
                route(profile: url, acceptsProfiles: acceptsProfiles)
            }
            return
        }
        do {
            let file = try SetupFile(data: data)
            setupFile = SetupFileRequest(file: file, fileName: url.lastPathComponent)
            removeInboxCopy(url)
        } catch SetupFileError.notASetupFile where !isSetupExtension {
            route(profile: url, acceptsProfiles: acceptsProfiles)
        } catch {
            self.error = error.localizedDescription
        }
    }

    private func route(profile url: URL, acceptsProfiles: Bool) {
        if acceptsProfiles {
            profileURL = url
        } else {
            error = "Tunnelkey is set up with a single configuration from your administrator. Remove it to add other profiles."
        }
    }

    /// Files opened from other apps are copied into Documents/Inbox; the
    /// encrypted copy isn't needed once it's been read.
    private func removeInboxCopy(_ url: URL) {
        guard let docs = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask).first else { return }
        let inbox = docs.appendingPathComponent("Inbox", isDirectory: true).standardizedFileURL.path + "/"
        if url.standardizedFileURL.path.hasPrefix(inbox) {
            try? FileManager.default.removeItem(at: url)
        }
    }
}

extension UTType {
    static let tunnelkeySetup = UTType(exportedAs: "com.proitservices.tunnelkey.setup", conformingTo: .json)
}
