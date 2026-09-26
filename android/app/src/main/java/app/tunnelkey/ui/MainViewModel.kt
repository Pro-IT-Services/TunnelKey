package app.tunnelkey.ui

import android.app.Application
import android.content.Context
import android.net.Uri
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import app.tunnelkey.TunnelkeyApp
import app.tunnelkey.data.CodePosition
import app.tunnelkey.data.ImportDraft
import app.tunnelkey.data.ImportException
import app.tunnelkey.data.Profile
import app.tunnelkey.vpn.TunnelService
import app.tunnelkey.vpn.TunnelState
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.delay
import app.tunnelkey.R
import app.tunnelkey.managed.ManagedLink
import app.tunnelkey.provision.SetupPayload
import app.tunnelkey.vpn.FailureKind
import app.tunnelkey.vpn.Phase
import kotlinx.coroutines.Job
import app.tunnelkey.vpn.TunnelStatus
import javax.crypto.Cipher
import kotlinx.coroutines.launch

/** What the sign-in sheet has to ask for before connecting. */
data class SignInRequest(
    val profile: Profile,
    val needsPassword: Boolean,
    val needsCode: Boolean,
    /** Profile uses OpenVPN's static-challenge instead of password+code. */
    val usesStaticChallenge: Boolean,
)

/** Form state for the profile editor (new import or existing profile). */
data class EditorForm(
    val id: String = "",
    val name: String = "",
    val remote: String = "",
    val username: String = "",
    val needsCredentials: Boolean = true,
    val twoFactor: Boolean = false,
    val codePosition: CodePosition = CodePosition.AFTER_PASSWORD,
    val codeLength: Int = 6,
    val rememberPassword: Boolean = false,
    val password: String = "",
    val hasSavedPassword: Boolean = false,
    val importedAt: Long = System.currentTimeMillis(),
    /** Set for new imports only. */
    val content: String? = null,
    val externalFiles: List<String> = emptyList(),
    val staticChallenge: String? = null,
) {
    val isNew: Boolean get() = id.isEmpty()
    val canSave: Boolean get() = name.isNotBlank() && (!needsCredentials || username.isNotBlank())
}

sealed interface UiEvent {
    data object OpenEditor : UiEvent
    data class Error(val title: String, val message: String) : UiEvent
    data class OpenLink(val link: ManagedLink) : UiEvent
}

class MainViewModel(app: Application) : AndroidViewModel(app) {

    private val repo = (app as TunnelkeyApp).profiles
    val managed = (app as TunnelkeyApp).managed
    private val prefs = app.getSharedPreferences("ui", Context.MODE_PRIVATE)

    /** Normal-mode profiles; the provisioned one is hidden. */
    val profiles: StateFlow<List<Profile>> = repo.profiles
        .map { list -> list.filterNot { it.managed } }
        .stateIn(viewModelScope, SharingStarted.Eagerly, repo.profiles.value.filterNot { it.managed })
    val managedConfig = managed.config
    val unlocked = managed.unlocked

    /** A scanned setup code waiting for the user to choose a lock. */
    var pendingSetup: SetupPayload? = null

    /** Short status line in single-config mode (e.g. waiting for a fresh code). */
    private val _hint = MutableStateFlow<String?>(null)
    val hint: StateFlow<String?> = _hint.asStateFlow()
    private var pendingLink: ManagedLink? = null
    val status = TunnelState.status
    val log = TunnelState.log

    private val _selectedId = MutableStateFlow(prefs.getString(KEY_SELECTED, null))
    val selectedId: StateFlow<String?> = _selectedId.asStateFlow()

    private val _form = MutableStateFlow(EditorForm())
    val form: StateFlow<EditorForm> = _form.asStateFlow()

    private val _signIn = MutableStateFlow<SignInRequest?>(null)
    val signIn: StateFlow<SignInRequest?> = _signIn.asStateFlow()

    private val _events = Channel<UiEvent>(Channel.BUFFERED)
    val events = _events.receiveAsFlow()

    init {
        viewModelScope.launch {
            var previous = status.value.phase
            status.collect { st ->
                val enteredFailed = st.phase == Phase.Failed && previous != Phase.Failed
                previous = st.phase
                when {
                    st.phase == Phase.Connected -> {
                        autoRetriesLeft = 0
                        val link = pendingLink ?: return@collect
                        pendingLink = null
                        _hint.value = null
                        _events.send(UiEvent.OpenLink(link))
                    }
                    enteredFailed && shouldAutoRetry(st) -> scheduleRetry()
                    enteredFailed || st.phase == Phase.Disconnected && retryJob == null -> {
                        pendingLink = null
                        _hint.value = null
                    }
                }
            }
        }
    }

    fun selectedProfile(): Profile? {
        val list = profiles.value
        return list.firstOrNull { it.id == _selectedId.value } ?: list.firstOrNull()
    }

    fun select(id: String) {
        _selectedId.value = id
        prefs.edit().putString(KEY_SELECTED, id).apply()
    }

    // ---- Import & edit --------------------------------------------------

    fun import(uri: Uri) = viewModelScope.launch {
        try {
            val draft: ImportDraft = repo.readDraft(uri)
            val s = draft.summary
            _form.value = EditorForm(
                name = draft.suggestedName,
                remote = s.remote,
                needsCredentials = s.needsCredentials,
                twoFactor = s.staticChallenge != null,
                content = draft.content,
                externalFiles = s.externalFiles,
                staticChallenge = s.staticChallenge,
            )
            _events.send(UiEvent.OpenEditor)
        } catch (e: ImportException) {
            _events.send(UiEvent.Error("import", e.message.orEmpty()))
        } catch (e: Exception) {
            _events.send(UiEvent.Error("import", e.message ?: e.javaClass.simpleName))
        }
    }

    fun edit(profile: Profile) {
        _form.value = EditorForm(
            id = profile.id,
            name = profile.name,
            remote = profile.remote,
            username = profile.username,
            needsCredentials = profile.needsCredentials,
            twoFactor = profile.twoFactor,
            codePosition = profile.codePosition,
            codeLength = profile.codeLength,
            rememberPassword = profile.rememberPassword,
            hasSavedPassword = repo.hasSavedPassword(profile.id),
            importedAt = profile.importedAt,
        )
    }

    fun updateForm(transform: (EditorForm) -> EditorForm) {
        _form.value = transform(_form.value)
    }

    fun saveForm(onDone: () -> Unit) = viewModelScope.launch {
        val f = _form.value
        val saved = repo.save(
            Profile(
                id = f.id,
                name = f.name.trim(),
                remote = f.remote,
                username = f.username.trim(),
                needsCredentials = f.needsCredentials,
                twoFactor = f.twoFactor,
                codePosition = f.codePosition,
                codeLength = f.codeLength,
                rememberPassword = f.rememberPassword && f.needsCredentials,
                importedAt = f.importedAt,
            ),
            content = f.content,
            password = f.password.takeIf { f.rememberPassword },
        )
        _form.value = EditorForm()
        select(saved.id)
        onDone()
    }

    fun delete(id: String, onDone: () -> Unit) = viewModelScope.launch {
        if (status.value.profileId == id && status.value.isActive) TunnelService.disconnect(getApplication())
        repo.delete(id)
        if (_selectedId.value == id) _selectedId.value = null
        onDone()
    }

    // ---- Connect --------------------------------------------------------

    /**
     * Starts a connection, or asks for whatever is missing first. The caller
     * must already hold VPN permission.
     */
    fun requestConnect(profile: Profile) {
        val saved = if (profile.rememberPassword) repo.savedPassword(profile.id) else null
        val needsPassword = profile.needsCredentials && saved == null
        val needsCode = profile.needsCredentials && profile.twoFactor
        if (!needsPassword && !needsCode) {
            TunnelService.connect(getApplication(), profile.id, saved, null)
            return
        }
        val usesStaticChallenge = repo.readConfig(profile.id).lineSequence()
            .any { it.trimStart().startsWith("static-challenge") }
        _signIn.value = SignInRequest(profile, needsPassword, needsCode, usesStaticChallenge)
    }

    fun submitSignIn(password: String, code: String) {
        val request = _signIn.value ?: return
        _signIn.value = null
        if (request.profile.managed) {
            viewModelScope.launch {
                val pw = if (request.needsPassword) password else managed.password()
                val otp = code.ifEmpty { null } ?: freshCode()
                TunnelService.connect(getApplication(), request.profile.id, pw, otp)
            }
            return
        }
        val pw = if (request.needsPassword) password else repo.savedPassword(request.profile.id)
        TunnelService.connect(getApplication(), request.profile.id, pw, code.ifEmpty { null })
    }

    fun dismissSignIn() {
        _signIn.value = null
    }

    fun disconnect() {
        cancelRetry()
        autoRetriesLeft = 0
        pendingLink = null
        TunnelService.disconnect(getApplication())
    }

    fun clearLog() = TunnelState.clearLog()

    // ---- Single-config mode ---------------------------------------------

    fun installWithPin(pin: String, onDone: () -> Unit) = install(onDone) { managed.installWithPin(it, pin) }
    fun installWithBiometric(cipher: Cipher, onDone: () -> Unit) = install(onDone) { managed.installWithBiometric(it, cipher) }
    fun installUnprotected(onDone: () -> Unit) = install(onDone) { managed.installUnprotected(it) }

    private fun install(onDone: () -> Unit, block: suspend (SetupPayload) -> Unit) = viewModelScope.launch {
        val payload = pendingSetup ?: return@launch
        if (status.value.isActive) TunnelService.disconnect(getApplication())
        block(payload)
        pendingSetup = null
        onDone()
    }

    fun removeManaged() = viewModelScope.launch {
        if (status.value.isActive) TunnelService.disconnect(getApplication())
        managed.remove()
        TunnelState.update { TunnelStatus() }
    }

    /** Connect the provisioned profile, generating the 2FA code when the secret is stored. */
    fun connectManaged() {
        cancelRetry()
        autoRetriesLeft = MAX_AUTO_RETRIES
        startManaged()
    }

    private fun startManaged() = viewModelScope.launch {
        val cfg = managed.config.value ?: return@launch
        val profile = repo.get(cfg.profileId) ?: return@launch
        val password = managed.password()
        if (profile.needsCredentials && password == null || cfg.manualCode) {
            _signIn.value = SignInRequest(
                profile = profile,
                needsPassword = profile.needsCredentials && password == null,
                needsCode = cfg.manualCode,
                usesStaticChallenge = repo.readConfig(profile.id).contains("static-challenge"),
            )
            return@launch
        }
        TunnelService.connect(getApplication(), profile.id, password, freshCode())
    }

    /** Current TOTP, waiting for the next one if this one is about to expire. */
    private suspend fun freshCode(): String? {
        val totp = managed.totp() ?: return null
        if (totp.secondsLeft() <= 3) {
            _hint.value = getApplication<Application>().getString(R.string.waiting_fresh_code)
            delay(totp.secondsLeft() * 1000L + 300)
            if (pendingLink == null) _hint.value = null
        }
        return totp.code()
    }

    // ---- Automatic retry with the next code ------------------------------

    private var retryJob: Job? = null
    private var autoRetriesLeft = 0

    /** Seconds until the automatic retry, while one is scheduled. */
    private val _retryIn = MutableStateFlow<Int?>(null)
    val retryIn: StateFlow<Int?> = _retryIn.asStateFlow()

    /**
     * A rejected sign-in with a generated code is usually a timing problem
     * (the code rolled over, or the server refuses a code it has seen). Retry
     * once the next code is out, a limited number of times so a real problem
     * doesn't trip the server's brute-force protection.
     */
    private fun shouldAutoRetry(st: TunnelStatus): Boolean {
        val cfg = managed.config.value ?: return false
        return autoRetriesLeft > 0 &&
            st.profileId == cfg.profileId &&
            (st.failure == FailureKind.AuthFailed || st.failure == FailureKind.NeedsSignIn) &&
            managed.totp() != null
    }

    private fun scheduleRetry() {
        val totp = managed.totp() ?: return
        autoRetriesLeft--
        cancelRetry()
        retryJob = viewModelScope.launch {
            // Wait for the next time step, plus a little for clock skew.
            var left = totp.secondsLeft() + 2
            while (left > 0) {
                _retryIn.value = left
                _hint.value = getApplication<Application>().getString(R.string.retry_next_code, left)
                delay(1000)
                left--
            }
            _retryIn.value = null
            _hint.value = pendingLink?.let {
                getApplication<Application>().getString(R.string.link_opens_after_connect, it.title)
            }
            retryJob = null
            startManaged()
        }
    }

    private fun cancelRetry() {
        retryJob?.cancel()
        retryJob = null
        if (_retryIn.value != null) {
            _retryIn.value = null
            _hint.value = null
        }
    }

    fun openLink(link: ManagedLink) {
        if (status.value.phase == Phase.Connected) {
            viewModelScope.launch { _events.send(UiEvent.OpenLink(link)) }
            return
        }
        pendingLink = link
        if (retryJob != null) return // already reconnecting; the link opens once connected
        _hint.value = getApplication<Application>().getString(R.string.link_opens_after_connect, link.title)
        if (!status.value.isActive) connectManaged()
    }

    private companion object {
        const val KEY_SELECTED = "selected_profile"
        const val MAX_AUTO_RETRIES = 2
    }
}
