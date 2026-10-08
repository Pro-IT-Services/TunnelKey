package app.tunnelkey.provision

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import java.io.ByteArrayOutputStream
import java.util.zip.Inflater

// Reader for setup codes produced by the Tunnelkey server.
// Format: docs/provisioning-format.md

@Serializable
data class SetupTotp(
    @SerialName("s") val secret: String,
    @SerialName("d") val digits: Int = 6,
    @SerialName("p") val period: Int = 30,
    @SerialName("a") val algorithm: String = "SHA1",
)

@Serializable
data class SetupLink(
    @SerialName("t") val title: String,
    @SerialName("k") val kind: String,
    @SerialName("u") val uri: String,
)

@Serializable
data class SetupPayload(
    @SerialName("v") val version: Int,
    @SerialName("n") val name: String,
    @SerialName("o") val ovpn: String,
    @SerialName("u") val username: String = "",
    @SerialName("p") val password: String? = null,
    @SerialName("t") val totp: SetupTotp? = null,
    @SerialName("f") val manualCode: Boolean = false,
    @SerialName("c") val codePosition: String = "a",
    @SerialName("l") val links: List<SetupLink> = emptyList(),
)

class SetupCodeException(val kind: Kind) : Exception(kind.name) {
    enum class Kind { Damaged, TooLarge, NewerVersion, Incomplete }
}

/** One scanned QR code of a set. */
data class SetupPart(val index: Int, val total: Int, val id: String, val data: String) {
    companion object {
        const val PREFIX = "TK1:"

        /** Returns null for QR codes that aren't Tunnelkey setup codes. */
        fun parse(text: String): SetupPart? {
            if (!text.startsWith(PREFIX)) return null
            val f = text.split(':', limit = 5)
            if (f.size != 5) return null
            val (i, n) = f[1].split('/').mapNotNull { it.toIntOrNull() }.takeIf { it.size == 2 } ?: return null
            val len = f[3].toIntOrNull() ?: return null
            if (i < 1 || i > n || n > 64 || f[2].isEmpty()) return null
            // Some scanners trim trailing spaces, which are valid Base45.
            return SetupPart(i, n, f[2], f[4].padEnd(len, ' '))
        }
    }
}

/** Collects the parts of one code set, in any order. */
class SetupCodeAssembler {
    private val parts = sortedMapOf<Int, String>()
    var id: String? = null
        private set
    var total: Int = 0
        private set

    val received: Set<Int> get() = parts.keys
    val isComplete: Boolean get() = total > 0 && parts.size == total

    /** Returns true when the part was new. A part from a different set restarts collection. */
    fun add(part: SetupPart): Boolean {
        if (id != part.id || total != part.total) {
            parts.clear()
            id = part.id
            total = part.total
        }
        return parts.put(part.index, part.data) == null
    }

    fun payload(): SetupPayload {
        if (!isComplete) throw SetupCodeException(SetupCodeException.Kind.Incomplete)
        return SetupCodes.decode(parts.values.joinToString(""))
    }
}

object SetupCodes {
    private const val ALPHABET = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:"
    private const val MAX_INFLATED = 512 * 1024
    private val json = Json { ignoreUnknownKeys = true }

    fun decode(base45: String): SetupPayload = fromZlib(base45Decode(base45))

    /** zlib-compressed JSON payload, shared with encrypted setup files. */
    fun fromZlib(compressed: ByteArray): SetupPayload {
        val raw = inflate(compressed)
        val payload = try {
            json.decodeFromString<SetupPayload>(raw.toString(Charsets.UTF_8))
        } catch (e: Exception) {
            throw SetupCodeException(SetupCodeException.Kind.Damaged)
        }
        if (payload.version != 1) throw SetupCodeException(SetupCodeException.Kind.NewerVersion)
        if (payload.name.isBlank() || payload.ovpn.isBlank()) throw SetupCodeException(SetupCodeException.Kind.Incomplete)
        return payload
    }

    fun base45Decode(s: String): ByteArray {
        if (s.length % 3 == 1) throw SetupCodeException(SetupCodeException.Kind.Damaged)
        val out = ByteArrayOutputStream(s.length * 2 / 3)
        var i = 0
        while (i < s.length) {
            val c0 = value(s[i])
            val c1 = value(s[i + 1])
            if (i + 2 < s.length) {
                val v = c0 + c1 * 45 + value(s[i + 2]) * 2025
                if (v > 0xFFFF) throw SetupCodeException(SetupCodeException.Kind.Damaged)
                out.write(v shr 8)
                out.write(v and 0xFF)
            } else {
                val v = c0 + c1 * 45
                if (v > 0xFF) throw SetupCodeException(SetupCodeException.Kind.Damaged)
                out.write(v)
            }
            i += 3
        }
        return out.toByteArray()
    }

    private fun value(c: Char): Int {
        val v = ALPHABET.indexOf(c)
        if (v < 0) throw SetupCodeException(SetupCodeException.Kind.Damaged)
        return v
    }

    /** zlib (RFC 1950) inflate; the Adler-32 trailer is verified by Inflater. */
    private fun inflate(data: ByteArray): ByteArray {
        val inflater = Inflater()
        try {
            inflater.setInput(data)
            val out = ByteArrayOutputStream(data.size * 3)
            val buf = ByteArray(8192)
            while (!inflater.finished()) {
                val n = inflater.inflate(buf)
                if (n == 0 && (inflater.needsInput() || inflater.needsDictionary())) {
                    throw SetupCodeException(SetupCodeException.Kind.Damaged)
                }
                out.write(buf, 0, n)
                if (out.size() > MAX_INFLATED) throw SetupCodeException(SetupCodeException.Kind.TooLarge)
            }
            return out.toByteArray()
        } catch (e: java.util.zip.DataFormatException) {
            throw SetupCodeException(SetupCodeException.Kind.Damaged)
        } finally {
            inflater.end()
        }
    }
}
