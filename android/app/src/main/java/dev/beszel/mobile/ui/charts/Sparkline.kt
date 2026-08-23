package dev.beszel.mobile.ui.charts

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.PathMeasure
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.unit.dp
import dev.beszel.mobile.ui.theme.BeszelMotion
import dev.beszel.mobile.ui.theme.rememberReducedMotion

/**
 * Axisless micro chart for card headers. Flat data renders as a calm
 * midline instead of a zero-height zigzag.
 */
@Composable
fun Sparkline(
    values: List<Float>,
    color: Color,
    modifier: Modifier = Modifier,
    lineWidth: Float = 2.4f,
    fill: Boolean = true,
    fillAlpha: Float = 0.22f,
    endDot: Boolean = true,
) {
    val reducedMotion = rememberReducedMotion()
    var target by remember { mutableFloatStateOf(0f) }
    LaunchedEffect(values) { target = 1f }
    val progress by animateFloatAsState(
        targetValue = target,
        animationSpec = tween(BeszelMotion.chartDrawMillis, easing = BeszelMotion.emphasizedDecelerate),
        label = "sparkline-draw",
    )
    val drawProgress = if (reducedMotion) 1f else progress

    Canvas(modifier) {
        if (values.size < 2) return@Canvas
        val minValue = values.min()
        val maxValue = values.max()
        val dataRange = maxValue - minValue
        val isFlat = dataRange <= 1e-6f
        val range = if (isFlat) 1f else dataRange
        val midY = size.height / 2f
        val xStep = size.width / (values.size - 1)

        fun yFor(value: Float): Float {
            val normalized = if (isFlat) 0.5f else (value - minValue) / range
            // A little vertical breathing room so peaks don't clip the stroke.
            val padded = 0.08f + normalized * 0.84f
            return size.height * (1f - padded)
        }

        val linePath = Path()
        val fillPath = Path()
        values.forEachIndexed { index, value ->
            val x = index * xStep
            val y = yFor(value)
            if (index == 0) {
                linePath.moveTo(x, y)
                fillPath.moveTo(x, size.height)
                fillPath.lineTo(x, y)
            } else {
                linePath.lineTo(x, y)
                fillPath.lineTo(x, y)
            }
        }
        fillPath.lineTo(size.width, size.height)
        fillPath.close()

        if (fill && drawProgress > 0.05f) {
            drawPath(
                fillPath,
                brush = Brush.verticalGradient(
                    colors = listOf(color.copy(alpha = fillAlpha), color.copy(alpha = 0.02f)),
                ),
            )
        }

        val measured = PathMeasure()
        measured.setPath(linePath, false)
        val partial = Path()
        measured.getSegment(0f, measured.length * drawProgress, partial, true)
        drawPath(partial, color = color, style = Stroke(width = lineWidth.dp.toPx(), cap = StrokeCap.Round))

        if (endDot && drawProgress >= 0.99f) {
            val lastX = (values.size - 1) * xStep
            val lastY = yFor(values.last())
            drawCircle(color.copy(alpha = 0.28f), radius = lineWidth.dp.toPx() * 1.1f, center = Offset(lastX, lastY))
            drawCircle(color, radius = lineWidth.dp.toPx() * 0.6f, center = Offset(lastX, lastY))
        }
    }
}
