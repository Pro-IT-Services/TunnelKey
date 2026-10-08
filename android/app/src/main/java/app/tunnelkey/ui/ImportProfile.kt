package app.tunnelkey.ui

import android.app.Activity
import android.content.Context
import android.content.Intent
import android.net.Uri
import androidx.activity.result.contract.ActivityResultContract
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.ContentPaste
import androidx.compose.material.icons.outlined.FolderOpen
import androidx.compose.material.icons.outlined.QrCodeScanner
import androidx.compose.material.icons.outlined.Visibility
import androidx.compose.material.icons.outlined.VisibilityOff
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp
import app.tunnelkey.R

/**
 * Picks a profile with ACTION_GET_CONTENT, offered as a chooser so the user's
 * file manager (e.g. Samsung My Files) does the browsing and sees every
 * folder. Some system pickers list nothing for files copied over USB/adb.
 */
class PickProfileFile : ActivityResultContract<Unit, Uri?>() {
    override fun createIntent(context: Context, input: Unit): Intent {
        val pick = Intent(Intent.ACTION_GET_CONTENT)
            .setType("*/*")
            .addCategory(Intent.CATEGORY_OPENABLE)
        return Intent.createChooser(pick, context.getString(R.string.import_chooser_title))
    }

    override fun parseResult(resultCode: Int, intent: Intent?): Uri? =
        if (resultCode == Activity.RESULT_OK) intent?.data else null
}

/** Password prompt for an opened .tunnelkey setup file. */
@Composable
fun SetupFileDialog(request: SetupFileRequest, onOpen: (String) -> Unit, onDismiss: () -> Unit) {
    var password by remember(request.text) { mutableStateOf("") }
    var shown by remember { mutableStateOf(false) }
    AlertDialog(
        onDismissRequest = { if (!request.busy) onDismiss() },
        title = { Text(stringResource(R.string.setup_file_title)) },
        text = {
            Column {
                Text(stringResource(R.string.setup_file_body))
                Spacer(Modifier.size(4.dp))
                Text(request.name, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                Spacer(Modifier.size(12.dp))
                OutlinedTextField(
                    value = password,
                    onValueChange = { password = it },
                    label = { Text(stringResource(R.string.setup_file_password)) },
                    singleLine = true,
                    enabled = !request.busy,
                    isError = request.error != null,
                    visualTransformation = if (shown) VisualTransformation.None else PasswordVisualTransformation(),
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Done),
                    keyboardActions = KeyboardActions(onDone = { if (password.isNotEmpty() && !request.busy) onOpen(password) }),
                    trailingIcon = {
                        IconButton(onClick = { shown = !shown }) {
                            Icon(
                                if (shown) Icons.Outlined.VisibilityOff else Icons.Outlined.Visibility,
                                contentDescription = stringResource(if (shown) R.string.action_hide_password else R.string.action_show_password),
                            )
                        }
                    },
                    modifier = Modifier.fillMaxWidth(),
                )
                request.error?.let {
                    Spacer(Modifier.size(8.dp))
                    Text(stringResource(it), color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium)
                }
                if (request.busy) {
                    Spacer(Modifier.size(12.dp))
                    LinearProgressIndicator(Modifier.fillMaxWidth())
                }
            }
        },
        confirmButton = {
            TextButton(onClick = { onOpen(password) }, enabled = password.isNotEmpty() && !request.busy) {
                Text(stringResource(R.string.setup_file_open))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss, enabled = !request.busy) { Text(stringResource(R.string.action_cancel)) }
        },
    )
}

/** Ways to add a profile: file, clipboard, setup code. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ImportSheet(
    onChooseFile: () -> Unit,
    onPaste: () -> Unit,
    onScan: () -> Unit,
    onDismiss: () -> Unit,
) {
    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        containerColor = MaterialTheme.colorScheme.surfaceContainerLow,
    ) {
        Column(
            Modifier
                .fillMaxWidth()
                .padding(horizontal = 20.dp)
                .navigationBarsPadding()
                .padding(bottom = 16.dp),
        ) {
            Text(
                stringResource(R.string.import_sheet_title),
                style = MaterialTheme.typography.titleLarge,
                modifier = Modifier.padding(start = 4.dp, bottom = 16.dp),
            )
            Option(Icons.Outlined.FolderOpen, stringResource(R.string.import_choose_file), stringResource(R.string.import_choose_file_body)) {
                onDismiss(); onChooseFile()
            }
            Spacer(Modifier.size(10.dp))
            Option(Icons.Outlined.ContentPaste, stringResource(R.string.import_paste), stringResource(R.string.import_paste_body)) {
                onDismiss(); onPaste()
            }
            Spacer(Modifier.size(10.dp))
            Option(Icons.Outlined.QrCodeScanner, stringResource(R.string.action_scan), stringResource(R.string.import_scan_body)) {
                onDismiss(); onScan()
            }
        }
    }
}

@Composable
private fun Option(icon: ImageVector, title: String, body: String, onClick: () -> Unit) {
    Surface(
        onClick = onClick,
        shape = RoundedCornerShape(16.dp),
        color = MaterialTheme.colorScheme.surfaceContainerHigh,
        modifier = Modifier.fillMaxWidth(),
    ) {
        Row(
            Modifier.heightIn(min = 68.dp).padding(horizontal = 16.dp, vertical = 12.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Icon(icon, contentDescription = null, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(26.dp))
            Spacer(Modifier.width(16.dp))
            Column {
                Text(title, style = MaterialTheme.typography.titleMedium)
                Text(body, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
    }
}
