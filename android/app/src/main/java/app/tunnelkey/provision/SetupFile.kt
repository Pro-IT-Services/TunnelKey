package app.tunnelkey.provision

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import java.text.Normalizer
import java.util.Base64
import javax.crypto.AEADBadTagException
import javax.crypto.Cipher
import javax.crypto.SecretKeyFactory
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.PBEKeySpec
import javax.crypto.spec.SecretKeySpec

// Reader for password-encrypted setup files (.tunnelkey) made by the setup
// pages. Format: docs/provisioning-format.md, "Setup file format (v1)".

class SetupFileException(val kind: Kind) : Exception(kind.name) {
    enum class Kind { NotSetupFile, NewerVersion, WrongPassword, Damaged }
}

@Serializable
private data class SetupFileKdf(
    @SerialName("alg") val alg: String = "",
    @SerialName("iter") val iter: Int = 0,
    @SerialName("salt") val salt: String = "",
)

@Serializable
private data class SetupFileEnc(
    @SerialName("alg") val alg: String = "",
    @SerialName("iv") val iv: String = "",
)

@Serializable
private data class SetupFileEnvelope(
    @SerialName("tunnelkey") val magic: String = "",
    @SerialName("v") val version: Int = 0,
    @SerialName("kdf") val kdf: SetupFileKdf = SetupFileKdf(),
    @SerialName("enc") val enc: SetupFileEnc = SetupFileEnc(),
    @SerialName("data") val data: String = "",
)

object SetupFiles {
    const val MIME_TYPE = "application/vnd.tunnelkey.setup+json"
    const val MAX_BYTES = 1024 * 1024
    private const val MIN_ITERATIONS = 100_000
    private const val MAX_ITERATIONS = 10_000_000
    private val json = Json { ignoreUnknownKeys = true }

    /** Cheap check used to route an opened file before asking for a password. */
    fun looksLike(text: String): Boolean {
        val head = text.take(4096).removePrefix(Char(0xFEFF).toString()).trimStart()
        return head.startsWith("{") && head.contains("\"tunnelkey\"")
    }

    /** Decrypts a setup file. Slow on purpose (PBKDF2); call off the main thread. */
    fun decrypt(text: String, password: String): SetupPayload {
        val env = try {
            json.decodeFromString<SetupFileEnvelope>(text.removePrefix(Char(0xFEFF).toString()))
        } catch (e: Exception) {
            throw SetupFileException(SetupFileException.Kind.NotSetupFile)
        }
        if (env.magic != "setup-file") throw SetupFileException(SetupFileException.Kind.NotSetupFile)
        if (env.version != 1) {
            throw SetupFileException(if (env.version > 1) SetupFileException.Kind.NewerVersion else SetupFileException.Kind.NotSetupFile)
        }
        if (env.kdf.alg != "PBKDF2-SHA256" || env.enc.alg != "A256GCM") throw SetupFileException(SetupFileException.Kind.NewerVersion)
        if (env.kdf.iter !in MIN_ITERATIONS..MAX_ITERATIONS) throw SetupFileException(SetupFileException.Kind.Damaged)

        val salt = b64(env.kdf.salt)
        val iv = b64(env.enc.iv)
        val data = b64(env.data)
        if (salt.size < 16 || iv.size != 12 || data.size < 16) throw SetupFileException(SetupFileException.Kind.Damaged)

        val compressed = try {
            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(Cipher.DECRYPT_MODE, key(password, salt, env.kdf.iter), GCMParameterSpec(128, iv))
            cipher.updateAAD("tunnelkey-setup-file:1:${env.kdf.iter}:${env.kdf.salt}:${env.enc.iv}".toByteArray(Charsets.US_ASCII))
            cipher.doFinal(data)
        } catch (e: AEADBadTagException) {
            throw SetupFileException(SetupFileException.Kind.WrongPassword)
        }
        return try {
            SetupCodes.fromZlib(compressed)
        } catch (e: SetupCodeException) {
            throw SetupFileException(
                if (e.kind == SetupCodeException.Kind.NewerVersion) SetupFileException.Kind.NewerVersion else SetupFileException.Kind.Damaged,
            )
        }
    }

    private fun key(password: String, salt: ByteArray, iterations: Int): SecretKeySpec {
        // PBKDF2WithHmacSHA256 hashes the password as UTF-8, like the browser.
        val spec = PBEKeySpec(Normalizer.normalize(password, Normalizer.Form.NFC).toCharArray(), salt, iterations, 256)
        try {
            return SecretKeySpec(SecretKeyFactory.getInstance("PBKDF2WithHmacSHA256").generateSecret(spec).encoded, "AES")
        } finally {
            spec.clearPassword()
        }
    }

    private fun b64(s: String): ByteArray = try {
        Base64.getUrlDecoder().decode(s)
    } catch (e: IllegalArgumentException) {
        throw SetupFileException(SetupFileException.Kind.Damaged)
    }
}
