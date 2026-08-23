package dev.beszel.mobile.ui.components

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.snap
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.ChevronRight
import androidx.compose.material.icons.rounded.NotificationAdd
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import dev.beszel.mobile.data.SystemRecord
import dev.beszel.mobile.data.formatUptime
import dev.beszel.mobile.ui.theme.BeszelMotion
import dev.beszel.mobile.ui.theme.BeszelTheme
import dev.beszel.mobile.ui.theme.dataMedium
import dev.beszel.mobile.ui.theme.dataSmall
import dev.beszel.mobile.ui.theme.rememberReducedMotion

/** Status -> semantic color. Meaning is fixed; dynamic color never touches it. */
@Composable
fun statusColor(status: String): Color {
    val metrics = BeszelTheme.metrics
    return when (status) {
        "up" -> metrics.healthy
        "down" -> metrics.critical
        "paused" -> metrics.paused
        else -> metrics.neutral
    }
}

fun statusLabel(status: String): String = when (status) {
    "up" -> "Online"
    "down" -> "Offline"
    "paused" -> "Paused"
    else -> "Pending"
}

/**
 * Console-style system card: glowing status dot, hue-coded CPU / memory /
 * disk meters with monospaced values, uptime, and an alert chip.
 */
@Composable
fun SystemCard(
    system: SystemRecord,
    alertCount: Int,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val metrics = BeszelTheme.metrics
    val reducedMotion = rememberReducedMotion()
    val color = statusColor(system.status)
    val accessibilityLabel = "${system.name}, ${statusLabel(system.status)}"

    Surface(
        modifier = modifier
            .fillMaxWidth()
            .semantics { contentDescription = accessibilityLabel },
        onClick = onClick,
        shape = MaterialTheme.shapes.large,
        color = MaterialTheme.colorScheme.surfaceContainerLow,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
    ) {
        Column(Modifier.padding(18.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                StatusDot(color = color, pulse = system.isUp)
                Spacer(Modifier.width(10.dp))
                Column(Modifier.weight(1f)) {
                    Text(
                        system.name,
                        style = MaterialTheme.typography.titleMedium,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                    Text(
                        system.info.hostname.ifBlank { system.host.substringAfterLast('/') },
                        style = MaterialTheme.typography.dataSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
                if (alertCount > 0) {
                    AlertChip(count = alertCount, color = metrics.critical, icon = Icons.Rounded.NotificationAdd)
                }
            }

            Row(horizontalArrangement = Arrangement.spacedBy(16.dp)) {
                MetricMeter(
                    label = "CPU",
                    fraction = system.info.cpu / 100f,
                    value = "${system.info.cpu.toInt()}%",
                    color = metrics.cpu,
                    modifier = Modifier.weight(1f),
                    animate = !reducedMotion,
                )
                MetricMeter(
                    label = "MEM",
                    fraction = system.info.memoryPercent / 100f,
                    value = "${system.info.memoryPercent.toInt()}%",
                    color = metrics.memory,
                    modifier = Modifier.weight(1f),
                    animate = !reducedMotion,
                )
                MetricMeter(
                    label = "DSK",
                    fraction = system.info.diskPercent / 100f,
                    value = "${system.info.diskPercent.toInt()}%",
                    color = metrics.disk,
                    modifier = Modifier.weight(1f),
                    animate = !reducedMotion,
                )
            }

            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    text = if (system.info.uptimeSeconds > 0) "up ${formatUptime(system.info.uptimeSeconds)}" else "waiting for metrics",
                    style = MaterialTheme.typography.dataSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.weight(1f),
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                Icon(
                    Icons.Rounded.ChevronRight,
                    contentDescription = null,
                    tint = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.size(18.dp),
                )
            }
        }
    }
}

@Composable
private fun MetricMeter(
    label: String,
    fraction: Float,
    value: String,
    color: Color,
    modifier: Modifier = Modifier,
    animate: Boolean = true,
) {
    val barFraction by animateFloatAsState(
        targetValue = fraction.coerceIn(0f, 1f),
        animationSpec = if (animate) BeszelMotion.springGentle() else snap(),
        label = "meter-$label",
    )
    Column(modifier, verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Text(
            label,
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Text(
            value,
            style = MaterialTheme.typography.dataMedium,
            color = MaterialTheme.colorScheme.onSurface,
        )
        Box(Modifier.fillMaxWidth().height(4.dp)) {
            Spacer(
                Modifier
                    .fillMaxWidth()
                    .height(4.dp)
                    .background(color.copy(alpha = 0.14f), CircleShape),
            )
            Spacer(
                Modifier
                    .fillMaxWidth(barFraction)
                    .height(4.dp)
                    .background(color, CircleShape),
            )
        }
    }
}

@Composable
private fun AlertChip(count: Int, color: Color, icon: ImageVector) {
    Surface(
        shape = CircleShape,
        color = color.copy(alpha = 0.14f),
        contentColor = color,
    ) {
        Row(
            Modifier.padding(horizontal = 10.dp, vertical = 5.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Icon(icon, contentDescription = null, modifier = Modifier.size(13.dp))
            Text("$count", style = MaterialTheme.typography.labelMedium)
        }
    }
}
