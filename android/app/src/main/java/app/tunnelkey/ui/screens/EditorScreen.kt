package app.tunnelkey.ui.screens

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.DeleteOutline
import androidx.compose.material.icons.outlined.WarningAmber
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
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
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import app.tunnelkey.R
import app.tunnelkey.data.CodePosition
import app.tunnelkey.ui.EditorForm
import app.tunnelkey.ui.theme.NumericStyle

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun EditorScreen(
    form: EditorForm,
    onChange: ((EditorForm) -> EditorForm) -> Unit,
    onSave: () -> Unit,
    onDelete: () -> Unit,
    onBack: () -> Unit,
) {
    var confirmDelete by remember { mutableStateOf(false) }
    val fieldShape = RoundedCornerShape(14.dp)

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(stringResource(if (form.isNew) R.string.editor_title_new else R.string.editor_title_edit)) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.action_back))
                    }
                },
                actions = {
                    if (!form.isNew) {
                        IconButton(onClick = { confirmDelete = true }) {
                            Icon(Icons.Outlined.DeleteOutline, contentDescription = stringResource(R.string.action_delete))
                        }
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background),
            )
        },
        bottomBar = {
            Surface(color = MaterialTheme.colorScheme.background) {
                Column(Modifier.navigationBarsPadding().imePadding()) {
                    HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
                    Button(
                        onClick = onSave,
                        enabled = form.canSave,
                        shape = RoundedCornerShape(16.dp),
                        modifier = Modifier.fillMaxWidth().padding(horizontal = 20.dp, vertical = 12.dp).height(56.dp),
                    ) {
                        Text(stringResource(R.string.action_save), style = MaterialTheme.typography.labelLarge)
                    }
                }
            }
        },
    ) { padding ->
        Column(
            Modifier
                .fillMaxSize()
                .padding(padding)
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 20.dp, vertical = 8.dp),
        ) {
            if (form.remote.isNotEmpty()) {
                Text(
                    form.remote,
                    style = NumericStyle.merge(MaterialTheme.typography.bodyMedium),
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Spacer(Modifier.height(16.dp))
            }

            if (form.externalFiles.isNotEmpty()) {
                Warning(stringResource(R.string.warning_external_files, form.externalFiles.joinToString()))
                Spacer(Modifier.height(12.dp))
            }
            if (form.isNew && form.staticChallenge != null) {
                Warning(stringResource(R.string.warning_static_challenge, form.staticChallenge))
                Spacer(Modifier.height(12.dp))
            }

            OutlinedTextField(
                value = form.name,
                onValueChange = { v -> onChange { it.copy(name = v) } },
                label = { Text(stringResource(R.string.field_name)) },
                singleLine = true,
                shape = fieldShape,
                modifier = Modifier.fillMaxWidth(),
            )

            if (!form.needsCredentials) {
                Spacer(Modifier.height(20.dp))
                Text(
                    stringResource(R.string.profile_no_auth),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                return@Column
            }

            Spacer(Modifier.height(12.dp))
            OutlinedTextField(
                value = form.username,
                onValueChange = { v -> onChange { it.copy(username = v) } },
                label = { Text(stringResource(R.string.field_username)) },
                singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email),
                shape = fieldShape,
                modifier = Modifier.fillMaxWidth(),
            )

            // ---- Two-factor ----
            SectionHeader(stringResource(R.string.section_2fa))
            SwitchRow(
                title = stringResource(R.string.toggle_2fa),
                body = stringResource(R.string.toggle_2fa_body),
                checked = form.twoFactor,
                onCheckedChange = { v -> onChange { it.copy(twoFactor = v) } },
            )
            if (form.twoFactor) {
                Spacer(Modifier.height(16.dp))
                Label(stringResource(R.string.code_position))
                SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
                    val options = listOf(
                        CodePosition.AFTER_PASSWORD to R.string.code_after,
                        CodePosition.BEFORE_PASSWORD to R.string.code_before,
                    )
                    options.forEachIndexed { i, (pos, label) ->
                        SegmentedButton(
                            selected = form.codePosition == pos,
                            onClick = { onChange { it.copy(codePosition = pos) } },
                            shape = SegmentedButtonDefaults.itemShape(i, options.size),
                        ) { Text(stringResource(label)) }
                    }
                }
                Spacer(Modifier.height(8.dp))
                Text(
                    if (form.codePosition == CodePosition.AFTER_PASSWORD) "password123456" else "123456password",
                    style = NumericStyle.merge(MaterialTheme.typography.bodySmall),
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )

                Spacer(Modifier.height(16.dp))
                Label(stringResource(R.string.code_length))
                SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
                    listOf(6, 8).forEachIndexed { i, len ->
                        SegmentedButton(
                            selected = form.codeLength == len,
                            onClick = { onChange { it.copy(codeLength = len) } },
                            shape = SegmentedButtonDefaults.itemShape(i, 2),
                        ) { Text(stringResource(R.string.code_length_value, len)) }
                    }
                }
            }

            // ---- Password ----
            SectionHeader(stringResource(R.string.section_password))
            SwitchRow(
                title = stringResource(R.string.toggle_remember),
                body = stringResource(R.string.toggle_remember_body),
                checked = form.rememberPassword,
                onCheckedChange = { v -> onChange { it.copy(rememberPassword = v) } },
            )
            if (form.rememberPassword) {
                Spacer(Modifier.height(12.dp))
                OutlinedTextField(
                    value = form.password,
                    onValueChange = { v -> onChange { it.copy(password = v) } },
                    label = { Text(stringResource(R.string.field_password)) },
                    supportingText = if (form.hasSavedPassword) {
                        { Text(stringResource(R.string.password_saved_hint)) }
                    } else null,
                    singleLine = true,
                    visualTransformation = PasswordVisualTransformation(),
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
                    shape = fieldShape,
                    modifier = Modifier.fillMaxWidth(),
                )
            }
            Spacer(Modifier.height(24.dp))
        }
    }

    if (confirmDelete) {
        AlertDialog(
            onDismissRequest = { confirmDelete = false },
            title = { Text(stringResource(R.string.delete_confirm_title, form.name)) },
            text = { Text(stringResource(R.string.delete_confirm_body)) },
            confirmButton = {
                TextButton(onClick = { confirmDelete = false; onDelete() }) {
                    Text(stringResource(R.string.action_delete), color = MaterialTheme.colorScheme.error)
                }
            },
            dismissButton = {
                TextButton(onClick = { confirmDelete = false }) { Text(stringResource(R.string.action_cancel)) }
            },
        )
    }
}

@Composable
private fun SectionHeader(text: String) {
    Spacer(Modifier.height(28.dp))
    Text(
        text.uppercase(),
        style = MaterialTheme.typography.labelSmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    Spacer(Modifier.height(8.dp))
}

@Composable
private fun Label(text: String) {
    Text(text, style = MaterialTheme.typography.titleSmall)
    Spacer(Modifier.height(8.dp))
}

@Composable
private fun SwitchRow(title: String, body: String, checked: Boolean, onCheckedChange: (Boolean) -> Unit) {
    Row(
        Modifier
            .fillMaxWidth()
            .toggleable(value = checked, role = Role.Switch, onValueChange = onCheckedChange)
            .padding(vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(Modifier.weight(1f)) {
            Text(title, style = MaterialTheme.typography.bodyLarge)
            Spacer(Modifier.height(2.dp))
            Text(body, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        Spacer(Modifier.width(16.dp))
        Switch(checked = checked, onCheckedChange = null)
    }
}

@Composable
private fun Warning(text: String) {
    Surface(
        color = MaterialTheme.colorScheme.primaryContainer,
        contentColor = MaterialTheme.colorScheme.onPrimaryContainer,
        shape = RoundedCornerShape(14.dp),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Row(Modifier.padding(14.dp)) {
            Icon(Icons.Outlined.WarningAmber, contentDescription = null)
            Spacer(Modifier.width(10.dp))
            Text(text, style = MaterialTheme.typography.bodyMedium)
        }
    }
}
