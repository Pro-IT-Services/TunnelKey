package app.tunnelkey.security

import javax.crypto.Mac
import javax.crypto.spec.SecretKeySpec

/** RFC 6238 time-based one-time passwords. */
data class Totp(
    val secret: ByteArray,
    val digits: Int = 6,
    val period: Int = 30,
    val algorithm: String = "SHA1",
) {
    fun code(epochSeconds: Long = System.currentTimeMillis() / 1000): String {
        val mac = Mac.getInstance(
            when (algorithm.uppercase()) {
                "SHA256" -> "HmacSHA256"
                "SHA512" -> "HmacSHA512"
                else -> "HmacSHA1"
            },
        )
        mac.init(SecretKeySpec(secret, "RAW"))
        val counter = epochSeconds / period
        val msg = ByteArray(8) { i -> (counter ushr (56 - 8 * i)).toByte() }
        val h = mac.doFinal(msg)
        val off = h.last().toInt() and 0x0F
        val bin = ((h[off].toInt() and 0x7F) shl 24) or
            ((h[off + 1].toInt() and 0xFF) shl 16) or
            ((h[off + 2].toInt() and 0xFF) shl 8) or
            (h[off + 3].toInt() and 0xFF)
        var mod = 1
        repeat(digits) { mod *= 10 }
        return (bin % mod).toString().padStart(digits, '0')
    }

    /** Seconds until the current code rolls over. */
    fun secondsLeft(epochSeconds: Long = System.currentTimeMillis() / 1000): Int =
        (period - (epochSeconds % period)).toInt()

    override fun equals(other: Any?) = other is Totp && secret.contentEquals(other.secret) &&
        digits == other.digits && period == other.period && algorithm == other.algorithm

    override fun hashCode() = secret.contentHashCode()

    companion object {
        fun base32Decode(input: String): ByteArray {
            val s = input.uppercase().filter { it != ' ' && it != '-' && it != '=' }
            val out = java.io.ByteArrayOutputStream(s.length * 5 / 8)
            var buffer = 0
            var bits = 0
            for (c in s) {
                val v = when (c) {
                    in 'A'..'Z' -> c - 'A'
                    in '2'..'7' -> c - '2' + 26
                    else -> throw IllegalArgumentException("Invalid base32 character")
                }
                buffer = (buffer shl 5) or v
                bits += 5
                if (bits >= 8) {
                    out.write((buffer shr (bits - 8)) and 0xFF)
                    bits -= 8
                }
            }
            return out.toByteArray()
        }
    }
}
