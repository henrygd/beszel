package dev.beszel.mobile.ui.components

import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.size
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import dev.beszel.mobile.ui.theme.rememberReducedMotion

/**
 * Status indicator with a soft glow halo. Never color-only in usage: pair
 * with a text label or icon at the call site.
 */
@Composable
fun StatusDot(
    color: Color,
    modifier: Modifier = Modifier,
    size: Dp = 10.dp,
    glow: Boolean = true,
    pulse: Boolean = false,
) {
    val reducedMotion = rememberReducedMotion()
    val haloAlpha: Float = if (pulse && !reducedMotion) {
        val transition = rememberInfiniteTransition(label = "status-pulse")
        val alpha by transition.animateFloat(
            initialValue = 0.18f,
            targetValue = 0.5f,
            animationSpec = infiniteRepeatable(tween(1100), RepeatMode.Reverse),
            label = "halo-alpha",
        )
        alpha
    } else {
        0.32f
    }
    Canvas(modifier.size(size * 2.6f)) {
        val radius = size.toPx() / 2f
        val center = Offset(this.size.width / 2f, this.size.height / 2f)
        if (glow) {
            drawCircle(
                brush = Brush.radialGradient(
                    colors = listOf(color.copy(alpha = haloAlpha), Color.Transparent),
                    center = center,
                    radius = radius * 3.4f,
                ),
                radius = radius * 3.4f,
                center = center,
            )
        }
        drawCircle(color = color, radius = radius, center = center)
    }
}
