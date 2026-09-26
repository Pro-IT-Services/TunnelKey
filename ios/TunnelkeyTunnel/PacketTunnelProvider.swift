import NetworkExtension
import OpenVPNAdapter

extension NEPacketTunnelFlow: OpenVPNAdapterPacketFlow {}

/// Runs one OpenVPN session. The profile is read from the App Group
/// container; the password and authenticator code arrive in the start options
/// and are only kept in memory.
final class PacketTunnelProvider: NEPacketTunnelProvider {

    private lazy var adapter: OpenVPNAdapter = {
        let adapter = OpenVPNAdapter()
        adapter.delegate = self
        return adapter
    }()

    private let reachability = OpenVPNReachability()
    private let log = SharedLog.shared

    private var startHandler: ((Error?) -> Void)?
    private var stopHandler: (() -> Void)?
    private var twoFactor = false

    override func startTunnel(options: [String: NSObject]?, completionHandler: @escaping (Error?) -> Void) {
        guard
            let proto = protocolConfiguration as? NETunnelProviderProtocol,
            let config = proto.providerConfiguration,
            let profileID = config[Shared.Config.profileID] as? String,
            let ovpn = try? String(contentsOf: Shared.profileURL(profileID), encoding: .utf8)
        else {
            completionHandler(tunnelError(.missingProfile, "The VPN profile is missing. Open Tunnelkey and connect again."))
            return
        }

        let username = config[Shared.Config.username] as? String ?? ""
        twoFactor = config[Shared.Config.twoFactor] as? Bool ?? false
        let position = CodePosition(rawValue: config[Shared.Config.codePosition] as? String ?? "") ?? .afterPassword
        let codeLength = config[Shared.Config.codeLength] as? Int ?? 6

        let configuration = OpenVPNConfiguration()
        configuration.fileContent = Data(ovpn.utf8)
        configuration.guiVersion = "Tunnelkey \(Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "")"
        configuration.tunPersist = true
        configuration.info = true

        let evaluation: OpenVPNConfigurationEvaluation
        do {
            evaluation = try adapter.apply(configuration: configuration)
        } catch {
            completionHandler(tunnelError(.profileInvalid, error.localizedDescription))
            return
        }

        if !evaluation.autologin {
            // Started from Settings or by the system: nobody typed a code.
            guard let password = options?[Shared.StartOption.password] as? String else {
                completionHandler(tunnelError(.needsSignIn, "Open Tunnelkey to sign in. Each connection needs your password and a fresh code."))
                return
            }
            let code = options?[Shared.StartOption.code] as? String ?? ""
            if twoFactor && !Credentials.isValidCode(code, length: codeLength) {
                completionHandler(tunnelError(.needsSignIn, "An authenticator code is required."))
                return
            }

            let credentials = OpenVPNCredentials()
            credentials.username = username
            if twoFactor, let challenge = evaluation.staticChallenge, !challenge.isEmpty {
                // static-challenge profiles: the core sends the code as the SCRV1 response.
                credentials.password = password
                credentials.response = code
            } else if twoFactor {
                // password+TOTP servers: one combined password.
                credentials.password = Credentials.combine(password: password, code: code, position: position)
            } else {
                credentials.password = password
            }

            do {
                try adapter.provide(credentials: credentials)
            } catch {
                completionHandler(tunnelError(.other, error.localizedDescription))
                return
            }
        }

        // Reconnect promptly when the device switches networks (Wi-Fi <-> cellular)
        // instead of waiting for keepalive timeouts. The first callback only
        // reports the current state.
        var lastStatus: OpenVPNReachabilityStatus?
        reachability.startTracking { [weak self] status in
            defer { lastStatus = status }
            guard let previous = lastStatus, previous != status, status != .notReachable else { return }
            self?.adapter.reconnect(afterTimeInterval: 2)
        }

        log.append("Connecting (\(evaluation.remoteHost ?? "?"))")
        startHandler = completionHandler
        adapter.connect(using: packetFlow)
    }

    override func stopTunnel(with reason: NEProviderStopReason, completionHandler: @escaping () -> Void) {
        log.append("Stopping (reason \(reason.rawValue))")
        stopHandler = completionHandler
        if reachability.isTracking {
            reachability.stopTracking()
        }
        adapter.disconnect()
    }

    /// The app polls this for live traffic counters.
    override func handleAppMessage(_ messageData: Data, completionHandler: ((Data?) -> Void)?) {
        let stats = adapter.transportStatistics
        let info = adapter.connectionInformation
        let payload: [String: Any] = [
            "bytesIn": stats.bytesIn,
            "bytesOut": stats.bytesOut,
            "vpnAddress": info?.vpnIPv4 ?? info?.vpnIPv6 ?? "",
            "server": [info?.serverHost, info?.serverPort].compactMap { $0 }.joined(separator: ":"),
        ]
        completionHandler?(try? JSONSerialization.data(withJSONObject: payload))
    }

    private func tunnelError(_ code: Shared.TunnelErrorCode, _ message: String) -> NSError {
        log.append("Error: \(message)")
        return NSError(domain: Shared.tunnelErrorDomain, code: code.rawValue, userInfo: [NSLocalizedDescriptionKey: message])
    }
}

extension PacketTunnelProvider: OpenVPNAdapterDelegate {

    func openVPNAdapter(
        _ openVPNAdapter: OpenVPNAdapter,
        configureTunnelWithNetworkSettings networkSettings: NEPacketTunnelNetworkSettings?,
        completionHandler: @escaping (Error?) -> Void
    ) {
        // Send all DNS queries through the tunnel's resolvers.
        networkSettings?.dnsSettings?.matchDomains = [""]
        setTunnelNetworkSettings(networkSettings, completionHandler: completionHandler)
    }

    func openVPNAdapter(_ openVPNAdapter: OpenVPNAdapter, handleEvent event: OpenVPNAdapterEvent, message: String?) {
        log.append("EVENT \(event.rawValue)\(message.map { ": \($0)" } ?? "")")
        switch event {
        case .connected:
            if reasserting { reasserting = false }
            startHandler?(nil)
            startHandler = nil
        case .disconnected:
            if reachability.isTracking { reachability.stopTracking() }
            stopHandler?()
            stopHandler = nil
        case .reconnecting:
            reasserting = true
        default:
            break
        }
    }

    func openVPNAdapter(_ openVPNAdapter: OpenVPNAdapter, handleError error: Error) {
        let nsError = error as NSError
        let message = nsError.userInfo[OpenVPNAdapterErrorMessageKey] as? String ?? nsError.localizedDescription
        log.append("ERROR \(nsError.code): \(message)")

        guard nsError.userInfo[OpenVPNAdapterErrorFatalKey] as? Bool == true else { return }

        let reported: NSError
        if nsError.domain == OpenVPNAdapterErrorDomain && nsError.code == OpenVPNAdapterError.authFailed.rawValue {
            reported = twoFactor
                ? tunnelError(.authFailedTwoFactor, "The server rejected the sign-in. Authenticator codes expire every 30 seconds — try again with a fresh code.")
                : tunnelError(.authFailed, "The server rejected your username or password.")
        } else {
            reported = tunnelError(.other, message)
        }

        if reachability.isTracking { reachability.stopTracking() }
        if let handler = startHandler {
            handler(reported)
            startHandler = nil
        } else {
            cancelTunnelWithError(reported)
        }
    }

    func openVPNAdapter(_ openVPNAdapter: OpenVPNAdapter, handleLogMessage logMessage: String) {
        log.append(logMessage)
    }
}
