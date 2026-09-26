import Foundation

struct TotpParams: Codable {
    let digits: Int
    let period: Int
    let algorithm: String
}

/// Non-secret part of a provisioned configuration.
struct ManagedConfig: Codable {
    let name: String
    let profileID: UUID
    let username: String
    let hasPassword: Bool
    let totp: TotpParams?
    let manualCode: Bool
    let links: [SetupLink]

    /// Password or TOTP secret stored: the app must be locked.
    var lockRequired: Bool { hasPassword || totp != nil }
}

enum LockChoice {
    case none
    case biometric
    case pin(String)
}

/// Single-configuration mode: owns the provisioned config, its encrypted
/// secrets and the in-memory unlocked session.
@MainActor
final class ManagedController: ObservableObject {
    @Published private(set) var config: ManagedConfig?
    /// Decrypted secrets while unlocked; nil when locked.
    @Published private(set) var secrets: VaultSecrets?
    @Published private(set) var lockMethod: LockMethod

    let vault = Vault()
    private let fileURL: URL

    init() {
        let dir = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        fileURL = dir.appendingPathComponent("managed.json")
        lockMethod = vault.method
        config = (try? Data(contentsOf: fileURL)).flatMap { try? JSONDecoder().decode(ManagedConfig.self, from: $0) }
        // Keychain items survive app deletion; drop secrets that no longer have a config.
        if config == nil && vault.exists {
            vault.wipe()
            lockMethod = .none
        }
        unlockIfUnprotected()
    }

    var needsUnlock: Bool { config != nil && lockMethod != .none && secrets == nil }

    // MARK: Provisioning

    func install(_ payload: SetupPayload, lock: LockChoice, store: ProfileStore) async throws {
        let secrets = VaultSecrets(
            password: (payload.password ?? "").isEmpty ? nil : payload.password,
            totpSecret: payload.totp?.secret
        )
        switch lock {
        case .none: vault.storeUnprotected(secrets)
        case .biometric: try vault.storeWithBiometric(secrets)
        case .pin(let pin):
            try await Task.detached(priority: .userInitiated) { [vault] in
                try vault.storeWithPin(pin, secrets: secrets)
            }.value
        }

        if let old = config { store.delete(old.profileID) }
        let summary = OvpnInspector.inspect(payload.ovpn)
        let profile = Profile(
            id: UUID(),
            name: payload.name,
            remote: summary.remote,
            username: payload.username,
            needsCredentials: summary.needsCredentials,
            twoFactor: payload.totp != nil || payload.manualCode,
            codePosition: payload.codePosition == "b" ? .beforePassword : .afterPassword,
            codeLength: payload.totp?.digits ?? 6,
            rememberPassword: false,
            managed: true
        )
        store.save(profile, content: payload.ovpn, password: nil)

        let cfg = ManagedConfig(
            name: payload.name,
            profileID: profile.id,
            username: payload.username,
            hasPassword: secrets.password != nil,
            totp: payload.totp.map { TotpParams(digits: $0.digits, period: $0.period, algorithm: $0.algorithm) },
            manualCode: payload.totp == nil && payload.manualCode,
            links: payload.links
        )
        try JSONEncoder().encode(cfg).write(to: fileURL, options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
        self.secrets = secrets
        lockMethod = vault.method
        config = cfg
    }

    func remove(store: ProfileStore) {
        if let cfg = config { store.delete(cfg.profileID) }
        try? FileManager.default.removeItem(at: fileURL)
        vault.wipe()
        secrets = nil
        config = nil
        lockMethod = .none
    }

    // MARK: Changing the lock (must be unlocked)

    func changeLock(to lock: LockChoice) async throws {
        guard let s = secrets else { return }
        switch lock {
        case .none: vault.storeUnprotected(s)
        case .biometric: try vault.storeWithBiometric(s)
        case .pin(let pin):
            try await Task.detached(priority: .userInitiated) { [vault] in
                try vault.storeWithPin(pin, secrets: s)
            }.value
        }
        lockMethod = vault.method
    }

    // MARK: Lock / unlock

    func unlockWithBiometric() async throws {
        secrets = try await vault.unlockWithBiometric(reason: "Unlock \(config?.name ?? "Tunnelkey")")
    }

    func unlockWithPin(_ pin: String, store: ProfileStore) async -> PinResult {
        let result = await Task.detached(priority: .userInitiated) { [vault] in vault.unlockWithPin(pin) }.value
        switch result {
        case .unlocked(let s): secrets = s
        case .wiped: remove(store: store)
        default: break
        }
        return result
    }

    func unlockIfUnprotected() {
        if config != nil, lockMethod == .none, secrets == nil {
            secrets = vault.unlockUnprotected()
        }
    }

    func lock() {
        if lockMethod != .none { secrets = nil }
    }

    // MARK: Connecting

    var password: String? { secrets?.password }

    var totp: Totp? {
        guard let params = config?.totp, let secret = secrets?.totpSecret, let key = Totp.base32Decode(secret) else { return nil }
        return Totp(secret: key, digits: params.digits, period: params.period, algorithm: params.algorithm)
    }

    /// Current code, waiting for the next one when this one is about to expire.
    func freshCode() async -> String? {
        guard let totp else { return nil }
        let left = totp.secondsLeft()
        if left <= 3 {
            try? await Task.sleep(nanoseconds: UInt64(left) * 1_000_000_000 + 300_000_000)
        }
        return totp.code()
    }
}
