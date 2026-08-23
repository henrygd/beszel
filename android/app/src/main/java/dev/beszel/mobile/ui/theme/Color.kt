package dev.beszel.mobile.ui.theme

import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Immutable
import androidx.compose.ui.graphics.Color

// Beszel brand: violet -> mint gradient anchors the accent system.
val BrandViolet = Color(0xFF747BFF)
val BrandMint = Color(0xFF24EB5C)

// ---------------------------------------------------------------------------
// Dark scheme - "Mission Control": deep console blue, never flat black.
// Elevation reads through surface tint plus 1dp hairline outlines.
// ---------------------------------------------------------------------------

val DarkColors = darkColorScheme(
    primary = Color(0xFF9BA1FF),
    onPrimary = Color(0xFF141A45),
    primaryContainer = Color(0xFF2E339C),
    onPrimaryContainer = Color(0xFFDDE0FF),
    secondary = Color(0xFF5CE8B4),
    onSecondary = Color(0xFF003826),
    secondaryContainer = Color(0xFF0F5C42),
    onSecondaryContainer = Color(0xFFB8F5DC),
    tertiary = Color(0xFF7CD7FF),
    onTertiary = Color(0xFF00344D),
    tertiaryContainer = Color(0xFF0E4A66),
    onTertiaryContainer = Color(0xFFC4E9FF),
    error = Color(0xFFFFB4AB),
    onError = Color(0xFF690005),
    errorContainer = Color(0xFF93000A),
    onErrorContainer = Color(0xFFFFDAD6),
    background = Color(0xFF0A0F1A),
    onBackground = Color(0xFFE6EAF2),
    surface = Color(0xFF0E1626),
    onSurface = Color(0xFFE6EAF2),
    surfaceVariant = Color(0xFF1C2A44),
    onSurfaceVariant = Color(0xFF96A3BD),
    surfaceTint = Color(0xFF9BA1FF),
    inverseSurface = Color(0xFFE6EAF2),
    inverseOnSurface = Color(0xFF131A2A),
    inversePrimary = Color(0xFF4F46E5),
    outline = Color(0xFF3A4A68),
    outlineVariant = Color(0xFF22304B),
    scrim = Color(0xFF000000),
    surfaceBright = Color(0xFF223150),
    surfaceDim = Color(0xFF070C15),
    surfaceContainer = Color(0xFF121C2E),
    surfaceContainerLow = Color(0xFF0B1320),
    surfaceContainerLowest = Color(0xFF080D16),
    surfaceContainerHigh = Color(0xFF1A2740),
    surfaceContainerHighest = Color(0xFF223150),
)

// ---------------------------------------------------------------------------
// Light scheme - "Homelab OS": cool paper daylight with ink-navy structure.
// ---------------------------------------------------------------------------

val LightColors = lightColorScheme(
    primary = Color(0xFF4F46E5),
    onPrimary = Color(0xFFFFFFFF),
    primaryContainer = Color(0xFFE2E0FF),
    onPrimaryContainer = Color(0xFF1B1658),
    secondary = Color(0xFF0E8A62),
    onSecondary = Color(0xFFFFFFFF),
    secondaryContainer = Color(0xFFC9F5E2),
    onSecondaryContainer = Color(0xFF00291B),
    tertiary = Color(0xFF0E6EA0),
    onTertiary = Color(0xFFFFFFFF),
    tertiaryContainer = Color(0xFFCBEAFF),
    onTertiaryContainer = Color(0xFF001E31),
    error = Color(0xFFBA1A1A),
    onError = Color(0xFFFFFFFF),
    errorContainer = Color(0xFFFFDAD6),
    onErrorContainer = Color(0xFF410002),
    background = Color(0xFFF3F6FB),
    onBackground = Color(0xFF151B2B),
    surface = Color(0xFFFBFCFE),
    onSurface = Color(0xFF151B2B),
    surfaceVariant = Color(0xFFDFE5F0),
    onSurfaceVariant = Color(0xFF5B6880),
    surfaceTint = Color(0xFF4F46E5),
    inverseSurface = Color(0xFF2A3142),
    inverseOnSurface = Color(0xFFEFF0F8),
    inversePrimary = Color(0xFF9BA1FF),
    outline = Color(0xFF7383A0),
    outlineVariant = Color(0xFFC3CDDE),
    scrim = Color(0xFF000000),
    surfaceBright = Color(0xFFFBFCFE),
    surfaceDim = Color(0xFFDCE2EC),
    surfaceContainer = Color(0xFFEDF1F8),
    surfaceContainerLow = Color(0xFFF5F8FC),
    surfaceContainerLowest = Color(0xFFFFFFFF),
    surfaceContainerHigh = Color(0xFFE3E9F3),
    surfaceContainerHighest = Color(0xFFDAE2EF),
)

// ---------------------------------------------------------------------------
// Semantic metric + status palette. Fixed per theme: wallpaper dynamic color
// may restyle Material roles, but metric meaning is never left to the
// wallpaper. Hue coding: CPU amber, memory violet, disk sky, network teal,
// temperature coral, GPU fuchsia, battery green.
// ---------------------------------------------------------------------------

@Immutable
data class MetricColors(
    val cpu: Color,
    val memory: Color,
    val disk: Color,
    val network: Color,
    val networkAlt: Color,
    val temperature: Color,
    val gpu: Color,
    val battery: Color,
    val healthy: Color,
    val warning: Color,
    val critical: Color,
    val neutral: Color,
    val paused: Color,
)

val DarkMetricColors = MetricColors(
    cpu = Color(0xFFFFC24D),
    memory = Color(0xFFB8A6FF),
    disk = Color(0xFF6FC9FF),
    network = Color(0xFF4DE0C0),
    networkAlt = Color(0xFF9BA1FF),
    temperature = Color(0xFFFF8A7A),
    gpu = Color(0xFFE9A8FF),
    battery = Color(0xFF6EE7A0),
    healthy = Color(0xFF4ADE8C),
    warning = Color(0xFFFFC24D),
    critical = Color(0xFFFF7A70),
    neutral = Color(0xFF96A3BD),
    paused = Color(0xFF8A93A8),
)

val LightMetricColors = MetricColors(
    cpu = Color(0xFFA86208),
    memory = Color(0xFF6D51D8),
    disk = Color(0xFF0B72B0),
    network = Color(0xFF0C8A72),
    networkAlt = Color(0xFF4F46E5),
    temperature = Color(0xFFC43D2F),
    gpu = Color(0xFF9936BD),
    battery = Color(0xFF0F7A45),
    healthy = Color(0xFF178A50),
    warning = Color(0xFFA86208),
    critical = Color(0xFFC0322B),
    neutral = Color(0xFF5B6880),
    paused = Color(0xFF6B7590),
)
