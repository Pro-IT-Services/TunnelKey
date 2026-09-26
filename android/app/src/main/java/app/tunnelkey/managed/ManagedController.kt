package app.tunnelkey.managed

import android.content.Context
import app.tunnelkey.data.CodePosition
import app.tunnelkey.data.OvpnInspector
import app.tunnelkey.data.Profile
import app.tunnelkey.data.ProfileRepository
import app.tunnelkey.provision.SetupPayload
import app.tunnelkey.security.LockMethod
import app.tunnelkey.security.PinResult
import app.tunnelkey.security.Totp
import app.tunnelkey.security.Vault
import app.tunnelkey.security.VaultSecrets
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.withContext
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import java.io.File
import java.util.UUID
import javax.crypto.Cipher

@Serializable
data class ManagedLink(val title: String, val kind: String, val uri: String)

@Serializable
data class TotpParams(val digits: Int, val period: Int, val algorithm: String)

/** Non-secret part of a provisioned configuration. */
@Serializable
data class ManagedConfig(
    val name: String,
    val profileId: String,
    val username: String,
    val hasPassword: Boolean,
    val totp: TotpParams? = null,
    val manualCode: Boolean = false,
    val links: List<ManagedLink> = emptyList(),
) {
    /** Password or TOTP secret stored: the app must be locked. */
    val lockRequired: Boolean get() = hasPassword || totp != null
}

/**
 * Single-configuration mode: owns the provisioned config, its encrypted
 * secrets and the in-memory unlocked session.
 */
class ManagedController(context: Context, private val profiles: ProfileRepository) {

    private val file = File(context.filesDir, "managed.json")
    private val json = Json { ignoreUnknownKeys = true; encodeDefaults = true }
    val vault = Vault(context)

    private val _config = MutableStateFlow(load())
    val config: StateFlow<ManagedConfig?> = _config.asStateFlow()

    /** Decrypted secrets while the app is unlocked; null when locked. */
    private val _session = MutableStateFlow<VaultSecrets?>(null)
    val unlocked: StateFlow<VaultSecrets?> = _session.asStateFlow()

    val isActive: Boolean get() = _config.value != null
    val lockMethod: LockMethod get() = vault.method

    val needsUnlock: Boolean
        get() = _config.value != null && vault.method != LockMethod.None && _session.value == null

    // ---- Provisioning -------------------------------------------------

    /** Saves a scanned setup code. Call one of the lock functions right after. */
    private suspend fun install(payload: SetupPayload): ManagedConfig = withContext(Dispatchers.IO) {
        _config.value?.let { profiles.delete(it.profileId) }
        val summary = OvpnInspector.inspect(payload.ovpn)
        val profile = profiles.save(
            Profile(
                id = UUID.randomUUID().toString(),
                name = payload.name,
                remote = summary.remote,
                username = payload.username,
                needsCredentials = summary.needsCredentials,
                twoFactor = payload.totp != null || payload.manualCode,
                codePosition = if (payload.codePosition == "b") CodePosition.BEFORE_PASSWORD else CodePosition.AFTER_PASSWORD,
                codeLength = payload.totp?.digits ?: 6,
                managed = true,
            ),
            content = payload.ovpn,
            password = null,
        )
        val cfg = ManagedConfig(
            name = payload.name,
            profileId = profile.id,
            username = payload.username,
            hasPassword = !payload.password.isNullOrEmpty(),
            totp = payload.totp?.let { TotpParams(it.digits, it.period, it.algorithm.uppercase()) },
            manualCode = payload.totp == null && payload.manualCode,
            links = payload.links.map { ManagedLink(it.title, it.kind, it.uri) },
        )
        file.writeText(json.encodeToString(cfg))
        cfg
    }

    private fun secretsOf(p: SetupPayload) = VaultSecrets(password = p.password?.ifEmpty { null }, totpSecret = p.totp?.secret)

    suspend fun installWithPin(payload: SetupPayload, pin: String) {
        val cfg = install(payload)
        val secrets = secretsOf(payload)
        withContext(Dispatchers.Default) { vault.storeWithPin(pin, secrets) }
        activate(cfg, secrets)
    }

    suspend fun installWithBiometric(payload: SetupPayload, cipher: Cipher) {
        val cfg = install(payload)
        val secrets = secretsOf(payload)
        vault.storeWithBiometric(cipher, secrets)
        activate(cfg, secrets)
    }

    suspend fun installUnprotected(payload: SetupPayload) {
        val cfg = install(payload)
        val secrets = secretsOf(payload)
        vault.storeUnprotected(secrets)
        activate(cfg, secrets)
    }

    private fun activate(cfg: ManagedConfig, secrets: VaultSecrets) {
        _session.value = secrets
        _config.value = cfg
    }

    // ---- Changing the lock (app must be unlocked) -----------------------

    suspend fun changeToPin(pin: String) {
        val s = _session.value ?: return
        withContext(Dispatchers.Default) { vault.storeWithPin(pin, s) }
    }

    fun changeToBiometric(cipher: Cipher) {
        val s = _session.value ?: return
        vault.storeWithBiometric(cipher, s)
    }

    // ---- Lock / unlock --------------------------------------------------

    fun unlockWithBiometric(cipher: Cipher) {
        _session.value = vault.unlockWithBiometric(cipher)
    }

    suspend fun unlockWithPin(pin: String): PinResult {
        val result = withContext(Dispatchers.Default) { vault.unlockWithPin(pin) }
        when (result) {
            is PinResult.Unlocked -> _session.value = result.secrets
            PinResult.Wiped -> removeLocalState()
            else -> Unit
        }
        return result
    }

    /** Unprotected configs unlock silently. */
    fun unlockIfUnprotected() {
        if (_config.value != null && vault.method == LockMethod.None && _session.value == null && vault.exists) {
            _session.value = runCatching { vault.unlockUnprotected() }.getOrNull()
        }
    }

    fun lock() {
        if (vault.method != LockMethod.None) _session.value = null
    }

    // ---- Connecting -----------------------------------------------------

    fun password(): String? = _session.value?.password

    fun totp(): Totp? {
        val params = _config.value?.totp ?: return null
        val secret = _session.value?.totpSecret ?: return null
        return Totp(Totp.base32Decode(secret), params.digits, params.period, params.algorithm)
    }

    // ---- Removal --------------------------------------------------------

    suspend fun remove() = withContext(Dispatchers.IO) { removeLocalState() }

    private suspend fun removeLocalState() {
        _config.value?.let { profiles.delete(it.profileId) }
        file.delete()
        vault.wipe()
        _session.value = null
        _config.value = null
    }

    private fun load(): ManagedConfig? = try {
        if (file.exists()) json.decodeFromString<ManagedConfig>(file.readText()) else null
    } catch (e: Exception) {
        null
    }
}
