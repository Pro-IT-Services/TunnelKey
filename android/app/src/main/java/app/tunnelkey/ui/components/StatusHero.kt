package app.tunnelkey.ui.components

import android.provider.Settings
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.ErrorOutline
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import app.tunnelkey.R
import app.tunnelkey.ui.theme.LocalSignals
import app.tunnelkey.ui.theme.NumericStyle
import app.tunnelkey.vpn.Phase
import app.tunnelkey.vpn.TunnelStatus
import kotlinx.coroutines.delay

/**
 * The tunnel: three nested arches. Grey when idle, brass shimmer while
 * connecting, and a slow teal flow through the arches once protected.
 */
@Composable
fun StatusHero(status: TunnelStatus, profileName: String?, modifier: Modifier = Modifier) {
    val signals = LocalSignals.current
    val colors = MaterialTheme.colorScheme
    val target = when (status.phase) {
        Phase.Connected -> signals.secure
        Phase.Connecting, Phase.Reconnecting, Phase.Disconnecting -> signals.pending
        Phase.Failed -> colors.error
        Phase.Disconnected -> signals.idle
    }
    val tint by animateColorAsState(target, tween(400), label = "tint")
    val animate = status.phase != Phase.Disconnected && status.phase != Phase.Failed && !reducedMotion()

    Column(modifier.fillMaxWidth(), horizontalAlignment = Alignment.CenterHorizontally) {
        TunnelArches(tint = tint, flowing = status.phase == Phase.Connected, animate = animate)

        Spacer(Modifier.height(20.dp))
        Text(
            text = stringResource(
                when (status.phase) {
                    Phase.Disconnected -> R.string.status_disconnected
                    Phase.Connecting -> R.string.status_connecting
                    Phase.Connected -> R.string.status_connected
                    Phase.Reconnecting -> R.string.status_reconnecting
                    Phase.Disconnecting -> R.string.status_disconnecting
                    Phase.Failed -> R.string.status_failed
                },
            ),
            style = MaterialTheme.typography.headlineSmall,
            color = if (status.phase == Phase.Connected) signals.secure else colors.onSurface,
            modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
        )
        Spacer(Modifier.height(4.dp))
        Row(verticalAlignment = Alignment.CenterVertically) {
            if (profileName != null) {
                Text(profileName, style = MaterialTheme.typography.bodyMedium, color = colors.onSurfaceVariant)
            }
            val since = status.connectedAt
            if (status.phase == Phase.Connected && since != null) {
                Text("  ·  ", color = colors.onSurfaceVariant)
                SessionTimer(since)
            } else if (status.phase == Phase.Connecting && stepLabel(status.step) != null) {
                Text("  ·  ", color = colors.onSurfaceVariant)
                Text(
                    stringResource(stepLabel(status.step)!!),
                    style = MaterialTheme.typography.bodyMedium,
                    color = colors.onSurfaceVariant,
                )
            }
        }

        if (status.phase == Phase.Connected) {
            Spacer(Modifier.height(20.dp))
            Row(
                Modifier.fillMaxWidth().padding(horizontal = 8.dp),
                horizontalArrangement = Arrangement.SpaceEvenly,
            ) {
                Stat(stringResource(R.string.label_address), status.vpnAddress.ifEmpty { "—" })
                Stat(stringResource(R.string.label_down), formatBytes(status.bytesIn))
                Stat(stringResource(R.string.label_up), formatBytes(status.bytesOut))
            }
        }

        if (status.phase == Phase.Failed && status.message.isNotBlank()) {
            Spacer(Modifier.height(16.dp))
            Surface(
                color = colors.errorContainer,
                contentColor = colors.onErrorContainer,
                shape = RoundedCornerShape(14.dp),
                modifier = Modifier.fillMaxWidth(),
            ) {
                Row(Modifier.padding(14.dp), verticalAlignment = Alignment.Top) {
                    Icon(Icons.Outlined.ErrorOutline, contentDescription = null, modifier = Modifier.size(20.dp))
                    Spacer(Modifier.width(10.dp))
                    Text(status.message, style = MaterialTheme.typography.bodyMedium)
                }
            }
        }
    }
}

@Composable
private fun TunnelArches(tint: Color, flowing: Boolean, animate: Boolean) {
    val idle = LocalSignals.current.idle
    val transition = rememberInfiniteTransition(label = "tunnel")
    val flow by transition.animateFloat(
        initialValue = 0f,
        targetValue = 1f,
        animationSpec = infiniteRepeatable(tween(if (flowing) 2600 else 1200, easing = LinearEasing)),
        label = "flow",
    )
    val pulse by transition.animateFloat(
        initialValue = 0.35f,
        targetValue = 1f,
        animationSpec = infiniteRepeatable(tween(900), RepeatMode.Reverse),
        label = "pulse",
    )

    Canvas(Modifier.size(width = 220.dp, height = 170.dp)) {
        val stroke = 7.dp.toPx()
        val gap = 20.dp.toPx()
        for (i in 0 until 3) {
            val inset = i * gap + stroke / 2
            val w = size.width - inset * 2
            val archTop = inset
            val radius = w / 2
            val bottom = size.height - stroke / 2
            // Fade the inner arches so the tunnel reads as depth.
            val alpha = when {
                !animate -> 1f - i * 0.28f
                flowing -> (1f - i * 0.22f)
                else -> (((pulse + i * 0.33f) % 1f) * 0.7f + 0.3f)
            }
            val color = tint.copy(alpha = alpha)
            val effect = if (flowing && animate) {
                val dash = 14.dp.toPx()
                PathEffect.dashPathEffect(floatArrayOf(dash * 3, dash), phase = -flow * dash * 4 * (i + 1))
            } else null

            // One path per arch (legs + vault) so the semi-transparent stroke
            // doesn't overlap itself where they meet.
            val path = Path().apply {
                moveTo(inset, bottom)
                lineTo(inset, archTop + radius)
                arcTo(Rect(Offset(inset, archTop), Size(w, w)), 180f, 180f, false)
                lineTo(size.width - inset, bottom)
            }
            drawPath(path, color, style = Stroke(stroke, cap = StrokeCap.Round, join = StrokeJoin.Round, pathEffect = effect))
        }
        // Ground line
        drawLine(idle.copy(alpha = 0.5f), Offset(0f, size.height), Offset(size.width, size.height), 1.dp.toPx())
    }
}

@Composable
private fun Stat(label: String, value: String) {
    Column(horizontalAlignment = Alignment.CenterHorizontally) {
        Text(
            label.uppercase(),
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Spacer(Modifier.height(4.dp))
        Text(value, style = NumericStyle.merge(MaterialTheme.typography.bodyMedium), textAlign = TextAlign.Center)
    }
}

@Composable
private fun SessionTimer(since: Long) {
    val now by produceState(System.currentTimeMillis(), since) {
        while (true) {
            value = System.currentTimeMillis()
            delay(1_000)
        }
    }
    val s = ((now - since) / 1000).coerceAtLeast(0)
    Text(
        "%02d:%02d:%02d".format(s / 3600, (s / 60) % 60, s % 60),
        style = NumericStyle.merge(MaterialTheme.typography.bodyMedium),
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

@Composable
private fun reducedMotion(): Boolean {
    val context = LocalContext.current
    return remember {
        Settings.Global.getFloat(context.contentResolver, Settings.Global.ANIMATOR_DURATION_SCALE, 1f) == 0f
    }
}

fun formatBytes(bytes: Long): String {
    if (bytes < 1024) return "$bytes B"
    val units = arrayOf("KB", "MB", "GB", "TB")
    var value = bytes / 1024.0
    var unit = 0
    while (value >= 1024 && unit < units.lastIndex) {
        value /= 1024
        unit++
    }
    return if (value >= 100) "%.0f %s".format(value, units[unit]) else "%.1f %s".format(value, units[unit])
}

/** Human-readable label for the OpenVPN core's connection steps; null hides the step. */
private fun stepLabel(step: String): Int? = when (step) {
    "RESOLVE" -> R.string.step_resolve
    "WAIT", "CONNECTING" -> R.string.step_connecting
    "AUTH_PENDING" -> R.string.step_auth
    "GET_CONFIG" -> R.string.step_get_config
    "ASSIGN_IP" -> R.string.step_assign_ip
    "ADD_ROUTES" -> R.string.step_add_routes
    else -> null
}
