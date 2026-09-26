package app.tunnelkey.ui.screens

import android.net.Uri
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.automirrored.outlined.OpenInNew
import androidx.compose.material.icons.outlined.Apps
import androidx.compose.material.icons.outlined.Computer
import androidx.compose.material.icons.outlined.DeleteOutline
import androidx.compose.material.icons.outlined.Fingerprint
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material.icons.outlined.Language
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.MoreVert
import androidx.compose.material.icons.automirrored.outlined.Notes
import androidx.compose.material.icons.outlined.Pin
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import app.tunnelkey.R
import app.tunnelkey.managed.ManagedConfig
import app.tunnelkey.managed.ManagedLink
import app.tunnelkey.security.LockMethod
import app.tunnelkey.ui.components.StatusHero
import app.tunnelkey.ui.theme.NumericStyle
import app.tunnelkey.vpn.TunnelStatus

/** Single-configuration mode: the config name is the title, links sit under the tunnel. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ManagedHomeScreen(
    config: ManagedConfig,
    status: TunnelStatus,
    hint: String?,
    onConnect: () -> Unit,
    onDisconnect: () -> Unit,
    onOpenLink: (ManagedLink) -> Unit,
    onOpenLogs: () -> Unit,
    onSecurity: () -> Unit,
    onAbout: () -> Unit,
    onRemove: () -> Unit,
) {
    var menuOpen by remember { mutableStateOf(false) }
    var confirmRemove by remember { mutableStateOf(false) }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(config.name, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background),
                actions = {
                    Box {
                        IconButton(onClick = { menuOpen = true }) { Icon(Icons.Outlined.MoreVert, contentDescription = null) }
                        DropdownMenu(expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
                            DropdownMenuItem(
                                text = { Text(stringResource(R.string.action_logs)) },
                                leadingIcon = { Icon(Icons.AutoMirrored.Outlined.Notes, null) },
                                onClick = { menuOpen = false; onOpenLogs() },
                            )
                            DropdownMenuItem(
                                text = { Text(stringResource(R.string.action_settings)) },
                                leadingIcon = { Icon(Icons.Outlined.Lock, null) },
                                onClick = { menuOpen = false; onSecurity() },
                            )
                            DropdownMenuItem(
                                text = { Text(stringResource(R.string.action_about)) },
                                leadingIcon = { Icon(Icons.Outlined.Info, null) },
                                onClick = { menuOpen = false; onAbout() },
                            )
                            DropdownMenuItem(
                                text = { Text(stringResource(R.string.action_remove_config), color = MaterialTheme.colorScheme.error) },
                                leadingIcon = { Icon(Icons.Outlined.DeleteOutline, null, tint = MaterialTheme.colorScheme.error) },
                                onClick = { menuOpen = false; confirmRemove = true },
                            )
                        }
                    }
                },
            )
        },
        bottomBar = { ConnectBar(status = status, enabled = true, onConnect = onConnect, onDisconnect = onDisconnect) },
    ) { padding ->
        LazyColumn(
            modifier = Modifier.fillMaxSize(),
            contentPadding = PaddingValues(
                start = 20.dp, end = 20.dp,
                top = padding.calculateTopPadding() + 8.dp,
                bottom = padding.calculateBottomPadding() + 16.dp,
            ),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            item(key = "hero") {
                StatusHero(status = status, profileName = config.username.ifEmpty { null }, modifier = Modifier.padding(vertical = 16.dp))
            }
            if (hint != null) {
                item(key = "hint") {
                    Text(hint, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.fillMaxWidth())
                }
            }
            if (config.links.isNotEmpty()) {
                item(key = "links-header") {
                    Text(
                        stringResource(R.string.links_header).uppercase(),
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(top = 12.dp, bottom = 2.dp, start = 4.dp),
                    )
                }
                itemsIndexed(config.links, key = { i, _ -> "link-$i" }) { _, link ->
                    LinkRow(link) { onOpenLink(link) }
                }
            }
        }
    }

    if (confirmRemove) {
        AlertDialog(
            onDismissRequest = { confirmRemove = false },
            title = { Text(stringResource(R.string.remove_confirm_title, config.name)) },
            text = { Text(stringResource(R.string.remove_confirm_body)) },
            confirmButton = {
                TextButton(onClick = { confirmRemove = false; onRemove() }) {
                    Text(stringResource(R.string.action_remove), color = MaterialTheme.colorScheme.error)
                }
            },
            dismissButton = { TextButton(onClick = { confirmRemove = false }) { Text(stringResource(R.string.action_cancel)) } },
        )
    }
}

@Composable
private fun LinkRow(link: ManagedLink, onClick: () -> Unit) {
    val colors = MaterialTheme.colorScheme
    Surface(
        onClick = onClick,
        shape = RoundedCornerShape(16.dp),
        color = colors.surfaceContainer,
        border = BorderStroke(1.dp, colors.outlineVariant),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Row(Modifier.heightIn(min = 68.dp).padding(horizontal = 16.dp), verticalAlignment = Alignment.CenterVertically) {
            Surface(color = colors.primaryContainer, contentColor = colors.onPrimaryContainer, shape = RoundedCornerShape(10.dp)) {
                Icon(linkIcon(link.kind), contentDescription = null, modifier = Modifier.padding(8.dp).size(22.dp))
            }
            Spacer(Modifier.width(14.dp))
            Column(Modifier.weight(1f).padding(vertical = 12.dp)) {
                Text(link.title, style = MaterialTheme.typography.titleMedium, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Text(
                    linkDetail(link),
                    style = NumericStyle.merge(MaterialTheme.typography.bodySmall),
                    color = colors.onSurfaceVariant,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
            Icon(Icons.AutoMirrored.Outlined.OpenInNew, contentDescription = null, tint = colors.onSurfaceVariant, modifier = Modifier.size(18.dp))
        }
    }
}

private fun linkIcon(kind: String): ImageVector = when (kind) {
    "rdp" -> Icons.Outlined.Computer
    "app" -> Icons.Outlined.Apps
    else -> Icons.Outlined.Language
}

/** Short, human-readable target: host for web, computer for RDP. */
private fun linkDetail(link: ManagedLink): String = when (link.kind) {
    "rdp" -> Regex("full%20address=s:([^&]+)").find(link.uri)?.groupValues?.get(1)?.let(Uri::decode) ?: "Remote Desktop"
    "web" -> Uri.parse(link.uri).host ?: link.uri
    else -> link.uri.substringBefore("?")
}

// ---------------------------------------------------------------- security

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SecurityScreen(
    method: LockMethod,
    lockRequired: Boolean,
    biometricAvailable: Boolean,
    onUseBiometric: () -> Unit,
    onUsePin: () -> Unit,
    onBack: () -> Unit,
) {
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.security_title)) },
                navigationIcon = {
                    IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Outlined.ArrowBack, stringResource(R.string.action_back)) }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background),
            )
        },
    ) { padding ->
        Column(Modifier.padding(padding).padding(20.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(
                stringResource(
                    when (method) {
                        LockMethod.Biometric -> R.string.security_current_biometric
                        LockMethod.Pin -> R.string.security_current_pin
                        LockMethod.None -> R.string.security_current_none
                    },
                ),
                style = MaterialTheme.typography.titleMedium,
            )
            if (lockRequired) {
                Text(stringResource(R.string.setup_lock_required), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            Spacer(Modifier.size(8.dp))
            if (biometricAvailable && method != LockMethod.Biometric) {
                SettingRow(Icons.Outlined.Fingerprint, stringResource(R.string.security_switch_biometric), onUseBiometric)
            }
            SettingRow(
                Icons.Outlined.Pin,
                stringResource(if (method == LockMethod.Pin) R.string.security_change_pin else R.string.security_switch_pin),
                onUsePin,
            )
        }
    }
}

@Composable
private fun SettingRow(icon: ImageVector, title: String, onClick: () -> Unit) {
    Surface(
        onClick = onClick,
        shape = RoundedCornerShape(16.dp),
        color = MaterialTheme.colorScheme.surfaceContainer,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Row(Modifier.heightIn(min = 60.dp).padding(horizontal = 18.dp), verticalAlignment = Alignment.CenterVertically) {
            Icon(icon, contentDescription = null, tint = MaterialTheme.colorScheme.primary)
            Spacer(Modifier.width(14.dp))
            Text(title, style = MaterialTheme.typography.bodyLarge)
        }
    }
}
