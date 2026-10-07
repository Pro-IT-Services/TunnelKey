package app.tunnelkey.ui

import android.Manifest
import android.app.Activity
import android.content.ActivityNotFoundException
import android.content.Intent
import android.content.pm.PackageManager
import android.content.res.Configuration
import android.net.Uri
import android.net.VpnService
import android.os.Build
import android.os.Bundle
import android.os.SystemClock
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.core.content.ContextCompat
import androidx.core.content.IntentCompat
import androidx.core.view.WindowCompat
import androidx.fragment.app.FragmentActivity
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavHostController
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import app.tunnelkey.R
import app.tunnelkey.data.Profile
import app.tunnelkey.managed.ManagedLink
import app.tunnelkey.ovpn3.OpenVpnClient
import app.tunnelkey.security.Biometrics
import app.tunnelkey.security.LockMethod
import app.tunnelkey.ui.screens.EditorScreen
import app.tunnelkey.ui.screens.HomeScreen
import app.tunnelkey.ui.screens.LockScreen
import app.tunnelkey.ui.screens.LockSetupScreen
import app.tunnelkey.ui.screens.LogScreen
import app.tunnelkey.ui.screens.ManagedHomeScreen
import app.tunnelkey.ui.screens.PinCreateScreen
import app.tunnelkey.ui.screens.ScanScreen
import app.tunnelkey.ui.screens.SecurityScreen
import app.tunnelkey.ui.screens.SignInSheet
import app.tunnelkey.ui.theme.TunnelkeyTheme
import kotlinx.coroutines.launch

class MainActivity : FragmentActivity() {

    private val vm: MainViewModel by viewModels()
    private var stoppedAt = 0L

    /**
     * Edge-to-edge without the deprecated Window bar-colour setters that
     * enableEdgeToEdge() still calls on Android 8-14 (flagged by Play). The theme
     * makes the bars transparent; Android 15+ enforces that on its own.
     */
    private fun drawEdgeToEdge() {
        WindowCompat.setDecorFitsSystemWindows(window, false)
        val dark = (resources.configuration.uiMode and Configuration.UI_MODE_NIGHT_MASK) == Configuration.UI_MODE_NIGHT_YES
        WindowCompat.getInsetsController(window, window.decorView).apply {
            isAppearanceLightStatusBars = !dark
            isAppearanceLightNavigationBars = !dark
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        drawEdgeToEdge()
        if (savedInstanceState == null) handleIncoming(intent)
        setContent {
            TunnelkeyTheme {
                // Root surface provides the theme's background and content colour, so
                // screens without their own Scaffold (lock, PIN, setup) aren't black-on-ink.
                Surface(Modifier.fillMaxSize(), color = MaterialTheme.colorScheme.background) { App(vm) }
            }
        }
    }

    override fun onStart() {
        super.onStart()
        // Re-lock after the app has been in the background for a while.
        if (stoppedAt != 0L && SystemClock.elapsedRealtime() - stoppedAt > LOCK_AFTER_MS) vm.managed.lock()
        vm.managed.unlockIfUnprotected()
    }

    override fun onStop() {
        super.onStop()
        stoppedAt = SystemClock.elapsedRealtime()
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleIncoming(intent)
    }

    /** "Open with Tunnelkey" or share sheet with an .ovpn file (normal mode only). */
    private fun handleIncoming(intent: Intent?) {
        if (vm.managed.isActive) return
        val uri: Uri? = when (intent?.action) {
            Intent.ACTION_VIEW -> intent.data
            Intent.ACTION_SEND -> IntentCompat.getParcelableExtra(intent, Intent.EXTRA_STREAM, Uri::class.java)
            else -> null
        }
        if (uri != null) vm.import(uri)
    }

    private companion object {
        const val LOCK_AFTER_MS = 30_000L
    }
}

private object Routes {
    const val HOME = "home"
    const val EDITOR = "editor"
    const val LOGS = "logs"
    const val SCAN = "scan"
    const val SETUP = "setup"
    const val SETUP_PIN = "setup-pin"
    const val SECURITY = "security"
    const val CHANGE_PIN = "change-pin"
}

@Composable
private fun App(vm: MainViewModel) {
    val nav = rememberNavController()
    val context = LocalContext.current
    val activity = context as FragmentActivity
    val scope = rememberCoroutineScope()
    val profiles by vm.profiles.collectAsStateWithLifecycle()
    val status by vm.status.collectAsStateWithLifecycle()
    val selectedId by vm.selectedId.collectAsStateWithLifecycle()
    val signIn by vm.signIn.collectAsStateWithLifecycle()
    val form by vm.form.collectAsStateWithLifecycle()
    val log by vm.log.collectAsStateWithLifecycle()
    val managedConfig by vm.managedConfig.collectAsStateWithLifecycle()
    val unlocked by vm.unlocked.collectAsStateWithLifecycle()
    val hint by vm.hint.collectAsStateWithLifecycle()
    val retryIn by vm.retryIn.collectAsStateWithLifecycle()

    val selected = profiles.firstOrNull { it.id == selectedId } ?: profiles.firstOrNull()
    var error by remember { mutableStateOf<Pair<Int, String>?>(null) }
    var showAbout by remember { mutableStateOf(false) }
    var pendingConnect by remember { mutableStateOf<(() -> Unit)?>(null) }
    var missingApp by remember { mutableStateOf<ManagedLink?>(null) }
    var lockMessage by remember { mutableStateOf<String?>(null) }
    var lockMethod by remember { mutableStateOf(vm.managed.lockMethod) }

    val filePicker = rememberLauncherForActivityResult(PickProfileFile()) { uri ->
        if (uri != null) vm.import(uri)
    }
    var showImport by remember { mutableStateOf(false) }

    fun pasteProfile() {
        val clip = context.getSystemService(android.content.ClipboardManager::class.java)
            ?.primaryClip?.takeIf { it.itemCount > 0 }?.getItemAt(0)?.coerceToText(context)?.toString().orEmpty()
        if (!clip.contains("remote")) {
            error = R.string.error_import to context.getString(R.string.import_clipboard_empty)
            return
        }
        vm.importText(clip, context.getString(R.string.import_pasted_name))
    }
    val vpnConsent = rememberLauncherForActivityResult(ActivityResultContracts.StartActivityForResult()) { result ->
        val action = pendingConnect
        pendingConnect = null
        if (result.resultCode == Activity.RESULT_OK && action != null) action()
        else if (action != null) error = R.string.status_failed to context.getString(R.string.error_vpn_permission)
    }
    // Prominent in-app disclosure required by Google Play's VpnService policy,
    // shown once before Android's own VPN consent dialog.
    val uiPrefs = remember { context.getSharedPreferences("ui", android.content.Context.MODE_PRIVATE) }
    var disclosureFor by remember { mutableStateOf<(() -> Unit)?>(null) }

    fun withVpnConsent(action: () -> Unit) {
        if (!uiPrefs.getBoolean(KEY_VPN_DISCLOSURE, false)) {
            disclosureFor = action
            return
        }
        val consent = VpnService.prepare(context)
        if (consent != null) {
            pendingConnect = action
            vpnConsent.launch(consent)
        } else {
            action()
        }
    }

    // Asked first and awaited: launching the VPN consent at the same time
    // dismisses this prompt, and without the permission Android hides the
    // ongoing VPN notification (with its Disconnect button).
    var afterNotificationPrompt by remember { mutableStateOf<(() -> Unit)?>(null) }
    val notificationPermission = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) {
        val next = afterNotificationPrompt
        afterNotificationPrompt = null
        next?.invoke() // connect either way; the notification is optional
    }

    /** Runs [action] once notification permission has been asked for and VPN consent is in place. */
    fun withVpnPermission(action: () -> Unit) {
        if (Build.VERSION.SDK_INT >= 33 &&
            ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            afterNotificationPrompt = { withVpnConsent(action) }
            notificationPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
            return
        }
        withVpnConsent(action)
    }

    fun openLink(link: ManagedLink) {
        try {
            context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(link.uri)))
        } catch (e: ActivityNotFoundException) {
            missingApp = link
        }
    }

    fun unlockWithBiometric() {
        val cipher = vm.managed.vault.biometricDecryptCipher()
        if (cipher == null) {
            lockMessage = context.getString(R.string.lock_biometric_changed)
            vm.removeManaged()
            return
        }
        scope.launch {
            val r = Biometrics.authenticate(
                activity, cipher,
                title = context.getString(R.string.lock_unlock),
                subtitle = context.getString(R.string.biometric_unlock_subtitle, managedConfig?.name.orEmpty()),
                negative = context.getString(R.string.action_cancel),
            )
            when (r) {
                is Biometrics.Result.Success -> runCatching { vm.managed.unlockWithBiometric(r.cipher) }
                    .onFailure { lockMessage = it.message }
                is Biometrics.Result.Error -> lockMessage = r.message
                Biometrics.Result.Cancelled -> Unit
            }
        }
    }

    /** Asks for a biometric confirmation that produces a key for storing secrets. */
    fun enrolBiometric(name: String, onCipher: (javax.crypto.Cipher) -> Unit) {
        scope.launch {
            val r = Biometrics.authenticate(
                activity, vm.managed.vault.biometricEncryptCipher(),
                title = context.getString(R.string.biometric_setup_title),
                subtitle = context.getString(R.string.biometric_setup_subtitle, name),
                negative = context.getString(R.string.action_cancel),
            )
            when (r) {
                is Biometrics.Result.Success -> onCipher(r.cipher)
                is Biometrics.Result.Error -> error = R.string.status_failed to r.message
                Biometrics.Result.Cancelled -> Unit
            }
        }
    }

    fun NavHostController.goHome() = navigate(Routes.HOME) { popUpTo(Routes.HOME) { inclusive = true } }

    LaunchedEffect(Unit) {
        vm.events.collect { e ->
            when (e) {
                UiEvent.OpenEditor -> nav.navigate(Routes.EDITOR) { launchSingleTop = true }
                is UiEvent.Error -> error = R.string.error_import to e.message
                is UiEvent.OpenLink -> openLink(e.link)
            }
        }
    }

    // ---- Lock gate: nothing of a protected configuration is visible while locked.
    val cfg = managedConfig
    if (cfg != null && lockMethod != LockMethod.None && unlocked == null) {
        LockScreen(
            name = cfg.name,
            usesPin = lockMethod == LockMethod.Pin,
            onBiometric = ::unlockWithBiometric,
            onPin = { pin -> vm.managed.unlockWithPin(pin) },
            lockedUntil = vm.managed.vault.lockedUntil,
            message = lockMessage,
        )
        return
    }

    NavHost(navController = nav, startDestination = Routes.HOME) {
        composable(Routes.HOME) {
            if (cfg != null) {
                ManagedHomeScreen(
                    config = cfg,
                    // While waiting to retry with the next code, present it as reconnecting.
                    status = if (retryIn != null) status.copy(phase = app.tunnelkey.vpn.Phase.Reconnecting) else status,
                    hint = hint,
                    onConnect = { withVpnPermission { vm.connectManaged() } },
                    onDisconnect = vm::disconnect,
                    onOpenLink = { link ->
                        if (status.phase == app.tunnelkey.vpn.Phase.Connected) vm.openLink(link)
                        else withVpnPermission { vm.openLink(link) }
                    },
                    onOpenLogs = { nav.navigate(Routes.LOGS) },
                    onSecurity = { nav.navigate(Routes.SECURITY) },
                    onAbout = { showAbout = true },
                    onRemove = { vm.removeManaged(); lockMethod = LockMethod.None },
                )
            } else {
                HomeScreen(
                    profiles = profiles,
                    selected = selected,
                    status = status,
                    onSelect = { vm.select(it.id) },
                    onEdit = { vm.edit(it); nav.navigate(Routes.EDITOR) },
                    onImport = { showImport = true },
                    onScan = { nav.navigate(Routes.SCAN) },
                    onConnect = { p: Profile -> withVpnPermission { vm.requestConnect(p) } },
                    onDisconnect = vm::disconnect,
                    onOpenLogs = { nav.navigate(Routes.LOGS) },
                    onAbout = { showAbout = true },
                )
            }
        }
        composable(Routes.EDITOR) {
            EditorScreen(
                form = form,
                onChange = vm::updateForm,
                onSave = { vm.saveForm { nav.popBackStack(Routes.HOME, false) } },
                onDelete = { vm.delete(form.id) { nav.popBackStack(Routes.HOME, false) } },
                onBack = { nav.popBackStack() },
            )
        }
        composable(Routes.LOGS) {
            LogScreen(lines = log, onClear = vm::clearLog, onBack = { nav.popBackStack() })
        }
        composable(Routes.SCAN) {
            ScanScreen(
                onScanned = { payload ->
                    vm.pendingSetup = payload
                    nav.navigate(Routes.SETUP) { popUpTo(Routes.SCAN) { inclusive = true } }
                },
                onBack = { nav.popBackStack() },
            )
        }
        composable(Routes.SETUP) {
            val payload = vm.pendingSetup
            if (payload == null) {
                LaunchedEffect(Unit) { nav.goHome() }
                return@composable
            }
            LockSetupScreen(
                payload = payload,
                biometricAvailable = Biometrics.available(context),
                replacesExisting = cfg != null,
                onBiometric = {
                    enrolBiometric(payload.name) { cipher ->
                        vm.installWithBiometric(cipher) { lockMethod = LockMethod.Biometric; nav.goHome() }
                    }
                },
                onPin = { nav.navigate(Routes.SETUP_PIN) },
                onNoLock = { vm.installUnprotected { lockMethod = LockMethod.None; nav.goHome() } },
                onBack = { vm.pendingSetup = null; nav.popBackStack() },
            )
        }
        composable(Routes.SETUP_PIN) {
            PinCreateScreen(
                onDone = { pin -> vm.installWithPin(pin) { lockMethod = LockMethod.Pin; nav.goHome() } },
                onBack = { nav.popBackStack() },
            )
        }
        composable(Routes.SECURITY) {
            val c = managedConfig
            SecurityScreen(
                method = lockMethod,
                lockRequired = c?.lockRequired == true,
                biometricAvailable = Biometrics.available(context),
                onUseBiometric = {
                    enrolBiometric(c?.name.orEmpty()) { cipher ->
                        vm.managed.changeToBiometric(cipher)
                        lockMethod = LockMethod.Biometric
                    }
                },
                onUsePin = { nav.navigate(Routes.CHANGE_PIN) },
                onBack = { nav.popBackStack() },
            )
        }
        composable(Routes.CHANGE_PIN) {
            PinCreateScreen(
                onDone = { pin ->
                    scope.launch {
                        vm.managed.changeToPin(pin)
                        lockMethod = LockMethod.Pin
                        nav.popBackStack()
                    }
                },
                onBack = { nav.popBackStack() },
            )
        }
    }

    signIn?.let { request ->
        SignInSheet(request = request, onSubmit = vm::submitSignIn, onDismiss = vm::dismissSignIn)
    }

    error?.let { (title, message) ->
        AlertDialog(
            onDismissRequest = { error = null },
            title = { Text(stringResource(title)) },
            text = { Text(message) },
            confirmButton = { TextButton(onClick = { error = null }) { Text(stringResource(R.string.action_ok)) } },
        )
    }

    missingApp?.let { link ->
        AlertDialog(
            onDismissRequest = { missingApp = null },
            title = { Text(link.title) },
            text = { Text(stringResource(R.string.link_no_app, link.title)) },
            confirmButton = {
                if (link.kind == "rdp") {
                    TextButton(onClick = {
                        missingApp = null
                        runCatching {
                            context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse("market://details?id=$RDP_PACKAGE")))
                        }.onFailure {
                            context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse("https://play.google.com/store/apps/details?id=$RDP_PACKAGE")))
                        }
                    }) { Text(stringResource(R.string.link_get_rdp_app)) }
                } else {
                    TextButton(onClick = { missingApp = null }) { Text(stringResource(R.string.action_ok)) }
                }
            },
            dismissButton = if (link.kind == "rdp") {
                { TextButton(onClick = { missingApp = null }) { Text(stringResource(R.string.action_cancel)) } }
            } else null,
        )
    }

    if (showImport) {
        ImportSheet(
            onChooseFile = { filePicker.launch(Unit) },
            onPaste = ::pasteProfile,
            onScan = { nav.navigate(Routes.SCAN) },
            onDismiss = { showImport = false },
        )
    }

    disclosureFor?.let { pending ->
        AlertDialog(
            onDismissRequest = { disclosureFor = null },
            title = { Text(stringResource(R.string.vpn_disclosure_title)) },
            text = { Text(stringResource(R.string.vpn_disclosure_body)) },
            confirmButton = {
                TextButton(onClick = {
                    uiPrefs.edit().putBoolean(KEY_VPN_DISCLOSURE, true).apply()
                    disclosureFor = null
                    withVpnConsent(pending)
                }) { Text(stringResource(R.string.vpn_disclosure_accept)) }
            },
            dismissButton = { TextButton(onClick = { disclosureFor = null }) { Text(stringResource(R.string.action_cancel)) } },
        )
    }

    if (showAbout) AboutDialog { showAbout = false }
}

private const val KEY_VPN_DISCLOSURE = "vpn_disclosure_accepted"

/** Microsoft's Windows App (formerly Remote Desktop) for Android. */
private const val RDP_PACKAGE = "com.microsoft.rdc.androidx"

@Composable
private fun AboutDialog(onDismiss: () -> Unit) {
    val context = LocalContext.current
    val core = remember { runCatching { OpenVpnClient.coreVersion }.getOrDefault("") }
    val version = remember {
        runCatching { context.packageManager.getPackageInfo(context.packageName, 0).versionName }.getOrNull().orEmpty()
    }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("${stringResource(R.string.app_name)} $version") },
        text = { Text(stringResource(R.string.about_body, core)) },
        confirmButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_ok)) } },
        dismissButton = {
            TextButton(onClick = {
                runCatching {
                    context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(context.getString(R.string.privacy_url))))
                }
            }) { Text(stringResource(R.string.action_privacy)) }
        },
    )
}
