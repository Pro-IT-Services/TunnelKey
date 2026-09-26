package app.tunnelkey.ui.screens

import android.content.ClipboardManager
import android.content.Context
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.ContentPaste
import androidx.compose.material.icons.outlined.Visibility
import androidx.compose.material.icons.outlined.VisibilityOff
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import app.tunnelkey.R
import app.tunnelkey.data.CodePosition
import app.tunnelkey.data.Credentials
import app.tunnelkey.ui.SignInRequest
import app.tunnelkey.ui.components.CodeField
import app.tunnelkey.ui.theme.NumericStyle

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SignInSheet(request: SignInRequest, onSubmit: (password: String, code: String) -> Unit, onDismiss: () -> Unit) {
    val profile = request.profile
    val context = LocalContext.current
    var password by rememberSaveable { mutableStateOf("") }
    var code by rememberSaveable { mutableStateOf("") }
    var showPassword by rememberSaveable { mutableStateOf(false) }
    val passwordFocus = remember { FocusRequester() }
    val codeFocus = remember { FocusRequester() }

    val passwordOk = !request.needsPassword || password.isNotEmpty()
    val codeOk = !request.needsCode || Credentials.isValidCode(code, profile.codeLength)
    val canSubmit = passwordOk && codeOk
    val submit = { if (canSubmit) onSubmit(password, code) }

    LaunchedEffect(Unit) {
        if (request.needsPassword) passwordFocus.requestFocus() else if (request.needsCode) codeFocus.requestFocus()
    }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        containerColor = MaterialTheme.colorScheme.surfaceContainerLow,
    ) {
        Column(
            Modifier
                .fillMaxWidth()
                .padding(horizontal = 24.dp)
                .navigationBarsPadding()
                .imePadding()
                .padding(bottom = 16.dp),
        ) {
            Text(stringResource(R.string.signin_title, profile.name), style = MaterialTheme.typography.titleLarge)
            if (profile.username.isNotEmpty()) {
                Spacer(Modifier.height(4.dp))
                Text(
                    stringResource(R.string.signin_user, profile.username),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Spacer(Modifier.height(20.dp))

            if (request.needsPassword) {
                OutlinedTextField(
                    value = password,
                    onValueChange = { password = it },
                    label = { Text(stringResource(R.string.field_password)) },
                    singleLine = true,
                    visualTransformation = if (showPassword) VisualTransformation.None else PasswordVisualTransformation(),
                    keyboardOptions = KeyboardOptions(
                        keyboardType = KeyboardType.Password,
                        imeAction = if (request.needsCode) ImeAction.Next else ImeAction.Done,
                    ),
                    keyboardActions = KeyboardActions(
                        onNext = { codeFocus.requestFocus() },
                        onDone = { submit() },
                    ),
                    trailingIcon = {
                        IconButton(onClick = { showPassword = !showPassword }) {
                            Icon(
                                if (showPassword) Icons.Outlined.VisibilityOff else Icons.Outlined.Visibility,
                                contentDescription = stringResource(
                                    if (showPassword) R.string.action_hide_password else R.string.action_show_password,
                                ),
                            )
                        }
                    },
                    shape = RoundedCornerShape(14.dp),
                    modifier = Modifier.fillMaxWidth().focusRequester(passwordFocus),
                )
                Spacer(Modifier.height(20.dp))
            }

            if (request.needsCode) {
                Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        stringResource(R.string.field_code),
                        style = MaterialTheme.typography.titleSmall,
                        modifier = Modifier.weight(1f),
                    )
                    TextButton(onClick = {
                        val clip = context.getSystemService(ClipboardManager::class.java)
                            ?.primaryClip?.getItemAt(0)?.coerceToText(context)?.toString().orEmpty()
                        val digits = clip.filter(Char::isDigit)
                        if (digits.length == profile.codeLength) code = digits
                    }) {
                        Icon(Icons.Outlined.ContentPaste, contentDescription = null)
                        Spacer(Modifier.width(6.dp))
                        Text(stringResource(R.string.action_paste))
                    }
                }
                Spacer(Modifier.height(8.dp))
                CodeField(
                    value = code,
                    onValueChange = { new ->
                        code = new
                        // Typing the last digit is the natural "go" moment.
                        if (new.length == profile.codeLength && passwordOk) onSubmit(password, new)
                    },
                    length = profile.codeLength,
                    label = stringResource(R.string.field_code),
                    onDone = { submit() },
                    modifier = Modifier.focusRequester(codeFocus),
                )
                Spacer(Modifier.height(10.dp))
                Text(
                    if (request.usesStaticChallenge) stringResource(R.string.signin_static_challenge)
                    else stringResource(R.string.signin_code_hint, profile.codeLength),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                if (!request.usesStaticChallenge) {
                    Spacer(Modifier.height(14.dp))
                    CombinedPreview(profile.codePosition, password, code, profile.codeLength, request.needsPassword)
                }
            }

            Spacer(Modifier.height(24.dp))
            Button(
                onClick = submit,
                enabled = canSubmit,
                shape = RoundedCornerShape(16.dp),
                modifier = Modifier.fillMaxWidth().height(56.dp),
            ) {
                Text(stringResource(R.string.action_connect), style = MaterialTheme.typography.labelLarge)
            }
        }
    }
}

/** Shows how the password and code are glued together, without revealing either. */
@Composable
private fun CombinedPreview(position: CodePosition, password: String, code: String, length: Int, passwordTyped: Boolean) {
    val colors = MaterialTheme.colorScheme
    val pw = if (passwordTyped) "•".repeat(password.length.coerceIn(4, 12)) else "••••••••"
    val otp = code.padEnd(length, '·')
    val text = buildAnnotatedString {
        val pwStyle = SpanStyle(color = colors.onSurfaceVariant)
        val codeStyle = SpanStyle(color = colors.primary)
        if (position == CodePosition.AFTER_PASSWORD) {
            withStyle(pwStyle) { append(pw) }
            withStyle(codeStyle) { append(otp) }
        } else {
            withStyle(codeStyle) { append(otp) }
            withStyle(pwStyle) { append(pw) }
        }
    }
    Surface(color = colors.surfaceContainerHigh, shape = RoundedCornerShape(12.dp), modifier = Modifier.fillMaxWidth()) {
        Row(
            Modifier.padding(horizontal = 14.dp, vertical = 10.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween,
        ) {
            Text(
                stringResource(R.string.signin_sent_as),
                style = MaterialTheme.typography.labelMedium,
                color = colors.onSurfaceVariant,
            )
            Spacer(Modifier.width(12.dp))
            Text(text, style = NumericStyle.merge(MaterialTheme.typography.bodyMedium), maxLines = 1)
        }
    }
}
