package dev.beszel.mobile.ui.theme

import android.provider.Settings
import androidx.compose.animation.core.CubicBezierEasing
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.SpringSpec
import androidx.compose.animation.core.TweenSpec
import androidx.compose.animation.core.spring
import androidx.compose.animation.core.tween
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.platform.LocalContext

/**
 * Motion tokens. Springs carry the personality (calm, instrument-like);
 * tweens cover fade-style steps. Durations stay inside 150-300ms for
 * micro-interactions and under ~450ms for the chart draw-in.
 */
object BeszelMotion {
    val emphasizedDecelerate = CubicBezierEasing(0.05f, 0.7f, 0.1f, 1f)
    val emphasizedAccelerate = CubicBezierEasing(0.3f, 0f, 0.8f, 0.15f)

    fun <T> springStandard(): SpringSpec<T> = spring(
        dampingRatio = Spring.DampingRatioLowBouncy,
        stiffness = Spring.StiffnessMediumLow,
    )

    fun <T> springGentle(): SpringSpec<T> = spring(
        dampingRatio = Spring.DampingRatioNoBouncy,
        stiffness = 170f,
    )

    fun <T> fadeFast(): TweenSpec<T> = tween(durationMillis = 180, easing = emphasizedDecelerate)
    fun <T> fadeSlow(): TweenSpec<T> = tween(durationMillis = 300, easing = emphasizedDecelerate)
    const val chartDrawMillis = 450
    const val listStaggerMillis = 35
}

/**
 * Decorative animation gate: when the user disables animator duration at the
 * system level (reduced motion), entrances render at their final state.
 */
@Composable
fun rememberReducedMotion(): Boolean {
    val context = LocalContext.current
    return remember {
        runCatching {
            Settings.Global.getFloat(
                context.contentResolver,
                Settings.Global.ANIMATOR_DURATION_SCALE,
                1f,
            ) == 0f
        }.getOrDefault(false)
    }
}
