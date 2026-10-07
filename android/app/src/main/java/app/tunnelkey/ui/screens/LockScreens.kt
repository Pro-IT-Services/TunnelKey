package app.tunnelkey.ui.screens

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Fingerprint
import androidx.compose.material.icons.outlined.Key
import androidx.compose.material.icons.outlined.Link
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.Password
import androidx.compose.material.icons.outlined.Pin
import androidx.compose.material.icons.outlined.VpnKey
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import app.tunnelkey.R
import app.tunnelkey.provision.SetupPayload
import app.tunnelkey.security.PinPolicy
import app.tunnelkey.security.PinResult
import app.tunnelkey.ui.components.PinDots
import app.tunnelkey.ui.components.PinPad
import app.tunnelkey.ui.components.appendPinDigit
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

// ---------------------------------------------------------------- lock setup

/** After scanning: pick how the configuration is protected. */
@Composable
fun LockSetupScreen(
    payload: SetupPayload,
    biometricAvailable: Boolean,
    replacesExisting: Boolean,
    onBiometric: () -> Unit,
    onPin: () -> Unit,
    onNoLock: () -> Unit,
    onBack: () -> Unit,
) {
    val lockRequired = !payload.password.isNullOrEmpty() || payload.totp != null
    Column(
        Modifier
            .fillMaxSize()
            .statusBarsPadding()
            .navigationBarsPadding()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 24.dp),
    ) {
        IconButton(onClick = onBack, modifier = Modifier.padding(top = 4.dp)) {
            Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.action_back))
        }
        Spacer(Modifier.height(8.dp))
        Text(stringResource(R.string.setup_title, payload.name), style = MaterialTheme.typography.headlineSmall)
        Spacer(Modifier.height(20.dp))

        Text(stringResource(R.string.setup_contains), style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Spacer(Modifier.height(10.dp))
        Included(Icons.Outlined.VpnKey, stringResource(R.string.setup_item_vpn))
        if (!payload.password.isNullOrEmpty()) Included(Icons.Outlined.Password, stringResource(R.string.setup_item_password))
        if (payload.totp != null) Included(Icons.Outlined.Key, stringResource(R.string.setup_item_totp))
        if (payload.links.isNotEmpty()) Included(Icons.Outlined.Link, pluralStringResource(R.plurals.setup_item_links, payload.links.size, payload.links.size))

        Spacer(Modifier.height(20.dp))
        Text(
            stringResource(if (lockRequired) R.string.setup_lock_required else R.string.setup_lock_optional),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        if (replacesExisting) {
            Spacer(Modifier.height(8.dp))
            Text(stringResource(R.string.setup_replace_warning), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.error)
        }

        Spacer(Modifier.height(32.dp))
        if (biometricAvailable) {
            Choice(Icons.Outlined.Fingerprint, stringResource(R.string.setup_use_biometric), stringResource(R.string.setup_use_biometric_body), onBiometric)
            Spacer(Modifier.height(12.dp))
        }
        Choice(Icons.Outlined.Pin, stringResource(R.string.setup_use_pin), stringResource(R.string.setup_use_pin_body), onPin)
        if (!lockRequired) {
            TextButton(onClick = onNoLock, modifier = Modifier.fillMaxWidth().padding(top = 8.dp).heightIn(min = 48.dp)) {
                Text(stringResource(R.string.setup_no_lock))
            }
        }
        Spacer(Modifier.height(16.dp))
    }
}

@Composable
private fun Included(icon: ImageVector, text: String) {
    Row(Modifier.padding(vertical = 6.dp), verticalAlignment = Alignment.CenterVertically) {
        Icon(icon, contentDescription = null, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(20.dp))
        Spacer(Modifier.width(12.dp))
        Text(text, style = MaterialTheme.typography.bodyLarge)
    }
}

@Composable
private fun Choice(icon: ImageVector, title: String, body: String, onClick: () -> Unit) {
    Surface(
        onClick = onClick,
        shape = RoundedCornerShape(16.dp),
        color = MaterialTheme.colorScheme.surfaceContainer,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Row(Modifier.padding(18.dp).heightIn(min = 40.dp), verticalAlignment = Alignment.CenterVertically) {
            Icon(icon, contentDescription = null, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(28.dp))
            Spacer(Modifier.width(16.dp))
            Column {
                Text(title, style = MaterialTheme.typography.titleMedium)
                Text(body, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
    }
}

// ---------------------------------------------------------------- PIN creation

/** Two-step PIN entry with the strength rules from [PinPolicy]. */
@Composable
fun PinCreateScreen(onDone: (String) -> Unit, onBack: () -> Unit) {
    var first by remember { mutableStateOf<String?>(null) }
    var entry by remember { mutableStateOf("") }
    var error by remember { mutableStateOf<String?>(null) }
    var errorKey by remember { mutableIntStateOf(0) }
    val context = LocalContext.current

    fun complete(pin: String) {
        val confirmed = first
        if (confirmed == null) {
            val problem = PinPolicy.check(pin)
            if (problem != null) {
                error = context.getString(problem.message())
                errorKey++
                entry = ""
            } else {
                first = pin
                entry = ""
                error = null
            }
        } else if (pin == confirmed) {
            onDone(pin)
        } else {
            error = context.getString(R.string.pin_mismatch)
            errorKey++
            first = null
            entry = ""
        }
    }

    PinScaffold(
        title = stringResource(if (first == null) R.string.pin_create_title else R.string.pin_confirm_title),
        subtitle = error ?: if (first == null) stringResource(R.string.pin_create_body) else "",
        subtitleIsError = error != null,
        filled = entry.length,
        errorKey = errorKey,
        onBack = onBack,
        onDigit = { c ->
            entry = entry.appendPinDigit(c)
            if (entry.length == PinPolicy.LENGTH) complete(entry)
        },
        onDelete = { entry = entry.dropLast(1) },
    )
}

// ---------------------------------------------------------------- lock screen

@Composable
fun LockScreen(
    name: String,
    usesPin: Boolean,
    onBiometric: () -> Unit,
    onPin: suspend (String) -> PinResult,
    lockedUntil: Long,
    message: String?,
) {
    var entry by remember { mutableStateOf("") }
    var error by remember { mutableStateOf(message) }
    var errorKey by remember { mutableIntStateOf(0) }
    var busy by remember { mutableStateOf(false) }
    var until by remember { mutableLongStateOf(lockedUntil) }
    var now by remember { mutableLongStateOf(System.currentTimeMillis()) }
    val scope = rememberCoroutineScope()
    val context = LocalContext.current

    LaunchedEffect(Unit) { if (!usesPin) onBiometric() }
    LaunchedEffect(until) {
        while (System.currentTimeMillis() < until) {
            now = System.currentTimeMillis()
            delay(1000)
        }
        now = System.currentTimeMillis()
    }
    val waiting = now < until
    val waitText = if (waiting) {
        val s = (until - now) / 1000 + 1
        context.getString(R.string.lock_wait, if (s >= 60) "${s / 60} min ${s % 60} s" else "$s s")
    } else null

    if (!usesPin) {
        Column(
            Modifier.fillMaxSize().statusBarsPadding().navigationBarsPadding().padding(24.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.Center,
        ) {
            Icon(Icons.Outlined.Lock, contentDescription = null, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(44.dp))
            Spacer(Modifier.height(16.dp))
            Text(name, style = MaterialTheme.typography.headlineSmall, textAlign = TextAlign.Center)
            Spacer(Modifier.height(6.dp))
            Text(stringResource(R.string.lock_title), color = MaterialTheme.colorScheme.onSurfaceVariant)
            error?.let {
                Spacer(Modifier.height(16.dp))
                Text(it, color = MaterialTheme.colorScheme.error, textAlign = TextAlign.Center)
            }
            Spacer(Modifier.height(40.dp))
            androidx.compose.material3.Button(
                onClick = onBiometric,
                shape = RoundedCornerShape(16.dp),
                modifier = Modifier.fillMaxWidth().height(56.dp),
            ) {
                Icon(Icons.Outlined.Fingerprint, contentDescription = null)
                Spacer(Modifier.width(10.dp))
                Text(stringResource(R.string.lock_unlock))
            }
        }
        return
    }

    PinScaffold(
        title = name,
        subtitle = waitText ?: error ?: stringResource(R.string.lock_enter_pin),
        subtitleIsError = waitText != null || error != null,
        filled = entry.length,
        errorKey = errorKey,
        onBack = null,
        enabled = !busy && !waiting,
        onDigit = { c ->
            entry = entry.appendPinDigit(c)
            if (entry.length == PinPolicy.LENGTH) {
                val pin = entry
                busy = true
                scope.launch {
                    when (val r = onPin(pin)) {
                        is PinResult.Unlocked -> Unit
                        is PinResult.Wrong -> {
                            error = context.resources.getQuantityString(R.plurals.lock_wrong_pin, r.attemptsLeft, r.attemptsLeft)
                            until = r.lockedUntil
                            errorKey++
                        }
                        is PinResult.LockedOut -> until = r.until
                        PinResult.Wiped -> error = context.getString(R.string.lock_wiped)
                    }
                    entry = ""
                    busy = false
                }
            }
        },
        onDelete = { entry = entry.dropLast(1) },
    )
}

@Composable
private fun PinScaffold(
    title: String,
    subtitle: String,
    subtitleIsError: Boolean,
    filled: Int,
    errorKey: Int,
    onBack: (() -> Unit)?,
    onDigit: (Char) -> Unit,
    onDelete: () -> Unit,
    enabled: Boolean = true,
) {
    Column(
        Modifier.fillMaxSize().statusBarsPadding().navigationBarsPadding().padding(bottom = 24.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Row(Modifier.fillMaxWidth().padding(4.dp)) {
            if (onBack != null) {
                IconButton(onClick = onBack) {
                    Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.action_back))
                }
            } else {
                Spacer(Modifier.height(48.dp))
            }
        }
        Spacer(Modifier.weight(1f))
        Icon(Icons.Outlined.Lock, contentDescription = null, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(36.dp))
        Spacer(Modifier.height(14.dp))
        Text(title, style = MaterialTheme.typography.headlineSmall, textAlign = TextAlign.Center, modifier = Modifier.padding(horizontal = 24.dp))
        Spacer(Modifier.height(8.dp))
        Text(
            subtitle,
            style = MaterialTheme.typography.bodyMedium,
            color = if (subtitleIsError) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center,
            modifier = Modifier.padding(horizontal = 32.dp).heightIn(min = 40.dp).semantics { liveRegion = LiveRegionMode.Polite },
        )
        Spacer(Modifier.height(20.dp))
        PinDots(PinPolicy.LENGTH, filled, errorKey, error = subtitleIsError && filled == 0)
        Spacer(Modifier.weight(1f))
        PinPad(onDigit = onDigit, onDelete = onDelete, enabled = enabled)
    }
}

fun PinPolicy.Problem.message(): Int = when (this) {
    PinPolicy.Problem.Length -> R.string.pin_problem_Length
    PinPolicy.Problem.TooFewDigits -> R.string.pin_problem_TooFewDigits
    PinPolicy.Problem.RepeatedDigit -> R.string.pin_problem_RepeatedDigit
    PinPolicy.Problem.Sequence -> R.string.pin_problem_Sequence
    PinPolicy.Problem.Progression -> R.string.pin_problem_Progression
    PinPolicy.Problem.RepeatedBlock -> R.string.pin_problem_RepeatedBlock
    PinPolicy.Problem.Pairs -> R.string.pin_problem_Pairs
    PinPolicy.Problem.Mirror -> R.string.pin_problem_Mirror
    PinPolicy.Problem.Date -> R.string.pin_problem_Date
    PinPolicy.Problem.Common -> R.string.pin_problem_Common
}
