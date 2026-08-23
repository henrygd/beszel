package dev.beszel.mobile.ui.theme

import androidx.compose.material3.Typography
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.googlefonts.Font
import androidx.compose.ui.text.googlefonts.GoogleFont
import androidx.compose.ui.unit.sp
import dev.beszel.mobile.R

private val googleFontProvider = GoogleFont.Provider(
    providerAuthority = "com.google.android.gms.fonts",
    providerPackage = "com.google.android.gms.fonts",
    certificates = R.array.com_google_android_gms_fonts_certs,
)

private val spaceGrotesk = GoogleFont("Space Grotesk")
private val jetBrainsMono = GoogleFont("JetBrains Mono")

// Downloadable fonts fall back to the platform family when unavailable
// (first launch offline, no GMS), so text always renders.
val SpaceGroteskFamily = FontFamily(
    Font(googleFont = spaceGrotesk, fontProvider = googleFontProvider, weight = FontWeight.Normal),
    Font(googleFont = spaceGrotesk, fontProvider = googleFontProvider, weight = FontWeight.Medium),
    Font(googleFont = spaceGrotesk, fontProvider = googleFontProvider, weight = FontWeight.SemiBold),
    Font(googleFont = spaceGrotesk, fontProvider = googleFontProvider, weight = FontWeight.Bold),
)

val JetBrainsMonoFamily = FontFamily(
    Font(googleFont = jetBrainsMono, fontProvider = googleFontProvider, weight = FontWeight.Normal),
    Font(googleFont = jetBrainsMono, fontProvider = googleFontProvider, weight = FontWeight.Medium),
    Font(googleFont = jetBrainsMono, fontProvider = googleFontProvider, weight = FontWeight.Bold),
)

val AppTypography = Typography(
    displaySmall = TextStyle(fontFamily = SpaceGroteskFamily, fontWeight = FontWeight.Bold, fontSize = 34.sp, lineHeight = 40.sp),
    headlineLarge = TextStyle(fontFamily = SpaceGroteskFamily, fontWeight = FontWeight.Bold, fontSize = 30.sp, lineHeight = 36.sp),
    headlineMedium = TextStyle(fontFamily = SpaceGroteskFamily, fontWeight = FontWeight.SemiBold, fontSize = 26.sp, lineHeight = 32.sp),
    headlineSmall = TextStyle(fontFamily = SpaceGroteskFamily, fontWeight = FontWeight.SemiBold, fontSize = 22.sp, lineHeight = 28.sp),
    titleLarge = TextStyle(fontFamily = SpaceGroteskFamily, fontWeight = FontWeight.SemiBold, fontSize = 20.sp, lineHeight = 26.sp),
    titleMedium = TextStyle(fontFamily = SpaceGroteskFamily, fontWeight = FontWeight.SemiBold, fontSize = 16.sp, lineHeight = 22.sp),
    titleSmall = TextStyle(fontFamily = SpaceGroteskFamily, fontWeight = FontWeight.Medium, fontSize = 14.sp, lineHeight = 20.sp),
    bodyLarge = TextStyle(fontFamily = SpaceGroteskFamily, fontWeight = FontWeight.Normal, fontSize = 16.sp, lineHeight = 24.sp),
    bodyMedium = TextStyle(fontFamily = SpaceGroteskFamily, fontWeight = FontWeight.Normal, fontSize = 14.sp, lineHeight = 20.sp),
    bodySmall = TextStyle(fontFamily = SpaceGroteskFamily, fontWeight = FontWeight.Normal, fontSize = 12.sp, lineHeight = 16.sp),
    labelLarge = TextStyle(fontFamily = SpaceGroteskFamily, fontWeight = FontWeight.Medium, fontSize = 14.sp, lineHeight = 20.sp),
    labelMedium = TextStyle(fontFamily = SpaceGroteskFamily, fontWeight = FontWeight.Medium, fontSize = 12.sp, lineHeight = 16.sp),
    labelSmall = TextStyle(fontFamily = SpaceGroteskFamily, fontWeight = FontWeight.Medium, fontSize = 11.sp, lineHeight = 14.sp, letterSpacing = 0.4.sp),
)

/**
 * Data typography. Monospaced tabular figures for every number the hub
 * reports, so live-updating values never shift layout.
 */
val Typography.metricValue: TextStyle
    get() = TextStyle(fontFamily = JetBrainsMonoFamily, fontWeight = FontWeight.Medium, fontSize = 20.sp, lineHeight = 26.sp)

val Typography.metricValueLarge: TextStyle
    get() = TextStyle(fontFamily = JetBrainsMonoFamily, fontWeight = FontWeight.Bold, fontSize = 32.sp, lineHeight = 38.sp)

val Typography.dataMedium: TextStyle
    get() = TextStyle(fontFamily = JetBrainsMonoFamily, fontWeight = FontWeight.Normal, fontSize = 13.sp, lineHeight = 18.sp)

val Typography.dataSmall: TextStyle
    get() = TextStyle(fontFamily = JetBrainsMonoFamily, fontWeight = FontWeight.Normal, fontSize = 11.sp, lineHeight = 15.sp)

val Typography.overlineMono: TextStyle
    get() = TextStyle(
        fontFamily = JetBrainsMonoFamily,
        fontWeight = FontWeight.Medium,
        fontSize = 11.sp,
        lineHeight = 14.sp,
        letterSpacing = 1.2.sp,
    )
