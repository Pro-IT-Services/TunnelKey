package app.tunnelkey.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.ColorScheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp

// "Ink & brass": a quiet graphite canvas, brass for the key action, and a
// single teal signal reserved for "you are protected".

private val Ink = Color(0xFF0B0F14)
private val InkRaised = Color(0xFF131A22)
private val InkHigh = Color(0xFF1B2430)
private val InkLine = Color(0xFF2A3542)
private val Paper = Color(0xFFF5F3EE)
private val PaperRaised = Color(0xFFFFFFFF)
private val PaperHigh = Color(0xFFECE8E0)
private val PaperLine = Color(0xFFD8D3C8)

private val DarkColors = darkColorScheme(
    primary = Color(0xFFE3A93B),
    onPrimary = Color(0xFF1E1404),
    primaryContainer = Color(0xFF3A2C10),
    onPrimaryContainer = Color(0xFFFFDFA6),
    secondary = Color(0xFF93A1B0),
    onSecondary = Ink,
    background = Ink,
    onBackground = Color(0xFFE6EDF3),
    surface = Ink,
    onSurface = Color(0xFFE6EDF3),
    surfaceVariant = InkHigh,
    onSurfaceVariant = Color(0xFF93A1B0),
    surfaceContainerLowest = Ink,
    surfaceContainerLow = InkRaised,
    surfaceContainer = InkRaised,
    surfaceContainerHigh = InkHigh,
    surfaceContainerHighest = Color(0xFF222C39),
    outline = InkLine,
    outlineVariant = Color(0xFF202A36),
    error = Color(0xFFFF7A6B),
    onError = Color(0xFF3B0A05),
    errorContainer = Color(0xFF4A1510),
    onErrorContainer = Color(0xFFFFD9D3),
)

private val LightColors = lightColorScheme(
    primary = Color(0xFF8A5A00),
    onPrimary = Color.White,
    primaryContainer = Color(0xFFFBE3B3),
    onPrimaryContainer = Color(0xFF2C1C00),
    secondary = Color(0xFF55606B),
    onSecondary = Color.White,
    background = Paper,
    onBackground = Color(0xFF161A1F),
    surface = Paper,
    onSurface = Color(0xFF161A1F),
    surfaceVariant = PaperHigh,
    onSurfaceVariant = Color(0xFF55606B),
    surfaceContainerLowest = PaperRaised,
    surfaceContainerLow = PaperRaised,
    surfaceContainer = PaperRaised,
    surfaceContainerHigh = PaperHigh,
    surfaceContainerHighest = Color(0xFFE4DFD5),
    outline = PaperLine,
    outlineVariant = Color(0xFFE6E1D7),
    error = Color(0xFFB3261E),
    onError = Color.White,
    errorContainer = Color(0xFFF9DEDC),
    onErrorContainer = Color(0xFF410E0B),
)

/** Status colours that Material's scheme has no slot for. */
@Immutable
data class SignalColors(
    val secure: Color,
    val secureContainer: Color,
    val pending: Color,
    val idle: Color,
)

private val DarkSignals = SignalColors(
    secure = Color(0xFF3CCB9B),
    secureContainer = Color(0xFF0F2E25),
    pending = Color(0xFFE3A93B),
    idle = Color(0xFF3A4654),
)

private val LightSignals = SignalColors(
    secure = Color(0xFF0E7C5A),
    secureContainer = Color(0xFFD6F2E7),
    pending = Color(0xFF8A5A00),
    idle = Color(0xFFBDB6A8),
)

val LocalSignals = staticCompositionLocalOf { DarkSignals }

val MonoFamily = FontFamily.Monospace

private fun typography(): Typography {
    val base = Typography()
    return base.copy(
        displaySmall = base.displaySmall.copy(fontWeight = FontWeight.SemiBold, letterSpacing = (-0.5).sp),
        headlineSmall = base.headlineSmall.copy(fontWeight = FontWeight.SemiBold, letterSpacing = (-0.25).sp),
        titleLarge = base.titleLarge.copy(fontWeight = FontWeight.SemiBold),
        titleMedium = base.titleMedium.copy(fontWeight = FontWeight.SemiBold),
        labelLarge = base.labelLarge.copy(fontWeight = FontWeight.SemiBold, letterSpacing = 0.2.sp),
        labelSmall = base.labelSmall.copy(fontWeight = FontWeight.SemiBold, letterSpacing = 1.2.sp),
    )
}

/** Tabular figures for timers, byte counters and codes. */
val NumericStyle = TextStyle(fontFamily = MonoFamily, fontWeight = FontWeight.Medium)

@Composable
fun TunnelkeyTheme(dark: Boolean = isSystemInDarkTheme(), content: @Composable () -> Unit) {
    val colors: ColorScheme = if (dark) DarkColors else LightColors
    CompositionLocalProvider(LocalSignals provides if (dark) DarkSignals else LightSignals) {
        MaterialTheme(colorScheme = colors, typography = typography(), content = content)
    }
}
