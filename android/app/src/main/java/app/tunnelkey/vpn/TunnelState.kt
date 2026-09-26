package app.tunnelkey.vpn

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

enum class Phase { Disconnected, Connecting, Connected, Reconnecting, Disconnecting, Failed }

enum class FailureKind {
    /** Server rejected username/password/code. With 2FA this is usually an expired code. */
    AuthFailed,
    /** Connection dropped and the server wants credentials again (e.g. auth-nocache). */
    NeedsSignIn,
    /** Profile could not be parsed or needs something we cannot provide. */
    Profile,
    /** VPN permission revoked, or another VPN app took over. */
    Revoked,
    Other,
}

data class TunnelStatus(
    val phase: Phase = Phase.Disconnected,
    val profileId: String? = null,
    /** Last core event name, e.g. "WAIT", "AUTH", "GET_CONFIG". */
    val step: String = "",
    val failure: FailureKind? = null,
    val message: String = "",
    val connectedAt: Long? = null,
    val bytesIn: Long = 0,
    val bytesOut: Long = 0,
    val vpnAddress: String = "",
    val server: String = "",
) {
    val isActive: Boolean get() = phase == Phase.Connecting || phase == Phase.Connected || phase == Phase.Reconnecting
}

data class LogLine(val time: String, val text: String)

/** Process-wide tunnel state shared between [TunnelService] and the UI. */
object TunnelState {
    private const val MAX_LOG_LINES = 800
    private val timeFormat = SimpleDateFormat("HH:mm:ss", Locale.ROOT)

    private val _status = MutableStateFlow(TunnelStatus())
    val status: StateFlow<TunnelStatus> = _status.asStateFlow()

    private val _log = MutableStateFlow<List<LogLine>>(emptyList())
    val log: StateFlow<List<LogLine>> = _log.asStateFlow()

    fun update(transform: (TunnelStatus) -> TunnelStatus) = _status.update(transform)

    fun log(text: String) {
        val stamp = synchronized(timeFormat) { timeFormat.format(Date()) }
        val lines = text.trimEnd().lines().map { LogLine(stamp, it) }
        _log.update { (it + lines).takeLast(MAX_LOG_LINES) }
    }

    fun clearLog() {
        _log.value = emptyList()
    }
}
