package app.tunnelkey.data

import kotlinx.serialization.Serializable

/** Where the one-time code goes relative to the password. */
@Serializable
enum class CodePosition { AFTER_PASSWORD, BEFORE_PASSWORD }

@Serializable
data class Profile(
    val id: String,
    val name: String,
    /** "host:port/proto" of the first remote, for display only. */
    val remote: String,
    val username: String = "",
    /** Profile contains `auth-user-pass`. */
    val needsCredentials: Boolean = true,
    /** Server expects an authenticator (TOTP) code together with the password. */
    val twoFactor: Boolean = false,
    val codePosition: CodePosition = CodePosition.AFTER_PASSWORD,
    val codeLength: Int = 6,
    /** Password is stored encrypted on the device. The code never is. */
    val rememberPassword: Boolean = false,
    val importedAt: Long = System.currentTimeMillis(),
    /** Installed from a setup code; hidden from the profile list. */
    val managed: Boolean = false,
)

object Credentials {
    /**
     * Builds the password the server's 2FA plugin expects: the static password
     * and the current authenticator code joined together, e.g. `hunter2` +
     * `123456` → `hunter2123456`.
     */
    fun combine(password: String, code: String, position: CodePosition): String =
        when (position) {
            CodePosition.AFTER_PASSWORD -> password + code
            CodePosition.BEFORE_PASSWORD -> code + password
        }

    fun isValidCode(code: String, length: Int): Boolean =
        code.length == length && code.all { it in '0'..'9' }
}
