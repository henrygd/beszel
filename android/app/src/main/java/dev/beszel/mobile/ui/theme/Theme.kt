package dev.beszel.mobile.ui.theme

import android.os.Build
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.dynamicDarkColorScheme
import androidx.compose.material3.dynamicLightColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.Typography
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.ui.unit.dp
import dev.beszel.mobile.data.ThemeMode

private val LightColors = lightColorScheme(
    primary = Color(0xFF006B62),
    onPrimary = Color.White,
    primaryContainer = Color(0xFF9CF2E5),
    onPrimaryContainer = Color(0xFF00201C),
    secondary = Color(0xFF4B635F),
    onSecondary = Color.White,
    secondaryContainer = Color(0xFFCDE8E2),
    onSecondaryContainer = Color(0xFF07201C),
    tertiary = Color(0xFF7A5900),
    tertiaryContainer = Color(0xFFFFDEA0),
    surface = Color(0xFFF5FAF8),
    surfaceContainer = Color(0xFFE7EFEC),
    surfaceContainerLow = Color(0xFFEEF5F2),
    surfaceContainerHigh = Color(0xFFDDE7E3),
)

private val DarkColors = darkColorScheme(
    primary = Color(0xFF80D5C9),
    onPrimary = Color(0xFF003731),
    primaryContainer = Color(0xFF005048),
    onPrimaryContainer = Color(0xFF9CF2E5),
    secondary = Color(0xFFB1CCC6),
    onSecondary = Color(0xFF1D3531),
    secondaryContainer = Color(0xFF344B47),
    onSecondaryContainer = Color(0xFFCDE8E2),
    tertiary = Color(0xFFFFC84C),
    tertiaryContainer = Color(0xFF5C4300),
    surface = Color(0xFF07131F),
    surfaceContainer = Color(0xFF10222D),
    surfaceContainerLow = Color(0xFF0C1B25),
    surfaceContainerHigh = Color(0xFF1A2B36),
)

private val AppShapes = Shapes(
    extraSmall = RoundedCornerShape(8.dp),
    small = RoundedCornerShape(12.dp),
    medium = RoundedCornerShape(18.dp),
    large = RoundedCornerShape(24.dp),
    extraLarge = RoundedCornerShape(32.dp),
)

@Composable
fun BeszelTheme(
    themeMode: ThemeMode,
    dynamicColor: Boolean,
    content: @Composable () -> Unit,
) {
    val dark = when (themeMode) {
        ThemeMode.SYSTEM -> isSystemInDarkTheme()
        ThemeMode.LIGHT -> false
        ThemeMode.DARK -> true
    }
    val context = LocalContext.current
    val colors = when {
        dynamicColor && Build.VERSION.SDK_INT >= Build.VERSION_CODES.S && dark -> dynamicDarkColorScheme(context)
        dynamicColor && Build.VERSION.SDK_INT >= Build.VERSION_CODES.S -> dynamicLightColorScheme(context)
        dark -> DarkColors
        else -> LightColors
    }
    MaterialTheme(
        colorScheme = colors,
        typography = Typography(),
        shapes = AppShapes,
        content = content,
    )
}
