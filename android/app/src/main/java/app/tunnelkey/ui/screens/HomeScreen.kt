package app.tunnelkey.ui.screens

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material.icons.outlined.Key
import androidx.compose.material.icons.outlined.MoreVert
import androidx.compose.material.icons.automirrored.outlined.Notes
import androidx.compose.material.icons.outlined.QrCodeScanner
import androidx.compose.material.icons.outlined.Tune
import androidx.compose.material3.Button
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import app.tunnelkey.R
import app.tunnelkey.data.Profile
import app.tunnelkey.ui.components.StatusHero
import app.tunnelkey.ui.theme.LocalSignals
import app.tunnelkey.ui.theme.NumericStyle
import app.tunnelkey.vpn.Phase
import app.tunnelkey.vpn.TunnelStatus

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HomeScreen(
    profiles: List<Profile>,
    selected: Profile?,
    status: TunnelStatus,
    onSelect: (Profile) -> Unit,
    onEdit: (Profile) -> Unit,
    onImport: () -> Unit,
    onScan: () -> Unit,
    onConnect: (Profile) -> Unit,
    onDisconnect: () -> Unit,
    onOpenLogs: () -> Unit,
    onAbout: () -> Unit,
) {
    var menuOpen by remember { mutableStateOf(false) }
    val activeProfile = profiles.firstOrNull { it.id == status.profileId && status.isActive }
    val heroProfile = activeProfile ?: selected

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.app_name), style = MaterialTheme.typography.titleLarge) },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background),
                actions = {
                    IconButton(onClick = onScan) {
                        Icon(Icons.Outlined.QrCodeScanner, contentDescription = stringResource(R.string.action_scan))
                    }
                    IconButton(onClick = onImport) {
                        Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.action_import))
                    }
                    Box {
                        IconButton(onClick = { menuOpen = true }) {
                            Icon(Icons.Outlined.MoreVert, contentDescription = null)
                        }
                        DropdownMenu(expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
                            DropdownMenuItem(
                                text = { Text(stringResource(R.string.action_logs)) },
                                leadingIcon = { Icon(Icons.AutoMirrored.Outlined.Notes, null) },
                                onClick = { menuOpen = false; onOpenLogs() },
                            )
                            DropdownMenuItem(
                                text = { Text(stringResource(R.string.action_about)) },
                                leadingIcon = { Icon(Icons.Outlined.Info, null) },
                                onClick = { menuOpen = false; onAbout() },
                            )
                        }
                    }
                },
            )
        },
        bottomBar = {
            if (profiles.isNotEmpty()) {
                ConnectBar(status = status, enabled = selected != null, onConnect = { selected?.let(onConnect) }, onDisconnect = onDisconnect)
            }
        },
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
                StatusHero(status = status, profileName = heroProfile?.name, modifier = Modifier.padding(vertical = 16.dp))
            }

            if (profiles.isEmpty()) {
                item(key = "empty") { EmptyState(onImport, onScan) }
            } else {
                item(key = "header") {
                    Text(
                        stringResource(R.string.profiles_header).uppercase(),
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(top = 12.dp, bottom = 2.dp, start = 4.dp),
                    )
                }
                items(profiles, key = { it.id }) { p ->
                    ProfileRow(
                        profile = p,
                        selected = p.id == selected?.id,
                        live = p.id == status.profileId && status.phase == Phase.Connected,
                        enabled = !status.isActive,
                        onSelect = { onSelect(p) },
                        onEdit = { onEdit(p) },
                    )
                }
            }
        }
    }
}

@Composable
private fun ProfileRow(
    profile: Profile,
    selected: Boolean,
    live: Boolean,
    enabled: Boolean,
    onSelect: () -> Unit,
    onEdit: () -> Unit,
) {
    val colors = MaterialTheme.colorScheme
    Surface(
        shape = RoundedCornerShape(16.dp),
        color = colors.surfaceContainer,
        border = BorderStroke(if (selected) 1.5.dp else 1.dp, if (selected) colors.primary else colors.outlineVariant),
        modifier = Modifier
            .fillMaxWidth()
            .selectable(selected = selected, enabled = enabled, role = Role.RadioButton, onClick = onSelect),
    ) {
        Row(
            Modifier.heightIn(min = 72.dp).padding(start = 8.dp, end = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            RadioButton(selected = selected, onClick = null, enabled = enabled, modifier = Modifier.padding(8.dp))
            Column(Modifier.weight(1f).padding(vertical = 12.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        profile.name,
                        style = MaterialTheme.typography.titleMedium,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f, fill = false),
                    )
                    if (profile.twoFactor) {
                        Spacer(Modifier.width(8.dp))
                        TwoFactorBadge()
                    }
                }
                if (profile.remote.isNotEmpty()) {
                    Spacer(Modifier.height(2.dp))
                    Text(
                        profile.remote,
                        style = NumericStyle.merge(MaterialTheme.typography.bodySmall),
                        color = if (live) LocalSignals.current.secure else colors.onSurfaceVariant,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
            }
            IconButton(onClick = onEdit, enabled = enabled) {
                Icon(Icons.Outlined.Tune, contentDescription = stringResource(R.string.action_edit))
            }
        }
    }
}

@Composable
private fun TwoFactorBadge() {
    Surface(
        color = MaterialTheme.colorScheme.primaryContainer,
        contentColor = MaterialTheme.colorScheme.onPrimaryContainer,
        shape = RoundedCornerShape(6.dp),
    ) {
        Row(Modifier.padding(horizontal = 6.dp, vertical = 2.dp), verticalAlignment = Alignment.CenterVertically) {
            Icon(Icons.Outlined.Key, contentDescription = null, modifier = Modifier.size(12.dp))
            Spacer(Modifier.width(3.dp))
            Text(stringResource(R.string.badge_2fa), style = MaterialTheme.typography.labelSmall)
        }
    }
}

@Composable
private fun EmptyState(onImport: () -> Unit, onScan: () -> Unit) {
    Column(
        Modifier.fillMaxWidth().padding(top = 8.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(stringResource(R.string.empty_title), style = MaterialTheme.typography.titleMedium)
        Spacer(Modifier.height(6.dp))
        Text(
            stringResource(R.string.empty_body),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center,
        )
        Spacer(Modifier.height(20.dp))
        Button(onClick = onImport, modifier = Modifier.fillMaxWidth().height(56.dp), shape = RoundedCornerShape(16.dp)) {
            Icon(Icons.Outlined.Add, contentDescription = null)
            Spacer(Modifier.width(8.dp))
            Text(stringResource(R.string.action_import))
        }
        Spacer(Modifier.height(10.dp))
        OutlinedButton(onClick = onScan, modifier = Modifier.fillMaxWidth().height(56.dp), shape = RoundedCornerShape(16.dp)) {
            Icon(Icons.Outlined.QrCodeScanner, contentDescription = null)
            Spacer(Modifier.width(8.dp))
            Text(stringResource(R.string.action_scan))
        }
    }
}

@Composable
internal fun ConnectBar(status: TunnelStatus, enabled: Boolean, onConnect: () -> Unit, onDisconnect: () -> Unit) {
    Surface(color = MaterialTheme.colorScheme.background) {
        Column(Modifier.navigationBarsPadding()) {
            HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
            val shape = RoundedCornerShape(16.dp)
            val modifier = Modifier.fillMaxWidth().padding(horizontal = 20.dp, vertical = 12.dp).height(56.dp)
            when (status.phase) {
                Phase.Connected -> OutlinedButton(onClick = onDisconnect, modifier = modifier, shape = shape) {
                    Text(stringResource(R.string.action_disconnect), style = MaterialTheme.typography.labelLarge)
                }
                Phase.Connecting, Phase.Reconnecting -> OutlinedButton(onClick = onDisconnect, modifier = modifier, shape = shape) {
                    Text(stringResource(R.string.action_cancel), style = MaterialTheme.typography.labelLarge)
                }
                Phase.Disconnecting -> OutlinedButton(onClick = {}, enabled = false, modifier = modifier, shape = shape) {
                    Text(stringResource(R.string.status_disconnecting), style = MaterialTheme.typography.labelLarge)
                }
                Phase.Disconnected, Phase.Failed -> Button(onClick = onConnect, enabled = enabled, modifier = modifier, shape = shape) {
                    Text(stringResource(R.string.action_connect), style = MaterialTheme.typography.labelLarge)
                }
            }
        }
    }
}
