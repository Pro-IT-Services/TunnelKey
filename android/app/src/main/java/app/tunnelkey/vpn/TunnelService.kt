package app.tunnelkey.vpn

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.net.IpPrefix
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat
import app.tunnelkey.BuildConfig
import app.tunnelkey.R
import app.tunnelkey.TunnelkeyApp
import app.tunnelkey.data.Credentials
import app.tunnelkey.data.Profile
import app.tunnelkey.ovpn3.OpenVpnClient
import app.tunnelkey.ui.MainActivity
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import java.net.InetAddress

/**
 * Runs one OpenVPN 3 session on a worker thread and exposes it to Android as a
 * VpnService. Credentials arrive in the start intent and are only held in
 * memory for as long as the core needs them.
 */
class TunnelService : VpnService(), OpenVpnClient.Callbacks {

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    private var statsJob: Job? = null

    @Volatile private var client: OpenVpnClient? = null
    @Volatile private var worker: Thread? = null
    @Volatile private var profile: Profile? = null
    @Volatile private var userStopped = false

    // Tun configuration being assembled by the core.
    private var builder: Builder? = null
    private var hasIpv6Address = false
    private var blockIpv6 = false

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_CONNECT -> {
                val id = intent.getStringExtra(EXTRA_PROFILE_ID) ?: return START_NOT_STICKY
                val password = intent.getStringExtra(EXTRA_PASSWORD)
                val code = intent.getStringExtra(EXTRA_CODE)
                intent.removeExtra(EXTRA_PASSWORD)
                intent.removeExtra(EXTRA_CODE)
                startTunnel(id, password, code)
            }
            ACTION_DISCONNECT -> if (worker?.isAlive == true) stopTunnel() else stopSelf()
            // System-initiated start (always-on VPN). Profiles with 2FA need
            // the user to type a code, so there is nothing we can do here.
            else -> {
                TunnelState.update {
                    it.copy(
                        phase = Phase.Failed,
                        failure = FailureKind.Other,
                        message = getString(R.string.error_always_on_unsupported),
                    )
                }
                stopSelf()
            }
        }
        return START_NOT_STICKY
    }

    override fun onRevoke() {
        TunnelState.update { it.copy(failure = FailureKind.Revoked) }
        stopTunnel()
        super.onRevoke()
    }

    override fun onDestroy() {
        stopTunnel()
        scope.cancel()
        super.onDestroy()
    }

    // ---------------------------------------------------------------------

    private fun startTunnel(profileId: String, password: String?, code: String?) {
        val repo = (application as TunnelkeyApp).profiles
        val p = repo.get(profileId) ?: return
        promoteToForeground(p.name)

        // Replace any running session.
        val previous = worker
        stopCore()
        userStopped = false
        profile = p

        TunnelState.update {
            TunnelStatus(phase = Phase.Connecting, profileId = p.id, server = p.remote)
        }
        TunnelState.log("Connecting to ${p.name} (${p.remote})")

        worker = Thread({
            previous?.join(5_000)
            runSession(p, repo.readConfig(p.id), password, code)
        }, "tunnelkey-openvpn").also { it.start() }
    }

    private fun runSession(p: Profile, config: String, password: String?, code: String?) {
        val c = OpenVpnClient(this)
        client = c
        var failure: FailureKind? = null
        var message = ""
        try {
            val eval = c.evaluate(config, "Tunnelkey ${BuildConfig.VERSION_NAME}")
            if (eval.error) {
                failure = FailureKind.Profile
                message = eval.message
                return
            }

            if (!eval.autologin) {
                val pw = password.orEmpty()
                val otp = code.orEmpty()
                if (p.twoFactor && !Credentials.isValidCode(otp, p.codeLength)) {
                    failure = FailureKind.AuthFailed
                    message = getString(R.string.error_code_missing)
                    return
                }
                val err = when {
                    // Profile uses OpenVPN's static-challenge: the core sends the
                    // code in the dedicated SCRV1 response field.
                    p.twoFactor && eval.staticChallenge.isNotEmpty() ->
                        c.provideCredentials(p.username, pw, response = otp)
                    // Classic "password+TOTP" servers: one combined password.
                    p.twoFactor -> c.provideCredentials(p.username, Credentials.combine(pw, otp, p.codePosition))
                    else -> c.provideCredentials(p.username, pw)
                }
                if (err != null) {
                    failure = FailureKind.Other
                    message = err
                    return
                }
            }

            if (userStopped) return // cancelled while we were setting up
            c.connect() // blocks until the session ends
        } catch (e: OpenVpnClient.ConnectException) {
            if (!userStopped) {
                failure = if (e.status.contains("auth", ignoreCase = true)) FailureKind.AuthFailed else FailureKind.Other
                message = e.message.orEmpty()
            }
        } catch (e: Exception) {
            failure = FailureKind.Other
            message = e.message ?: e.javaClass.simpleName
        } finally {
            if (client === c) client = null
            c.close()
            finishSession(failure, message)
        }
    }

    private fun finishSession(failure: FailureKind?, message: String) {
        // A newer session may already be starting on another worker.
        if (worker != Thread.currentThread()) return
        statsJob?.cancel()
        TunnelState.update { current ->
            val f = failure ?: current.failure.takeUnless { userStopped }
            TunnelStatus(
                phase = if (f != null) Phase.Failed else Phase.Disconnected,
                profileId = current.profileId,
                failure = f,
                message = when {
                    f == FailureKind.AuthFailed && profile?.twoFactor == true -> getString(R.string.error_auth_failed_2fa)
                    f == FailureKind.AuthFailed -> getString(R.string.error_auth_failed)
                    f == FailureKind.Revoked -> getString(R.string.error_revoked)
                    f == FailureKind.NeedsSignIn && profile?.twoFactor == true -> getString(R.string.error_need_creds_2fa)
                    f == FailureKind.NeedsSignIn -> getString(R.string.error_need_creds)
                    else -> message.ifBlank { current.message }
                },
            )
        }
        if (message.isNotBlank()) TunnelState.log("Session ended: $message")
        ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE)
        stopSelf()
    }

    private fun stopTunnel() {
        userStopped = true
        if (worker?.isAlive == true) {
            TunnelState.update { it.copy(phase = Phase.Disconnecting) }
        }
        stopCore()
    }

    private fun stopCore() {
        client?.stop()
    }

    // ---- OpenVpnClient.Callbacks ---------------------------------------

    override fun onEvent(name: String, info: String, error: Boolean, fatal: Boolean) {
        TunnelState.log(if (info.isNotEmpty()) "EVENT $name: $info" else "EVENT $name")
        when (name) {
            "CONNECTED" -> {
                val ci = client?.connectionInfo()
                TunnelState.update {
                    it.copy(
                        phase = Phase.Connected,
                        step = name,
                        failure = null,
                        message = "",
                        connectedAt = it.connectedAt ?: System.currentTimeMillis(),
                        vpnAddress = ci?.vpnIp4?.ifEmpty { ci.vpnIp6 } ?: "",
                        server = ci?.let { c -> "${c.serverHost}:${c.serverPort}/${c.serverProto.lowercase()}" } ?: it.server,
                    )
                }
                updateNotification(connected = true)
                startStats()
            }
            "RECONNECTING" -> {
                TunnelState.update { it.copy(phase = Phase.Reconnecting, step = name) }
                updateNotification(connected = false)
            }
            "AUTH_FAILED" -> TunnelState.update {
                it.copy(failure = FailureKind.AuthFailed, message = info)
            }
            // Profiles/servers with auth-nocache wipe the password after the
            // first attempt, so any reconnect needs a new sign-in.
            "NEED_CREDS" -> TunnelState.update { it.copy(failure = FailureKind.NeedsSignIn) }
            "DISCONNECTED" -> Unit
            else -> {
                if (fatal) TunnelState.update { it.copy(failure = FailureKind.Other, message = info.ifEmpty { name }) }
                else TunnelState.update { it.copy(step = name) }
            }
        }
    }

    override fun onLog(text: String) = TunnelState.log(text)

    override fun protectSocket(fd: Int): Boolean = protect(fd)

    override fun tunNew(): Boolean {
        hasIpv6Address = false
        blockIpv6 = false
        builder = Builder()
            .setSession(profile?.name ?: getString(R.string.app_name))
            .setConfigureIntent(mainActivityIntent())
            .also { if (Build.VERSION.SDK_INT >= 29) it.setMetered(false) }
        return true
    }

    override fun tunAddAddress(address: String, prefixLength: Int, ipv6: Boolean): Boolean = tunCall {
        it.addAddress(address, prefixLength)
        if (ipv6) hasIpv6Address = true
    }

    override fun tunRerouteGateway(ipv4: Boolean, ipv6: Boolean): Boolean = tunCall {
        if (ipv4) it.addRoute("0.0.0.0", 0)
        if (ipv6) it.addRoute("::", 0)
    }

    override fun tunAddRoute(address: String, prefixLength: Int, ipv6: Boolean): Boolean = tunCall {
        it.addRoute(address, prefixLength)
    }

    override fun tunExcludeRoute(address: String, prefixLength: Int, ipv6: Boolean): Boolean = tunCall {
        if (Build.VERSION.SDK_INT >= 33) {
            it.excludeRoute(IpPrefix(InetAddress.getByName(address), prefixLength))
        } else {
            TunnelState.log("Route exclusion needs Android 13; ignoring $address/$prefixLength")
        }
    }

    override fun tunAddDnsServer(address: String): Boolean = tunCall { it.addDnsServer(address) }

    override fun tunAddSearchDomain(domain: String): Boolean = tunCall { it.addSearchDomain(domain) }

    override fun tunSetMtu(mtu: Int): Boolean = tunCall { it.setMtu(mtu) }

    override fun tunSetSessionName(name: String): Boolean = tunCall { it.setSession(name) }

    override fun tunSetAllowFamily(ipv6: Boolean, allow: Boolean): Boolean {
        if (ipv6 && !allow) blockIpv6 = true
        return true
    }

    override fun tunEstablish(): Int {
        val b = builder ?: return -1
        return try {
            if (blockIpv6 && !hasIpv6Address) {
                // Route all IPv6 into the tunnel (where the core drops it)
                // instead of letting it leak past the VPN.
                b.addAddress("fd00:746b::1", 128)
                b.addRoute("::", 0)
            }
            val pfd: ParcelFileDescriptor = b.establish() ?: return -1
            pfd.detachFd() // ownership passes to the core
        } catch (e: Exception) {
            TunnelState.log("Could not create tunnel: ${e.message}")
            -1
        }
    }

    override fun tunTeardown(disconnect: Boolean) {
        builder = null
    }

    private inline fun tunCall(block: (Builder) -> Unit): Boolean {
        val b = builder ?: return false
        return try {
            block(b)
            true
        } catch (e: Exception) {
            TunnelState.log("Tunnel setting rejected: ${e.message}")
            false
        }
    }

    // ---- Stats & notification ------------------------------------------

    private fun startStats() {
        statsJob?.cancel()
        statsJob = scope.launch {
            while (isActive) {
                val c = client ?: break
                val (bytesIn, bytesOut) = c.transportStats()
                TunnelState.update { it.copy(bytesIn = bytesIn, bytesOut = bytesOut) }
                delay(1_000)
            }
        }
    }

    private fun promoteToForeground(profileName: String) {
        val nm = getSystemService(NotificationManager::class.java)
        if (nm.getNotificationChannel(CHANNEL_ID) == null) {
            nm.createNotificationChannel(
                NotificationChannel(CHANNEL_ID, getString(R.string.notification_channel), NotificationManager.IMPORTANCE_LOW),
            )
        }
        ServiceCompat.startForeground(
            this,
            NOTIFICATION_ID,
            buildNotification(profileName, connected = false),
            if (Build.VERSION.SDK_INT >= 34) ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE else 0,
        )
    }

    private fun updateNotification(connected: Boolean) {
        val name = profile?.name ?: return
        getSystemService(NotificationManager::class.java).notify(NOTIFICATION_ID, buildNotification(name, connected))
    }

    private fun buildNotification(profileName: String, connected: Boolean): Notification {
        val disconnect = PendingIntent.getService(
            this, 1,
            Intent(this, TunnelService::class.java).setAction(ACTION_DISCONNECT),
            PendingIntent.FLAG_IMMUTABLE,
        )
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_stat_tunnel)
            .setContentTitle(getString(if (connected) R.string.notification_connected else R.string.notification_connecting))
            .setContentText(profileName)
            .setContentIntent(mainActivityIntent())
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setCategory(NotificationCompat.CATEGORY_SERVICE)
            .addAction(0, getString(R.string.action_disconnect), disconnect)
            .build()
    }

    private fun mainActivityIntent(): PendingIntent = PendingIntent.getActivity(
        this, 0,
        Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP),
        PendingIntent.FLAG_IMMUTABLE,
    )

    companion object {
        private const val ACTION_CONNECT = "app.tunnelkey.CONNECT"
        private const val ACTION_DISCONNECT = "app.tunnelkey.DISCONNECT"
        private const val EXTRA_PROFILE_ID = "profile"
        private const val EXTRA_PASSWORD = "password"
        private const val EXTRA_CODE = "code"
        private const val CHANNEL_ID = "tunnel"
        private const val NOTIFICATION_ID = 1

        /** Caller must have obtained VPN consent via [VpnService.prepare] first. */
        fun connect(context: Context, profileId: String, password: String?, code: String?) {
            val intent = Intent(context, TunnelService::class.java)
                .setAction(ACTION_CONNECT)
                .putExtra(EXTRA_PROFILE_ID, profileId)
                .putExtra(EXTRA_PASSWORD, password)
                .putExtra(EXTRA_CODE, code)
            ContextCompat.startForegroundService(context, intent)
        }

        fun disconnect(context: Context) {
            context.startService(Intent(context, TunnelService::class.java).setAction(ACTION_DISCONNECT))
        }
    }
}
