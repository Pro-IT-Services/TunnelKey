package app.tunnelkey.ui.components

import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.tunnelkey.ui.theme.NumericStyle

/**
 * One-time-code input rendered as a row of digit cells. It is a single text
 * field underneath, so paste, IME suggestions and TalkBack all behave normally.
 */
@Composable
fun CodeField(
    value: String,
    onValueChange: (String) -> Unit,
    length: Int,
    label: String,
    modifier: Modifier = Modifier,
    onDone: () -> Unit = {},
) {
    var focused by remember { mutableStateOf(false) }
    val colors = MaterialTheme.colorScheme

    BasicTextField(
        value = TextFieldValue(value, selection = TextRange(value.length)),
        onValueChange = { v -> onValueChange(v.text.filter(Char::isDigit).take(length)) },
        singleLine = true,
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.NumberPassword, imeAction = ImeAction.Done),
        keyboardActions = KeyboardActions(onDone = { onDone() }),
        cursorBrush = SolidColor(colors.primary),
        modifier = modifier
            .fillMaxWidth()
            .onFocusChanged { focused = it.isFocused }
            .semantics { contentDescription = label },
        decorationBox = {
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.Center) {
                for (i in 0 until length) {
                    val char = value.getOrNull(i)
                    val active = focused && (i == value.length || (i == length - 1 && value.length == length))
                    Box(
                        modifier = Modifier
                            .width(if (length > 6) 32.dp else 44.dp)
                            .height(58.dp)
                            .border(
                                width = if (active) 2.dp else 1.dp,
                                color = when {
                                    active -> colors.primary
                                    char != null -> colors.onSurfaceVariant
                                    else -> colors.outline
                                },
                                shape = RoundedCornerShape(12.dp),
                            ),
                        contentAlignment = Alignment.Center,
                    ) {
                        Text(
                            text = char?.toString() ?: "",
                            style = NumericStyle.copy(fontSize = 24.sp, color = colors.onSurface),
                        )
                    }
                    if (i != length - 1) {
                        // A wider gap in the middle groups the code like authenticator apps do.
                        Spacer(Modifier.width(if (i == length / 2 - 1) 14.dp else 6.dp))
                    }
                }
            }
        },
    )
}
