package app.tunnelkey.security

import android.content.Context
import android.os.Build
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import java.security.KeyStore
import java.security.SecureRandom
import javax.crypto.AEADBadTagException
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.SecretKeyFactory
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.PBEKeySpec
import javax.crypto.spec.SecretKeySpec

enum class LockMethod { None, Biometric, Pin }

/** Secrets of the provisioned configuration. Only ever held decrypted in memory. */
@Serializable
data class VaultSecrets(
    val password: String? = null,
    val totpSecret: String? = null,
)

sealed interface PinResult {
    data class Unlocked(val secrets: VaultSecrets) : PinResult
    data class Wrong(val attemptsLeft: Int, val lockedUntil: Long) : PinResult
    data class LockedOut(val until: Long) : PinResult
    /** Too many wrong PINs: the configuration was erased. */
    data object Wiped : PinResult
}

/**
 * Encrypted storage for the provisioned password and TOTP secret.
 *
 *  - Biometric: AES-256-GCM key in the Android Keystore that can only be used
 *    after a strong biometric check (BiometricPrompt + CryptoObject), and is
 *    invalidated when fingerprints/faces change.
 *  - PIN: AES-256-GCM with a key derived from the PIN (PBKDF2-SHA256), then
 *    encrypted again with a Keystore key so the blob can't be brute-forced
 *    off the device. Wrong PINs cause growing delays; the 10th erases it.
 *  - None: Keystore key only (allowed when nothing secret is stored).
 */
class Vault(context: Context) {

    private val prefs = context.getSharedPreferences("vault", Context.MODE_PRIVATE)
    private val json = Json { ignoreUnknownKeys = true }
    private val random = SecureRandom()

    val method: LockMethod
        get() = prefs.getString(KEY_METHOD, null)?.let { runCatching { LockMethod.valueOf(it) }.getOrNull() } ?: LockMethod.None

    val exists: Boolean get() = prefs.contains(KEY_BLOB)

    val failedAttempts: Int get() = prefs.getInt(KEY_FAILURES, 0)
    val lockedUntil: Long get() = prefs.getLong(KEY_LOCKED_UNTIL, 0)

    // ---- Store ------------------------------------------------------------

    fun storeUnprotected(secrets: VaultSecrets) {
        val sealed = seal(keystoreKey(ALIAS_PLAIN, requireAuth = false), encode(secrets))
        save(LockMethod.None, sealed, salt = null)
    }

    fun storeWithPin(pin: String, secrets: VaultSecrets) {
        val salt = ByteArray(16).also(random::nextBytes)
        val inner = seal(pinKey(pin, salt), encode(secrets))
        val outer = seal(keystoreKey(ALIAS_PIN_LAYER, requireAuth = false), inner)
        save(LockMethod.Pin, outer, salt)
    }

    /** A cipher to hand to BiometricPrompt before [storeWithBiometric]. */
    fun biometricEncryptCipher(): Cipher {
        deleteKey(ALIAS_BIOMETRIC) // fresh key per enrolment
        return Cipher.getInstance(TRANSFORMATION).apply {
            init(Cipher.ENCRYPT_MODE, keystoreKey(ALIAS_BIOMETRIC, requireAuth = true))
        }
    }

    /** [cipher] must have been authenticated by BiometricPrompt. */
    fun storeWithBiometric(cipher: Cipher, secrets: VaultSecrets) {
        val plain = encode(secrets)
        val sealed = cipher.iv + cipher.doFinal(plain)
        plain.fill(0)
        save(LockMethod.Biometric, sealed, salt = null)
    }

    // ---- Unlock -----------------------------------------------------------

    fun unlockUnprotected(): VaultSecrets =
        decode(open(keystoreKey(ALIAS_PLAIN, requireAuth = false), blob()))

    /**
     * A cipher to hand to BiometricPrompt, or null when the key was
     * invalidated (biometrics changed); the configuration must then be rescanned.
     */
    fun biometricDecryptCipher(): Cipher? = try {
        val blob = blob()
        Cipher.getInstance(TRANSFORMATION).apply {
            init(Cipher.DECRYPT_MODE, keystoreKey(ALIAS_BIOMETRIC, requireAuth = true, create = false),
                GCMParameterSpec(128, blob, 0, IV_BYTES))
        }
    } catch (e: Exception) {
        null
    }

    fun unlockWithBiometric(cipher: Cipher): VaultSecrets {
        val blob = blob()
        return decode(cipher.doFinal(blob, IV_BYTES, blob.size - IV_BYTES))
    }

    fun unlockWithPin(pin: String, now: Long = System.currentTimeMillis()): PinResult {
        if (now < lockedUntil) return PinResult.LockedOut(lockedUntil)
        val salt = Base64.decode(prefs.getString(KEY_SALT, "") ?: "", Base64.NO_WRAP)
        return try {
            val inner = open(keystoreKey(ALIAS_PIN_LAYER, requireAuth = false), blob())
            val secrets = decode(open(pinKey(pin, salt), inner))
            prefs.edit().putInt(KEY_FAILURES, 0).putLong(KEY_LOCKED_UNTIL, 0).apply()
            PinResult.Unlocked(secrets)
        } catch (e: AEADBadTagException) {
            val failures = failedAttempts + 1
            if (failures >= MAX_ATTEMPTS) {
                wipe()
                return PinResult.Wiped
            }
            val until = now + delayAfter(failures)
            prefs.edit().putInt(KEY_FAILURES, failures).putLong(KEY_LOCKED_UNTIL, until).apply()
            PinResult.Wrong(MAX_ATTEMPTS - failures, until)
        }
    }

    fun wipe() {
        prefs.edit().clear().apply()
        listOf(ALIAS_PLAIN, ALIAS_PIN_LAYER, ALIAS_BIOMETRIC).forEach(::deleteKey)
    }

    // ---- Internals --------------------------------------------------------

    private fun save(method: LockMethod, sealed: ByteArray, salt: ByteArray?) {
        prefs.edit()
            .putString(KEY_METHOD, method.name)
            .putString(KEY_BLOB, Base64.encodeToString(sealed, Base64.NO_WRAP))
            .putString(KEY_SALT, salt?.let { Base64.encodeToString(it, Base64.NO_WRAP) })
            .putInt(KEY_FAILURES, 0)
            .putLong(KEY_LOCKED_UNTIL, 0)
            .apply()
        // Drop keys of other methods.
        listOf(ALIAS_PLAIN to LockMethod.None, ALIAS_PIN_LAYER to LockMethod.Pin, ALIAS_BIOMETRIC to LockMethod.Biometric)
            .filter { it.second != method }
            .forEach { deleteKey(it.first) }
    }

    private fun blob(): ByteArray =
        Base64.decode(prefs.getString(KEY_BLOB, null) ?: throw IllegalStateException("no vault"), Base64.NO_WRAP)

    private fun encode(s: VaultSecrets) = json.encodeToString(s).toByteArray(Charsets.UTF_8)

    private fun decode(b: ByteArray): VaultSecrets {
        try {
            return json.decodeFromString(b.toString(Charsets.UTF_8))
        } finally {
            b.fill(0)
        }
    }

    private fun seal(key: SecretKey, plain: ByteArray): ByteArray {
        val c = Cipher.getInstance(TRANSFORMATION)
        c.init(Cipher.ENCRYPT_MODE, key)
        return c.iv + c.doFinal(plain)
    }

    private fun open(key: SecretKey, sealed: ByteArray): ByteArray {
        val c = Cipher.getInstance(TRANSFORMATION)
        c.init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(128, sealed, 0, IV_BYTES))
        return c.doFinal(sealed, IV_BYTES, sealed.size - IV_BYTES)
    }

    private fun pinKey(pin: String, salt: ByteArray): SecretKey {
        val spec = PBEKeySpec(pin.toCharArray(), salt, PBKDF2_ITERATIONS, 256)
        try {
            val bytes = SecretKeyFactory.getInstance("PBKDF2WithHmacSHA256").generateSecret(spec).encoded
            return SecretKeySpec(bytes, "AES")
        } finally {
            spec.clearPassword()
        }
    }

    private fun keystoreKey(alias: String, requireAuth: Boolean, create: Boolean = true): SecretKey {
        val ks = KeyStore.getInstance(ANDROID_KEYSTORE).apply { load(null) }
        (ks.getEntry(alias, null) as? KeyStore.SecretKeyEntry)?.let { return it.secretKey }
        if (!create) throw IllegalStateException("key $alias missing")
        val builder = KeyGenParameterSpec.Builder(alias, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
            .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
            .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
            .setKeySize(256)
        if (requireAuth) {
            builder.setUserAuthenticationRequired(true)
                .setInvalidatedByBiometricEnrollment(true)
            if (Build.VERSION.SDK_INT >= 30) {
                builder.setUserAuthenticationParameters(0, KeyProperties.AUTH_BIOMETRIC_STRONG)
            } else {
                @Suppress("DEPRECATION")
                builder.setUserAuthenticationValidityDurationSeconds(-1)
            }
        }
        val gen = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, ANDROID_KEYSTORE)
        gen.init(builder.build())
        return gen.generateKey()
    }

    private fun deleteKey(alias: String) {
        runCatching { KeyStore.getInstance(ANDROID_KEYSTORE).apply { load(null) }.deleteEntry(alias) }
    }

    companion object {
        const val MAX_ATTEMPTS = 10
        private const val PBKDF2_ITERATIONS = 310_000
        private const val ANDROID_KEYSTORE = "AndroidKeyStore"
        private const val TRANSFORMATION = "AES/GCM/NoPadding"
        private const val IV_BYTES = 12
        private const val ALIAS_PLAIN = "tunnelkey.vault.plain"
        private const val ALIAS_PIN_LAYER = "tunnelkey.vault.pin"
        private const val ALIAS_BIOMETRIC = "tunnelkey.vault.biometric"
        private const val KEY_METHOD = "method"
        private const val KEY_BLOB = "blob"
        private const val KEY_SALT = "salt"
        private const val KEY_FAILURES = "failures"
        private const val KEY_LOCKED_UNTIL = "locked_until"

        /** No delay for the first 4 mistakes, then 30 s, 1 min, 5 min, 15 min, 1 h. */
        fun delayAfter(failures: Int): Long = when {
            failures < 5 -> 0L
            failures == 5 -> 30_000L
            failures == 6 -> 60_000L
            failures == 7 -> 5 * 60_000L
            failures == 8 -> 15 * 60_000L
            else -> 60 * 60_000L
        }
    }
}
