import CommonCrypto
import CryptoKit
import Foundation
import LocalAuthentication
import Security

enum LockMethod: String, Codable {
    case none, biometric, pin
}

/// Secrets of the provisioned configuration. Only ever held decrypted in memory.
struct VaultSecrets: Codable {
    var password: String?
    var totpSecret: String?
}

enum PinResult {
    case unlocked(VaultSecrets)
    case wrong(attemptsLeft: Int, lockedUntil: Date)
    case lockedOut(until: Date)
    /// Too many wrong PINs: the configuration was erased.
    case wiped
}

enum VaultError: LocalizedError {
    case cancelled
    case biometryChanged
    case keychain(OSStatus)

    var errorDescription: String? {
        switch self {
        case .cancelled: return nil
        case .biometryChanged:
            return "Face ID / Touch ID on this phone changed, so the saved secrets can't be unlocked anymore. Scan the setup code again."
        case .keychain(let status):
            return SecCopyErrorMessageString(status, nil) as String? ?? "Keychain error \(status)"
        }
    }
}

/// Encrypted storage for the provisioned password and TOTP secret, in the
/// Keychain (this device only, never backed up or synced).
///
///  - Biometric: the item carries `.biometryCurrentSet` access control, so
///    iOS only releases it after Face ID / Touch ID, and it becomes unreadable
///    when the enrolled biometrics change.
///  - PIN: AES-GCM with a key derived from the PIN (PBKDF2-SHA256), stored in
///    the Keychain. Wrong PINs cause growing delays; the 10th erases it.
///  - None: plain Keychain item (only when nothing secret is stored).
final class Vault {
    static let maxAttempts = 10
    private static let service = "app.tunnelkey.vault"
    private static let pbkdf2Rounds: UInt32 = 310_000

    private struct State: Codable {
        var method: LockMethod
        var salt: Data?
        var failures = 0
        var lockedUntil: Date?
    }

    private var state: State? {
        get { read(account: "state").flatMap { try? JSONDecoder().decode(State.self, from: $0) } }
        set {
            if let newValue, let data = try? JSONEncoder().encode(newValue) {
                write(account: "state", data: data, access: nil)
            } else {
                delete(account: "state")
            }
        }
    }

    var method: LockMethod { state?.method ?? .none }
    var exists: Bool { state != nil }
    var lockedUntil: Date? { state?.lockedUntil }

    static var biometryAvailable: Bool {
        LAContext().canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: nil)
    }

    static var biometryName: String {
        let context = LAContext()
        _ = context.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: nil)
        return context.biometryType == .touchID ? "Touch ID" : "Face ID"
    }

    // MARK: Store

    func storeUnprotected(_ secrets: VaultSecrets) {
        write(account: "secrets", data: encode(secrets), access: nil)
        state = State(method: .none)
    }

    func storeWithPin(_ pin: String, secrets: VaultSecrets) throws {
        var salt = Data(count: 16)
        _ = salt.withUnsafeMutableBytes { SecRandomCopyBytes(kSecRandomDefault, 16, $0.baseAddress!) }
        let sealed = try AES.GCM.seal(encode(secrets), using: pinKey(pin, salt: salt)).combined!
        write(account: "secrets", data: sealed, access: nil)
        state = State(method: .pin, salt: salt)
    }

    func storeWithBiometric(_ secrets: VaultSecrets) throws {
        var error: Unmanaged<CFError>?
        guard let access = SecAccessControlCreateWithFlags(
            nil, kSecAttrAccessibleWhenPasscodeSetThisDeviceOnly, .biometryCurrentSet, &error
        ) else {
            throw error!.takeRetainedValue() as Error
        }
        let status = write(account: "secrets", data: encode(secrets), access: access)
        guard status == errSecSuccess else { throw VaultError.keychain(status) }
        state = State(method: .biometric)
    }

    // MARK: Unlock

    func unlockUnprotected() -> VaultSecrets? {
        read(account: "secrets").flatMap(decode)
    }

    /// Shows Face ID / Touch ID and returns the secrets.
    func unlockWithBiometric(reason: String) async throws -> VaultSecrets {
        let context = LAContext()
        context.localizedReason = reason
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: Self.service,
            kSecAttrAccount as String: "secrets",
            kSecReturnData as String: true,
            kSecUseAuthenticationContext as String: context,
        ]
        // SecItemCopyMatching blocks while the system sheet is up.
        let (status, data): (OSStatus, Data?) = await Task.detached {
            var result: AnyObject?
            let status = SecItemCopyMatching(query as CFDictionary, &result)
            return (status, result as? Data)
        }.value
        switch status {
        case errSecSuccess:
            guard let data, let secrets = decode(data) else { throw VaultError.keychain(errSecDecode) }
            return secrets
        case errSecUserCanceled, errSecAuthFailed:
            throw VaultError.cancelled
        case errSecItemNotFound:
            throw VaultError.biometryChanged
        default:
            throw VaultError.keychain(status)
        }
    }

    func unlockWithPin(_ pin: String, now: Date = Date()) -> PinResult {
        guard var st = state, let salt = st.salt, let sealed = read(account: "secrets") else { return .wiped }
        if let until = st.lockedUntil, now < until { return .lockedOut(until: until) }
        do {
            let box = try AES.GCM.SealedBox(combined: sealed)
            let plain = try AES.GCM.open(box, using: pinKey(pin, salt: salt))
            st.failures = 0
            st.lockedUntil = nil
            state = st
            guard let secrets = decode(plain) else { return .wiped }
            return .unlocked(secrets)
        } catch {
            st.failures += 1
            if st.failures >= Self.maxAttempts {
                wipe()
                return .wiped
            }
            let until = now.addingTimeInterval(Self.delay(after: st.failures))
            st.lockedUntil = until
            state = st
            return .wrong(attemptsLeft: Self.maxAttempts - st.failures, lockedUntil: until)
        }
    }

    func wipe() {
        delete(account: "secrets")
        delete(account: "state")
    }

    /// No delay for the first 4 mistakes, then 30 s, 1 min, 5 min, 15 min, 1 h.
    static func delay(after failures: Int) -> TimeInterval {
        switch failures {
        case ..<5: return 0
        case 5: return 30
        case 6: return 60
        case 7: return 5 * 60
        case 8: return 15 * 60
        default: return 60 * 60
        }
    }

    // MARK: Internals

    private func encode(_ s: VaultSecrets) -> Data { (try? JSONEncoder().encode(s)) ?? Data() }
    private func decode(_ d: Data) -> VaultSecrets? { try? JSONDecoder().decode(VaultSecrets.self, from: d) }

    private func pinKey(_ pin: String, salt: Data) -> SymmetricKey {
        var derived = Data(count: 32)
        let pinBytes = Array(pin.utf8)
        derived.withUnsafeMutableBytes { out in
            salt.withUnsafeBytes { saltPtr in
                _ = CCKeyDerivationPBKDF(
                    CCPBKDFAlgorithm(kCCPBKDF2),
                    pin, pinBytes.count,
                    saltPtr.bindMemory(to: UInt8.self).baseAddress, salt.count,
                    CCPseudoRandomAlgorithm(kCCPRFHmacAlgSHA256), Self.pbkdf2Rounds,
                    out.bindMemory(to: UInt8.self).baseAddress, 32
                )
            }
        }
        return SymmetricKey(data: derived)
    }

    @discardableResult
    private func write(account: String, data: Data, access: SecAccessControl?) -> OSStatus {
        delete(account: account)
        var query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: Self.service,
            kSecAttrAccount as String: account,
            kSecValueData as String: data,
        ]
        if let access {
            query[kSecAttrAccessControl as String] = access
        } else {
            query[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        }
        return SecItemAdd(query as CFDictionary, nil)
    }

    private func read(account: String) -> Data? {
        let context = LAContext()
        context.interactionNotAllowed = true // never prompt for these reads
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: Self.service,
            kSecAttrAccount as String: account,
            kSecReturnData as String: true,
            kSecUseAuthenticationContext as String: context,
        ]
        var result: AnyObject?
        guard SecItemCopyMatching(query as CFDictionary, &result) == errSecSuccess else { return nil }
        return result as? Data
    }

    private func delete(account: String) {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: Self.service,
            kSecAttrAccount as String: account,
        ]
        SecItemDelete(query as CFDictionary)
    }
}
