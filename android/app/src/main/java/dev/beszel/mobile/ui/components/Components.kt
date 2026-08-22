package dev.beszel.mobile.ui.components

import androidx.compose.animation.animateContentSize
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.spring
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.KeyboardArrowRight
import androidx.compose.material.icons.rounded.Dns
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import dev.beszel.mobile.data.StatPoint
import dev.beszel.mobile.data.SystemRecord
import dev.beszel.mobile.data.formatUptime
import kotlin.math.max

@Composable
fun BeszelMark(modifier: Modifier = Modifier, compact: Boolean = false) {
    Box(
        modifier = modifier
            .size(if (compact) 38.dp else 64.dp)
            .background(
                brush = Brush.linearGradient(listOf(Color(0xFF747BFF), Color(0xFF24EB5C))),
                shape = RoundedCornerShape(if (compact) 12.dp else 20.dp),
            ),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            text = "B",
            color = Color.White,
            fontSize = if (compact) 22.sp else 38.sp,
            fontWeight = FontWeight.Black,
        )
    }
}

@Composable
fun StatusBadge(status: String, modifier: Modifier = Modifier) {
    val (label, color) = when (status) {
        "up" -> "Online" to MaterialTheme.colorScheme.secondary
        "down" -> "Offline" to MaterialTheme.colorScheme.error
        "paused" -> "Paused" to MaterialTheme.colorScheme.tertiary
        else -> "Pending" to MaterialTheme.colorScheme.onSurfaceVariant
    }
    Surface(
        modifier = modifier,
        color = color.copy(alpha = 0.12f),
        contentColor = color,
        shape = CircleShape,
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 6.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            Box(Modifier.size(7.dp).background(color, CircleShape))
            Text(label, style = MaterialTheme.typography.labelMedium, fontWeight = FontWeight.SemiBold)
        }
    }
}

@Composable
fun MetricRing(
    value: Float,
    label: String,
    modifier: Modifier = Modifier,
    color: Color = MaterialTheme.colorScheme.primary,
) {
    val safeValue = value.coerceIn(0f, 100f)
    val progress by animateFloatAsState(safeValue / 100f, spring(dampingRatio = 0.78f), label = "metric")
    Column(modifier, horizontalAlignment = Alignment.CenterHorizontally) {
        Box(Modifier.size(58.dp), contentAlignment = Alignment.Center) {
            Canvas(Modifier.fillMaxSize().semantics { contentDescription = "$label ${safeValue.toInt()} percent" }) {
                drawArc(
                    color = color.copy(alpha = 0.14f),
                    startAngle = -90f,
                    sweepAngle = 360f,
                    useCenter = false,
                    style = Stroke(6.dp.toPx(), cap = StrokeCap.Round),
                )
                drawArc(
                    color = color,
                    startAngle = -90f,
                    sweepAngle = progress * 360f,
                    useCenter = false,
                    style = Stroke(6.dp.toPx(), cap = StrokeCap.Round),
                )
            }
            Text("${safeValue.toInt()}%", style = MaterialTheme.typography.labelLarge, fontWeight = FontWeight.Bold)
        }
        Spacer(Modifier.height(6.dp))
        Text(label, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
fun SystemCard(system: SystemRecord, onClick: () -> Unit, modifier: Modifier = Modifier) {
    Surface(
        modifier = modifier.fillMaxWidth().animateContentSize(),
        onClick = onClick,
        shape = MaterialTheme.shapes.large,
        color = MaterialTheme.colorScheme.surfaceContainerLow,
        tonalElevation = 1.dp,
    ) {
        Column(Modifier.padding(20.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Surface(
                    modifier = Modifier.size(46.dp),
                    color = MaterialTheme.colorScheme.primaryContainer,
                    contentColor = MaterialTheme.colorScheme.onPrimaryContainer,
                    shape = RoundedCornerShape(15.dp),
                ) {
                    Box(contentAlignment = Alignment.Center) {
                        Icon(Icons.Rounded.Dns, contentDescription = null)
                    }
                }
                Spacer(Modifier.width(12.dp))
                Column(Modifier.weight(1f)) {
                    Text(
                        system.name,
                        style = MaterialTheme.typography.titleMedium,
                        fontWeight = FontWeight.Bold,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                    Text(
                        system.info.hostname.ifBlank { system.host.substringAfterLast('/') },
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
                StatusBadge(system.status)
            }

            Spacer(Modifier.height(22.dp))
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceAround) {
                MetricRing(system.info.cpu, "CPU")
                MetricRing(system.info.memoryPercent, "Memory", color = MaterialTheme.colorScheme.secondary)
                MetricRing(system.info.diskPercent, "Disk", color = MaterialTheme.colorScheme.tertiary)
            }
            Spacer(Modifier.height(18.dp))
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    if (system.info.uptimeSeconds > 0) "Up ${formatUptime(system.info.uptimeSeconds)}" else "Waiting for metrics",
                    modifier = Modifier.weight(1f),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Text("Details", style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.primary)
                Icon(
                    Icons.AutoMirrored.Rounded.KeyboardArrowRight,
                    contentDescription = null,
                    tint = MaterialTheme.colorScheme.primary,
                    modifier = Modifier.size(20.dp),
                )
            }
        }
    }
}

@Composable
fun SummaryMetric(
    value: String,
    label: String,
    modifier: Modifier = Modifier,
    icon: ImageVector? = null,
    color: Color = MaterialTheme.colorScheme.primary,
) {
    Column(modifier) {
        if (icon != null) {
            Icon(icon, contentDescription = null, tint = color, modifier = Modifier.size(22.dp))
            Spacer(Modifier.height(12.dp))
        }
        Text(value, style = MaterialTheme.typography.headlineMedium, fontWeight = FontWeight.Bold)
        Text(label, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
fun MetricTile(
    title: String,
    value: String,
    icon: ImageVector,
    modifier: Modifier = Modifier,
    supporting: String? = null,
    tint: Color = MaterialTheme.colorScheme.primary,
) {
    Surface(
        modifier = modifier,
        shape = MaterialTheme.shapes.medium,
        color = MaterialTheme.colorScheme.surfaceContainerLow,
    ) {
        Column(Modifier.padding(18.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Surface(
                    color = tint.copy(alpha = 0.12f),
                    contentColor = tint,
                    shape = RoundedCornerShape(12.dp),
                ) {
                    Icon(icon, contentDescription = null, modifier = Modifier.padding(9.dp).size(21.dp))
                }
                Spacer(Modifier.weight(1f))
                Text(value, style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold)
            }
            Spacer(Modifier.height(14.dp))
            Text(title, style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.SemiBold)
            if (!supporting.isNullOrBlank()) {
                Text(
                    supporting,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
    }
}

enum class ChartMetric(val label: String) { CPU("CPU"), MEMORY("Memory"), DISK("Disk") }

@Composable
fun MetricChart(
    points: List<StatPoint>,
    metric: ChartMetric,
    modifier: Modifier = Modifier,
    lineColor: Color = MaterialTheme.colorScheme.primary,
) {
    if (points.size < 2) {
        Box(modifier, contentAlignment = Alignment.Center) {
            Text(
                "Not enough historical data yet",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        return
    }
    val values = points.map {
        when (metric) {
            ChartMetric.CPU -> it.cpu
            ChartMetric.MEMORY -> it.memory
            ChartMetric.DISK -> it.disk
        }
    }
    val gridColor = MaterialTheme.colorScheme.outlineVariant.copy(alpha = 0.55f)
    val surfaceColor = MaterialTheme.colorScheme.surface
    Canvas(modifier.semantics { contentDescription = "${metric.label} history chart" }) {
        val chartHeight = size.height - 28.dp.toPx()
        val chartWidth = size.width
        val top = 8.dp.toPx()
        val minValue = 0f
        val maxValue = max(100f, values.maxOrNull() ?: 100f)
        val xStep = chartWidth / (values.size - 1).coerceAtLeast(1)

        repeat(4) { index ->
            val y = top + chartHeight * index / 3f
            drawLine(
                color = gridColor,
                start = Offset(0f, y),
                end = Offset(chartWidth, y),
                strokeWidth = 1.dp.toPx(),
            )
        }

        val linePath = Path()
        val fillPath = Path()
        values.forEachIndexed { index, value ->
            val x = index * xStep
            val y = top + chartHeight * (1f - ((value - minValue) / (maxValue - minValue)).coerceIn(0f, 1f))
            if (index == 0) {
                linePath.moveTo(x, y)
                fillPath.moveTo(x, chartHeight + top)
                fillPath.lineTo(x, y)
            } else {
                linePath.lineTo(x, y)
                fillPath.lineTo(x, y)
            }
        }
        fillPath.lineTo(chartWidth, chartHeight + top)
        fillPath.close()
        drawPath(
            fillPath,
            brush = Brush.verticalGradient(
                colors = listOf(lineColor.copy(alpha = 0.28f), lineColor.copy(alpha = 0.01f)),
                startY = top,
                endY = chartHeight + top,
            ),
        )
        drawPath(linePath, color = lineColor, style = Stroke(width = 3.dp.toPx(), cap = StrokeCap.Round))
        val lastY = top + chartHeight * (1f - (values.last() / maxValue).coerceIn(0f, 1f))
        drawCircle(lineColor, radius = 5.dp.toPx(), center = Offset(chartWidth, lastY))
        drawCircle(surfaceColor, radius = 2.dp.toPx(), center = Offset(chartWidth, lastY))
    }
}

@Composable
fun LoadingState(modifier: Modifier = Modifier, label: String = "Loading your systems") {
    Column(modifier.fillMaxSize(), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.Center) {
        CircularProgressIndicator(strokeWidth = 3.dp)
        Spacer(Modifier.height(18.dp))
        Text(label, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}
