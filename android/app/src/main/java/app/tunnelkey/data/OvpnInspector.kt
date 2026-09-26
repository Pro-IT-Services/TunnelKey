package app.tunnelkey.data

/**
 * Lightweight, pure-Kotlin look at an .ovpn file, used to pre-fill the import
 * form. The OpenVPN core does the authoritative parsing at connect time.
 */
data class OvpnSummary(
    val remote: String,
    val needsCredentials: Boolean,
    /** Text of a `static-challenge` directive, if present. */
    val staticChallenge: String?,
    /** Directives that point at files outside the profile (not available on a phone). */
    val externalFiles: List<String>,
    val isValid: Boolean,
)

object OvpnInspector {

    private val fileDirectives = setOf(
        "ca", "cert", "key", "tls-auth", "tls-crypt", "tls-crypt-v2", "pkcs12", "extra-certs", "crl-verify",
    )

    // Hard upper bound that matches the OpenVPN 3 core's own profile limit.
    const val MAX_PROFILE_BYTES = 256 * 1024

    fun inspect(content: String): OvpnSummary {
        var remote: String? = null
        var needsCredentials = false
        var staticChallenge: String? = null
        var sawClient = false
        val external = mutableListOf<String>()
        var inlineBlock: String? = null

        for (raw in content.lineSequence()) {
            val line = raw.trim()
            if (line.isEmpty() || line.startsWith("#") || line.startsWith(";")) continue

            if (inlineBlock != null) {
                if (line.equals("</$inlineBlock>", ignoreCase = true)) inlineBlock = null
                continue
            }
            if (line.startsWith("<") && line.endsWith(">")) {
                val tag = line.trim('<', '>', '/').lowercase()
                // <connection> blocks hold ordinary directives (e.g. remote); read through them.
                if (tag != "connection" && !line.startsWith("</")) inlineBlock = tag
                if (tag == "auth-user-pass") needsCredentials = true
                continue
            }

            val parts = tokenize(line)
            if (parts.isEmpty()) continue
            val directive = parts[0].lowercase()
            when {
                directive == "client" || directive == "tls-client" -> sawClient = true
                directive == "remote" && remote == null && parts.size >= 2 -> {
                    val host = parts[1]
                    val port = parts.getOrNull(2) ?: "1194"
                    val proto = parts.getOrNull(3)
                    remote = if (proto != null) "$host:$port/$proto" else "$host:$port"
                }
                directive == "auth-user-pass" -> needsCredentials = true
                directive == "static-challenge" -> staticChallenge = parts.getOrNull(1) ?: ""
                directive in fileDirectives && parts.size >= 2 -> external += parts[1]
            }
        }

        return OvpnSummary(
            remote = remote ?: "",
            needsCredentials = needsCredentials,
            staticChallenge = staticChallenge,
            externalFiles = external,
            isValid = remote != null && (sawClient || content.contains("<ca>", ignoreCase = true)),
        )
    }

    /** Splits a directive line, honouring double quotes. */
    internal fun tokenize(line: String): List<String> {
        val out = mutableListOf<String>()
        val sb = StringBuilder()
        var quoted = false
        var escaped = false
        for (c in line) {
            when {
                escaped -> { sb.append(c); escaped = false }
                c == '\\' && quoted -> escaped = true
                c == '"' -> quoted = !quoted
                c.isWhitespace() && !quoted -> if (sb.isNotEmpty()) { out += sb.toString(); sb.clear() }
                else -> sb.append(c)
            }
        }
        if (sb.isNotEmpty()) out += sb.toString()
        return out
    }
}
