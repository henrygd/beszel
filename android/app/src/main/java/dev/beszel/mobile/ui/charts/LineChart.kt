package dev.beszel.mobile.ui.charts

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.gestures.detectHorizontalDragGestures
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.PathMeasure
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.drawText
import androidx.compose.ui.text.rememberTextMeasurer
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import dev.beszel.mobile.ui.theme.BeszelMotion
import dev.beszel.mobile.ui.theme.dataMedium
import dev.beszel.mobile.ui.theme.dataSmall
import dev.beszel.mobile.ui.theme.rememberReducedMotion
import kotlin.math.log10
import kotlin.math.pow
import kotlin.math.roundToInt

/** One line on the chart. All series share the same x positions. */
data class ChartSeries(
    val values: List<Float>,
    val color: Color,
    val label: String,
)

/**
 * History line chart with axis labels, gradient fill, draw-in animation,
 * min/max markers, and drag scrubbing with a value tooltip. Pure Compose
 * Canvas; no chart library.
 */
@Composable
fun LineChart(
    timestamps: List<Long>,
    series: List<ChartSeries>,
    valueFormat: (Float) -> String,
    timeFormat: (Long) -> String,
    modifier: Modifier = Modifier,
    yMax: Float? = null,
    yMin: Float = 0f,
    chartDescription: String = "History chart",
) {
    require(series.isNotEmpty()) { "LineChart needs at least one series" }
    val pointCount = timestamps.size
    val primary = series.first()

    val reducedMotion = rememberReducedMotion()
    var drawTarget by remember { mutableFloatStateOf(0f) }
    LaunchedEffect(timestamps, series) { drawTarget = 1f }
    val drawProgress by animateFloatAsState(
        targetValue = drawTarget,
        animationSpec = tween(BeszelMotion.chartDrawMillis, easing = BeszelMotion.emphasizedDecelerate),
        label = "chart-draw",
    )
    val progress = if (reducedMotion) 1f else drawProgress

    var scrubFraction by remember { mutableStateOf<Float?>(null) }
    var canvasWidthPx by remember { mutableIntStateOf(0) }
    val textMeasurer = rememberTextMeasurer()

    val labelStyle = MaterialTheme.typography.dataSmall.copy(color = MaterialTheme.colorScheme.onSurfaceVariant)
    val gridColor = MaterialTheme.colorScheme.outlineVariant.copy(alpha = 0.5f)
    val scrubLineColor = MaterialTheme.colorScheme.outline
    val plotAreaColor = MaterialTheme.colorScheme.surface

    val topMax = yMax ?: niceCeil(series.maxOf { it.values.max() } * 1.12f)
    val range = (topMax - yMin).takeIf { it > 1e-6f } ?: 1f

    Box(modifier.semantics { contentDescription = chartDescription }) {
        Canvas(
            Modifier
                .fillMaxSize()
                .pointerInput(pointCount) {
                    if (pointCount < 2) return@pointerInput
                    detectTapGestures { offset ->
                        scrubFraction = (offset.x / size.width).coerceIn(0f, 1f)
                    }
                }
                .pointerInput(pointCount) {
                    if (pointCount < 2) return@pointerInput
                    detectHorizontalDragGestures { change, _ ->
                        scrubFraction = (change.position.x / size.width).coerceIn(0f, 1f)
                    }
                },
        ) {
            canvasWidthPx = size.width.toInt()
            if (pointCount < 2) return@Canvas
            val yAxisWidth = 40.dp.toPx()
            val xAxisHeight = 18.dp.toPx()
            val plotLeft = yAxisWidth
            val plotRight = size.width - 4.dp.toPx()
            val plotTop = 4.dp.toPx()
            val plotBottom = size.height - xAxisHeight
            val plotWidth = (plotRight - plotLeft).coerceAtLeast(1f)
            val plotHeight = (plotBottom - plotTop).coerceAtLeast(1f)

            fun xFor(index: Int) = plotLeft + plotWidth * index / (pointCount - 1).coerceAtLeast(1)
            fun yFor(value: Float) = plotTop + plotHeight * (1f - ((value - yMin) / range).coerceIn(0f, 1f))

            // Horizontal gridlines at 0 / 50 / 100 percent of the range,
            // kept faint so data stays the loudest element.
            listOf(0f, 0.5f, 1f).forEach { fraction ->
                val y = plotBottom - plotHeight * fraction
                drawLine(gridColor, Offset(plotLeft, y), Offset(plotRight, y), strokeWidth = 1.dp.toPx())
                val measured = textMeasurer.measure(
                    text = valueFormat(yMin + range * fraction),
                    style = labelStyle,
                    softWrap = false,
                )
                drawText(
                    measured,
                    topLeft = Offset(plotLeft - measured.size.width - 6.dp.toPx(), y - measured.size.height / 2f),
                )
            }

            // X labels: start, middle, end.
            listOf(0, pointCount / 2, pointCount - 1).forEach { index ->
                val measured = textMeasurer.measure(
                    text = timeFormat(timestamps[index]),
                    style = labelStyle,
                    softWrap = false,
                )
                val x = (xFor(index) - measured.size.width / 2f)
                    .coerceIn(plotLeft, (plotRight - measured.size.width).coerceAtLeast(plotLeft))
                drawText(measured, topLeft = Offset(x, plotBottom + 4.dp.toPx()))
            }

            // Gradient fill under the primary series.
            val fillPath = Path()
            primary.values.forEachIndexed { index, value ->
                val x = xFor(index)
                val y = yFor(value)
                if (index == 0) {
                    fillPath.moveTo(x, plotBottom)
                    fillPath.lineTo(x, y)
                } else {
                    fillPath.lineTo(x, y)
                }
            }
            fillPath.lineTo(xFor(pointCount - 1), plotBottom)
            fillPath.close()
            drawPath(
                fillPath,
                brush = Brush.verticalGradient(
                    colors = listOf(primary.color.copy(alpha = 0.26f), primary.color.copy(alpha = 0.02f)),
                    startY = plotTop,
                    endY = plotBottom,
                ),
            )

            // Lines, revealed left to right.
            series.forEach { line ->
                val linePath = Path()
                line.values.forEachIndexed { index, value ->
                    val x = xFor(index)
                    val y = yFor(value)
                    if (index == 0) linePath.moveTo(x, y) else linePath.lineTo(x, y)
                }
                val measure = PathMeasure()
                measure.setPath(linePath, false)
                val partial = Path()
                measure.getSegment(0f, measure.length * progress, partial, true)
                drawPath(partial, color = line.color, style = Stroke(2.2.dp.toPx(), cap = StrokeCap.Round))
            }

            if (progress >= 1f) {
                // Min / max markers on the primary series.
                val maxIndex = primary.values.indices.maxBy(primary.values::get)
                val minIndex = primary.values.indices.minBy(primary.values::get)
                if (maxIndex != minIndex) {
                    listOf(maxIndex, minIndex).forEach { index ->
                        val center = Offset(xFor(index), yFor(primary.values[index]))
                        drawCircle(primary.color.copy(alpha = 0.3f), radius = 5.dp.toPx(), center = center)
                        drawCircle(plotAreaColor, radius = 3.4.dp.toPx(), center = center)
                        drawCircle(primary.color, radius = 1.8.dp.toPx(), center = center)
                    }
                }

                // Scrub crosshair + point markers.
                scrubFraction?.let { fraction ->
                    val index = (fraction * (pointCount - 1)).roundToInt().coerceIn(0, pointCount - 1)
                    val x = xFor(index)
                    drawLine(scrubLineColor, Offset(x, plotTop), Offset(x, plotBottom), strokeWidth = 1.2.dp.toPx())
                    series.forEach { line ->
                        val center = Offset(x, yFor(line.values[index]))
                        drawCircle(line.color, radius = 4.4.dp.toPx(), center = center)
                        drawCircle(plotAreaColor, radius = 1.8.dp.toPx(), center = center)
                    }
                }
            }
        }

        // Tooltip above the scrubbed point, clamped inside the chart.
        scrubFraction?.let { fraction ->
            if (canvasWidthPx > 0) {
                val index = (fraction * (pointCount - 1)).roundToInt().coerceIn(0, pointCount - 1)
                var tooltipWidthPx by remember { mutableIntStateOf(0) }
                Surface(
                    shape = MaterialTheme.shapes.small,
                    color = MaterialTheme.colorScheme.surfaceContainerHighest,
                    modifier = Modifier
                        .align(Alignment.TopStart)
                        .onGloballyPositioned { tooltipWidthPx = it.size.width }
                        .offset {
                            val desired = (canvasWidthPx * fraction).roundToInt() - tooltipWidthPx / 2
                            val clamped = desired.coerceIn(0, (canvasWidthPx - tooltipWidthPx).coerceAtLeast(0))
                            IntOffset(clamped, 0)
                        },
                ) {
                    Column(Modifier.padding(horizontal = 10.dp, vertical = 8.dp)) {
                        Text(
                            timeFormat(timestamps[index]),
                            style = MaterialTheme.typography.dataSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                        series.forEach { line ->
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Box(Modifier.height(8.dp).aspectRatio(1f).background(line.color, CircleShape))
                                Spacer(Modifier.width(6.dp))
                                Text(
                                    "${line.label}  ${valueFormat(line.values[index])}",
                                    style = MaterialTheme.typography.dataMedium,
                                    color = MaterialTheme.colorScheme.onSurface,
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}

/** Round up to a pleasant axis ceiling (1 / 2 / 5 x 10^n). */
internal fun niceCeil(value: Float): Float {
    if (value <= 0f) return 1f
    val exponent = log10(value.toDouble()).toInt()
    val base = 10f.pow(exponent)
    val scaled = value / base
    val step = listOf(1f, 2f, 5f, 10f).first { it >= scaled }
    return (step * base).coerceAtLeast(1f)
}
