package app.tunnelkey.ovpn3

import java.io.Closeable

/**
 * Thin Kotlin wrapper around the OpenVPN 3 ClientAPI (see `ovpn3_jni.cpp`).
 *
 * Lifecycle: [evaluate] the profile, [provideCredentials] if needed, then call
 * [connect] from a dedicated thread — it blocks until the session ends. [stop]
 * may be called from any thread. [close] releases native memory and must only
 * be called after [connect] has returned.
 */
class OpenVpnClient(callbacks: Callbacks) : Closeable {

    /**
     * Called by the core on its own threads. `tun*` methods mirror
     * `android.net.VpnService.Builder`; returning `false` aborts the connection.
     */
    interface Callbacks {
        fun onEvent(name: String, info: String, error: Boolean, fatal: Boolean)
        fun onLog(text: String)
        fun protectSocket(fd: Int): Boolean

        fun tunNew(): Boolean
        fun tunAddAddress(address: String, prefixLength: Int, ipv6: Boolean): Boolean
        fun tunRerouteGateway(ipv4: Boolean, ipv6: Boolean): Boolean
        fun tunAddRoute(address: String, prefixLength: Int, ipv6: Boolean): Boolean
        fun tunExcludeRoute(address: String, prefixLength: Int, ipv6: Boolean): Boolean
        fun tunAddDnsServer(address: String): Boolean
        fun tunAddSearchDomain(domain: String): Boolean
        fun tunSetMtu(mtu: Int): Boolean
        fun tunSetSessionName(name: String): Boolean
        fun tunSetAllowFamily(ipv6: Boolean, allow: Boolean): Boolean
        /** Returns the detached tun file descriptor, or -1 on failure. */
        fun tunEstablish(): Int
        fun tunTeardown(disconnect: Boolean)
    }

    data class Evaluation(
        val error: Boolean,
        val message: String,
        /** True when the profile does not need a username/password. */
        val autologin: Boolean,
        /** Non-empty when the profile declares `static-challenge`. */
        val staticChallenge: String,
        val staticChallengeEcho: Boolean,
        val profileName: String,
        val remoteHost: String,
        val remotePort: String,
        val remoteProto: String,
    )

    data class ConnectionInfo(
        val vpnIp4: String,
        val vpnIp6: String,
        val serverHost: String,
        val serverIp: String,
        val serverPort: String,
        val serverProto: String,
        val user: String,
    )

    class ConnectException(val status: String, message: String) : Exception(message)

    // Guards [handle]: stop()/stats may come from other threads while the
    // session thread closes the client. connect() is not guarded (it blocks
    // for the whole session and close() only runs after it returns).
    private val lock = Any()
    @Volatile private var handle: Long = nativeCreate(callbacks)

    private inline fun <T> withHandle(fallback: T, block: (Long) -> T): T = synchronized(lock) {
        if (handle == 0L) fallback else block(handle)
    }

    fun evaluate(
        profile: String,
        guiVersion: String,
        connectTimeoutSeconds: Int = 0,
        allowLocalLan: Boolean = false,
    ): Evaluation {
        val r = nativeEval(handle, profile.utf8(), guiVersion.utf8(), connectTimeoutSeconds, allowLocalLan)
        return Evaluation(
            error = r[0] == "1",
            message = r[1],
            autologin = r[2] == "1",
            staticChallenge = r[3],
            staticChallengeEcho = r[4] == "1",
            profileName = r[5],
            remoteHost = r[6],
            remotePort = r[7],
            remoteProto = r[8],
        )
    }

    /** Returns an error message, or null when the core accepted the credentials. */
    fun provideCredentials(username: String, password: String, response: String = ""): String? {
        val pw = password.utf8()
        val rsp = response.utf8()
        try {
            return nativeProvideCreds(handle, username.utf8(), pw, rsp)
        } finally {
            pw.fill(0)
            rsp.fill(0)
        }
    }

    /** Blocks until the session ends. Throws [ConnectException] on failure. */
    fun connect() {
        val result = nativeConnect(handle) ?: return
        val status = result.substringBefore('\n')
        val message = result.substringAfter('\n', "")
        throw ConnectException(status, message.ifEmpty { status })
    }

    /** Safe from any thread, also after [close] (then it does nothing). */
    fun stop() = withHandle(Unit) { nativeStop(it) }
    fun reconnect(seconds: Int) = withHandle(Unit) { nativeReconnect(it, seconds) }
    fun pause(reason: String) = withHandle(Unit) { nativePause(it, reason.utf8()) }
    fun resume() = withHandle(Unit) { nativeResume(it) }

    /** Returns (bytesIn, bytesOut); zeros once closed. */
    fun transportStats(): Pair<Long, Long> = withHandle(0L to 0L) {
        val s = nativeTransportStats(it)
        s[0] to s[1]
    }

    fun connectionInfo(): ConnectionInfo? = withHandle(null) {
        val r = nativeConnectionInfo(it)
        ConnectionInfo(r[0], r[1], r[2], r[3], r[4], r[5], r[6])
    }

    override fun close() = synchronized(lock) {
        if (handle != 0L) {
            nativeDestroy(handle)
            handle = 0L
        }
    }

    private fun String.utf8(): ByteArray = toByteArray(Charsets.UTF_8)

    companion object {
        init {
            System.loadLibrary("tunnelkey_ovpn3")
        }

        val coreVersion: String get() = nativeCoreVersion()

        @JvmStatic private external fun nativeCreate(callbacks: Callbacks): Long
        @JvmStatic private external fun nativeDestroy(handle: Long)
        @JvmStatic private external fun nativeEval(
            handle: Long, content: ByteArray, guiVersion: ByteArray, connTimeout: Int, allowLan: Boolean,
        ): Array<String>
        @JvmStatic private external fun nativeProvideCreds(
            handle: Long, username: ByteArray, password: ByteArray, response: ByteArray,
        ): String?
        @JvmStatic private external fun nativeConnect(handle: Long): String?
        @JvmStatic private external fun nativeStop(handle: Long)
        @JvmStatic private external fun nativeReconnect(handle: Long, seconds: Int)
        @JvmStatic private external fun nativePause(handle: Long, reason: ByteArray)
        @JvmStatic private external fun nativeResume(handle: Long)
        @JvmStatic private external fun nativeTransportStats(handle: Long): LongArray
        @JvmStatic private external fun nativeConnectionInfo(handle: Long): Array<String>
        @JvmStatic private external fun nativeCoreVersion(): String
    }
}
