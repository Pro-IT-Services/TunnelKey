package app.tunnelkey.data

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/**
 * Stores saved passwords encrypted with an AES-256-GCM key that lives in the
 * Android Keystore (hardware-backed where available) and never leaves it.
 */
class SecretStore(context: Context) {

    private val prefs = context.getSharedPreferences("secrets", Context.MODE_PRIVATE)

    fun put(profileId: String, secret: String) {
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.ENCRYPT_MODE, key())
        val plain = secret.toByteArray(Charsets.UTF_8)
        val sealed = cipher.iv + cipher.doFinal(plain)
        plain.fill(0)
        prefs.edit().putString(profileId, Base64.encodeToString(sealed, Base64.NO_WRAP)).apply()
    }

    fun get(profileId: String): String? {
        val stored = prefs.getString(profileId, null) ?: return null
        return try {
            val sealed = Base64.decode(stored, Base64.NO_WRAP)
            val cipher = Cipher.getInstance(TRANSFORMATION)
            cipher.init(Cipher.DECRYPT_MODE, key(), GCMParameterSpec(128, sealed, 0, IV_BYTES))
            String(cipher.doFinal(sealed, IV_BYTES, sealed.size - IV_BYTES), Charsets.UTF_8)
        } catch (e: Exception) {
            // Key invalidated (e.g. device credential reset): forget the secret.
            remove(profileId)
            null
        }
    }

    fun has(profileId: String): Boolean = prefs.contains(profileId)

    fun remove(profileId: String) {
        prefs.edit().remove(profileId).apply()
    }

    private fun key(): SecretKey {
        val ks = KeyStore.getInstance(ANDROID_KEYSTORE).apply { load(null) }
        (ks.getEntry(KEY_ALIAS, null) as? KeyStore.SecretKeyEntry)?.let { return it.secretKey }
        val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, ANDROID_KEYSTORE)
        generator.init(
            KeyGenParameterSpec.Builder(KEY_ALIAS, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(256)
                .build(),
        )
        return generator.generateKey()
    }

    private companion object {
        const val ANDROID_KEYSTORE = "AndroidKeyStore"
        const val KEY_ALIAS = "tunnelkey.passwords"
        const val TRANSFORMATION = "AES/GCM/NoPadding"
        const val IV_BYTES = 12
    }
}
