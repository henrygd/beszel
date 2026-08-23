package dev.beszel.mobile.ui.components

import android.text.format.DateUtils
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import dev.beszel.mobile.ui.charts.Sparkline
import dev.beszel.mobile.ui.theme.BeszelTheme
import dev.beszel.mobile.ui.theme.dataMedium
import dev.beszel.mobile.ui.theme.dataSmall
import dev.beszel.mobile.ui.theme.overlineMono

enum class FleetStatus(val label: String) {
    NOMINAL("Nominal"),
    DEGRADED("Degraded"),
    CRITICAL("Critical"),
    STANDBY("Standby"),
}

fun resolveFleetStatus(onlineCount: Int, downCount: Int, alertCount: Int, pendingCount: Int): FleetStatus = when {
    downCount > 0 -> FleetStatus.CRITICAL
    alertCount > 0 -> FleetStatus.DEGRADED
    pendingCount > 0 -> FleetStatus.STANDBY
    else -> FleetStatus.NOMINAL
}

@Composable
fun fleetStatusColor(status: FleetStatus): Color {
    val metrics = BeszelTheme.metrics
    return when (status) {
        FleetStatus.NOMINAL -> metrics.healthy
        FleetStatus.DEGRADED -> metrics.warning
        FleetStatus.CRITICAL -> metrics.critical
        FleetStatus.STANDBY -> metrics.neutral
    }
}

/**
 * The fleet's instrument strip: live status word, a pulse dot, and the
 * fleet-wide CPU sparkline sampled once per poll.
 */
@Composable
fun FleetPulseHeader(
    pulse: List<Float>,
    onlineCount: Int,
    downCount: Int,
    pendingCount: Int,
    pausedCount: Int,
    alertCount: Int,
    lastUpdated: Long?,
    modifier: Modifier = Modifier,
) {
    val status = resolveFleetStatus(onlineCount, downCount, alertCount, pendingCount)
    val statusColor = fleetStatusColor(status)
    val hasPulse = pulse.size >= 2

    Surface(
        modifier = modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.extraLarge,
        color = MaterialTheme.colorScheme.surfaceContainerLow,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
    ) {
        Column(Modifier.padding(20.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text(
                        "FLEET STATUS",
                        style = MaterialTheme.typography.overlineMono,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Spacer(Modifier.height(4.dp))
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        StatusDot(color = statusColor, pulse = true)
                        Spacer(Modifier.width(10.dp))
                        Text(
                            status.label,
                            style = MaterialTheme.typography.titleLarge,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                        )
                    }
                    Spacer(Modifier.height(6.dp))
                    Text(
                        buildString {
                            append("$onlineCount online")
                            if (downCount > 0) append(" · $downCount down")
                            if (pendingCount > 0) append(" · $pendingCount pending")
                            if (pausedCount > 0) append(" · $pausedCount paused")
                            if (alertCount > 0) append(" · $alertCount alerting")
                        },
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                if (hasPulse) {
                    Column(horizontalAlignment = Alignment.CenterHorizontally) {
                        Text(
                            "avg ${pulse.last().toInt()}%",
                            style = MaterialTheme.typography.dataMedium,
                            color = MaterialTheme.colorScheme.onSurface,
                        )
                        Sparkline(
                            values = pulse,
                            color = statusColor,
                            modifier = Modifier
                                .padding(top = 4.dp)
                                .width(108.dp)
                                .height(40.dp),
                        )
                        Text(
                            "CPU",
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }

            if (lastUpdated != null) {
                Spacer(Modifier.height(14.dp))
                HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant.copy(alpha = 0.6f))
                Spacer(Modifier.height(10.dp))
                Text(
                    text = "updated " + DateUtils.getRelativeTimeSpanString(
                        lastUpdated,
                        System.currentTimeMillis(),
                        DateUtils.SECOND_IN_MILLIS,
                    ).toString(),
                    style = MaterialTheme.typography.dataSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}
