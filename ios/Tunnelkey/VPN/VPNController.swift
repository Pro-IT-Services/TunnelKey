import Foundation
import NetworkExtension

enum TunnelPhase: Equatable {
    case disconnected, connecting, connected, reconnecting, disconnecting, failed

    var isActive: Bool { self == .connecting || self == .connected || self == .reconnecting }
}

/// Owns the single NETunnelProviderManager and mirrors its state for the UI.
/// The manager is re-pointed at whichever profile the user connects.
@MainActor
final class VPNController: ObservableObject {
    @Published private(set) var phase: TunnelPhase = .disconnected
    @Published private(set) var profileID: UUID?
    @Published private(set) var connectedAt: Date?
    @Published private(set) var bytesIn: Int = 0
    @Published private(set) var bytesOut: Int = 0
    @Published private(set) var vpnAddress = ""
    @Published var errorMessage: String?

    private var manager: NETunnelProviderManager?
    private var statusObserver: NSObjectProtocol?
    private var statsTask: Task<Void, Never>?
    private var userStopped = false

    private var tunnelBundleID: String {
        (Bundle.main.bundleIdentifier ?? "app.tunnelkey") + ".tunnel"
    }

    init() {
        statusObserver = NotificationCenter.default.addObserver(
            forName: .NEVPNStatusDidChange, object: nil, queue: .main
        ) { [weak self] note in
            guard let connection = note.object as? NEVPNConnection else { return }
            Task { @MainActor in self?.statusChanged(connection) }
        }
        Task { await load() }
    }

    deinit {
        if let statusObserver { NotificationCenter.default.removeObserver(statusObserver) }
    }

    func load() async {
        let managers = (try? await NETunnelProviderManager.loadAllFromPreferences()) ?? []
        manager = managers.first
        if let manager {
            readProfileID(from: manager)
            statusChanged(manager.connection)
        }
    }

    /// Password and code travel to the extension in the start options only.
    func connect(profile: Profile, password: String?, code: String?) async {
        errorMessage = nil
        userStopped = false
        do {
            let manager = self.manager ?? NETunnelProviderManager()
            let proto = NETunnelProviderProtocol()
            proto.providerBundleIdentifier = tunnelBundleID
            proto.serverAddress = profile.remote.isEmpty ? profile.name : profile.remote
            proto.providerConfiguration = [
                Shared.Config.profileID: profile.id.uuidString,
                Shared.Config.username: profile.username,
                Shared.Config.twoFactor: profile.twoFactor,
                Shared.Config.codePosition: profile.codePosition.rawValue,
                Shared.Config.codeLength: profile.codeLength,
            ]
            manager.protocolConfiguration = proto
            manager.localizedDescription = profile.name
            manager.isEnabled = true
            manager.isOnDemandEnabled = false

            try await manager.saveToPreferences()
            // Required after saving, otherwise the first start fails with "configuration is stale".
            try await manager.loadFromPreferences()
            self.manager = manager
            profileID = profile.id

            var options: [String: NSObject] = [:]
            if let password { options[Shared.StartOption.password] = password as NSString }
            if let code { options[Shared.StartOption.code] = code as NSString }
            phase = .connecting
            try manager.connection.startVPNTunnel(options: options)
        } catch {
            phase = .failed
            errorMessage = (error as? NEVPNError)?.code == .configurationReadWriteFailed
                ? "VPN permission is needed to connect."
                : error.localizedDescription
        }
    }

    func disconnect() {
        userStopped = true
        manager?.connection.stopVPNTunnel()
    }

    /// Removes the system VPN configuration when its profile is deleted.
    func forget(profileID id: UUID) async {
        guard profileID == id, let manager else { return }
        if phase.isActive { manager.connection.stopVPNTunnel() }
        try? await manager.removeFromPreferences()
        self.manager = nil
        profileID = nil
        phase = .disconnected
    }

    // MARK: - Status

    private func statusChanged(_ connection: NEVPNConnection) {
        let previous = phase
        switch connection.status {
        case .connecting: phase = previous == .connected ? .reconnecting : .connecting
        case .connected:
            phase = .connected
            connectedAt = connection.connectedDate
            errorMessage = nil
            startStats()
        case .reasserting: phase = .reconnecting
        case .disconnecting: phase = .disconnecting
        case .disconnected, .invalid:
            stopStats()
            connectedAt = nil
            if previous.isActive || previous == .disconnecting, !userStopped {
                phase = .failed
                fetchDisconnectReason(connection)
            } else if phase != .failed {
                phase = .disconnected
            }
        @unknown default:
            break
        }
    }

    private func fetchDisconnectReason(_ connection: NEVPNConnection) {
        connection.fetchLastDisconnectError { [weak self] error in
            Task { @MainActor in
                guard let self else { return }
                if let error {
                    self.errorMessage = error.localizedDescription
                } else {
                    self.phase = .disconnected
                }
            }
        }
    }

    private func readProfileID(from manager: NETunnelProviderManager) {
        let config = (manager.protocolConfiguration as? NETunnelProviderProtocol)?.providerConfiguration
        profileID = (config?[Shared.Config.profileID] as? String).flatMap(UUID.init(uuidString:))
    }

    // MARK: - Traffic counters

    private func startStats() {
        statsTask?.cancel()
        statsTask = Task { [weak self] in
            while !Task.isCancelled {
                await self?.pollStats()
                try? await Task.sleep(nanoseconds: 1_000_000_000)
            }
        }
    }

    private func stopStats() {
        statsTask?.cancel()
        statsTask = nil
    }

    private func pollStats() async {
        guard let session = manager?.connection as? NETunnelProviderSession else { return }
        let data: Data? = await withCheckedContinuation { continuation in
            do {
                try session.sendProviderMessage(Data()) { continuation.resume(returning: $0) }
            } catch {
                continuation.resume(returning: nil)
            }
        }
        guard let data,
              let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else { return }
        bytesIn = json["bytesIn"] as? Int ?? bytesIn
        bytesOut = json["bytesOut"] as? Int ?? bytesOut
        vpnAddress = json["vpnAddress"] as? String ?? vpnAddress
    }
}
