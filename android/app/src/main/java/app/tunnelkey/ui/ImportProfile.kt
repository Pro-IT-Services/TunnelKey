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
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.ContentPaste
import androidx.compose.material.icons.outlined.FolderOpen
import androidx.compose.material.icons.outlined.QrCodeScanner
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
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
