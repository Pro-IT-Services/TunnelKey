package app.tunnelkey.data

import android.content.Context
import android.net.Uri
import android.provider.OpenableColumns
import app.tunnelkey.R
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import java.io.File
import java.util.UUID

/** A picked .ovpn file that has not been saved yet. */
data class ImportDraft(
    val suggestedName: String,
    val content: String,
    val summary: OvpnSummary,
)

/** [messageRes] is a user-facing string resource. */
class ImportException(@androidx.annotation.StringRes val messageRes: Int) : Exception()

/**
 * Profiles live in app-private storage: one `<id>.ovpn` per profile plus a
 * small JSON index with the user's settings for each.
 */
class ProfileRepository(private val context: Context, private val secrets: SecretStore) {

    private val dir = File(context.filesDir, "profiles").apply { mkdirs() }
    private val indexFile = File(dir, "index.json")
    private val json = Json { ignoreUnknownKeys = true; encodeDefaults = true }

    private val _profiles = MutableStateFlow(loadIndex())
    val profiles: StateFlow<List<Profile>> = _profiles.asStateFlow()

    fun get(id: String): Profile? = _profiles.value.firstOrNull { it.id == id }

    fun readConfig(id: String): String = File(dir, "$id.ovpn").readText()

    suspend fun readDraft(uri: Uri): ImportDraft = withContext(Dispatchers.IO) {
        val resolver = context.contentResolver
        val name = resolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)?.use { c ->
            if (c.moveToFirst()) c.getString(0) else null
        } ?: uri.lastPathSegment ?: "Profile"

        val bytes = resolver.openInputStream(uri)?.use { input ->
            val buffer = input.readNBytesCompat(OvpnInspector.MAX_PROFILE_BYTES + 1)
            if (buffer.size > OvpnInspector.MAX_PROFILE_BYTES) throw ImportException(R.string.import_too_large)
            buffer
        } ?: throw ImportException(R.string.import_unreadable)

        val content = String(bytes, Charsets.UTF_8).removePrefix(Char(0xFEFF).toString()) // byte-order mark
        val summary = OvpnInspector.inspect(content)
        if (!summary.isValid) throw ImportException(R.string.import_not_profile)
        ImportDraft(name.substringBeforeLast('.').ifBlank { "Profile" }, content, summary)
    }

    /** Saves a new profile (when [content] is given) or updates an existing one. */
    suspend fun save(profile: Profile, content: String?, password: String?) = withContext(Dispatchers.IO) {
        val id = profile.id.ifEmpty { UUID.randomUUID().toString() }
        val saved = profile.copy(id = id)
        if (content != null) File(dir, "$id.ovpn").writeText(content)

        when {
            !saved.rememberPassword -> secrets.remove(id)
            !password.isNullOrEmpty() -> secrets.put(id, password)
        }

        val list = _profiles.value.filterNot { it.id == id } + saved
        writeIndex(list.sortedBy { it.importedAt })
        saved
    }

    suspend fun delete(id: String) = withContext(Dispatchers.IO) {
        File(dir, "$id.ovpn").delete()
        secrets.remove(id)
        writeIndex(_profiles.value.filterNot { it.id == id })
    }

    fun savedPassword(id: String): String? = secrets.get(id)
    fun hasSavedPassword(id: String): Boolean = secrets.has(id)

    private fun loadIndex(): List<Profile> = try {
        if (indexFile.exists()) json.decodeFromString<List<Profile>>(indexFile.readText()) else emptyList()
    } catch (e: Exception) {
        emptyList()
    }

    private fun writeIndex(list: List<Profile>) {
        val tmp = File(dir, "index.json.tmp")
        tmp.writeText(json.encodeToString(list))
        tmp.renameTo(indexFile)
        _profiles.value = list
    }

    private fun java.io.InputStream.readNBytesCompat(limit: Int): ByteArray {
        val out = java.io.ByteArrayOutputStream()
        val buf = ByteArray(8192)
        while (out.size() < limit) {
            val n = read(buf, 0, minOf(buf.size, limit - out.size()))
            if (n < 0) break
            out.write(buf, 0, n)
        }
        return out.toByteArray()
    }
}
