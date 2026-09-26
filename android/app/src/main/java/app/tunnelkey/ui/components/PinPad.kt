package app.tunnelkey.ui.components

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.spring
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.Backspace
import androidx.compose.material.icons.outlined.Fingerprint
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.tunnelkey.R
import app.tunnelkey.ui.theme.NumericStyle

/** Eight dots showing how many digits were typed. Shakes when [errorKey] changes. */
@Composable
fun PinDots(length: Int, filled: Int, errorKey: Int, error: Boolean) {
    val shake = remember { Animatable(0f) }
    LaunchedEffect(errorKey) {
        if (errorKey == 0) return@LaunchedEffect
        for (x in listOf(-14f, 12f, -9f, 6f, -3f, 0f)) shake.animateTo(x, spring(stiffness = Spring.StiffnessHigh))
    }
    val colors = MaterialTheme.colorScheme
    val description = stringResource(R.string.pin_dots_description, filled, length)
    Row(
        Modifier
            .offset { IntOffset(shake.value.dp.roundToPx(), 0) }
            .semantics { contentDescription = description },
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        repeat(length) { i ->
            val on = i < filled
            Box(
                Modifier
                    .size(14.dp)
                    .then(
                        if (on) Modifier.background(if (error) colors.error else colors.primary, CircleShape)
                        else Modifier.border(1.5.dp, if (error) colors.error else colors.outline, CircleShape),
                    ),
            )
            if (i == 3) Spacer(Modifier.width(6.dp))
        }
    }
}

/** Large numeric keypad for the thumb zone. */
@Composable
fun PinPad(
    onDigit: (Char) -> Unit,
    onDelete: () -> Unit,
    onBiometric: (() -> Unit)? = null,
    enabled: Boolean = true,
) {
    val haptics = LocalHapticFeedback.current
    val rows = listOf("123", "456", "789")
    Column(verticalArrangement = Arrangement.spacedBy(14.dp), horizontalAlignment = Alignment.CenterHorizontally) {
        for (row in rows) {
            Row(horizontalArrangement = Arrangement.spacedBy(22.dp)) {
                for (c in row) Key(enabled, onClick = { haptics.performHapticFeedback(HapticFeedbackType.TextHandleMove); onDigit(c) }) {
                    Text(c.toString(), style = NumericStyle.copy(fontSize = 28.sp))
                }
            }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(22.dp)) {
            if (onBiometric != null) {
                Key(enabled, onClick = onBiometric, plain = true, label = stringResource(R.string.lock_use_biometric)) {
                    Icon(Icons.Outlined.Fingerprint, contentDescription = null, modifier = Modifier.size(30.dp))
                }
            } else {
                Spacer(Modifier.size(KEY))
            }
            Key(enabled, onClick = { haptics.performHapticFeedback(HapticFeedbackType.TextHandleMove); onDigit('0') }) {
                Text("0", style = NumericStyle.copy(fontSize = 28.sp))
            }
            Key(enabled, onClick = onDelete, plain = true, label = stringResource(R.string.action_delete_digit)) {
                Icon(Icons.AutoMirrored.Outlined.Backspace, contentDescription = null)
            }
        }
    }
}

private val KEY = 76.dp

@Composable
private fun Key(
    enabled: Boolean,
    onClick: () -> Unit,
    plain: Boolean = false,
    label: String? = null,
    content: @Composable () -> Unit,
) {
    Surface(
        onClick = onClick,
        enabled = enabled,
        shape = CircleShape,
        color = if (plain) MaterialTheme.colorScheme.background else MaterialTheme.colorScheme.surfaceContainerHigh,
        modifier = Modifier
            .size(KEY)
            .then(if (label != null) Modifier.semantics { contentDescription = label } else Modifier),
    ) {
        Box(contentAlignment = Alignment.Center) { content() }
    }
}

/** Keeps a PIN entry at 8 digits. */
fun String.appendPinDigit(c: Char, max: Int = 8) = if (length < max) this + c else this
