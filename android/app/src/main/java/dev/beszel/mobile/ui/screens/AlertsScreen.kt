package dev.beszel.mobile.ui.screens

import android.text.format.DateUtils
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.CheckCircle
import androidx.compose.material.icons.rounded.NotificationsNone
import androidx.compose.material.icons.rounded.WarningAmber
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import dev.beszel.mobile.AppUiState
import dev.beszel.mobile.R
import dev.beszel.mobile.data.AlertHistoryRecord
import dev.beszel.mobile.data.AlertRecord
import dev.beszel.mobile.data.alertPresentation
import dev.beszel.mobile.data.formatNumber
import dev.beszel.mobile.ui.components.EmptyState
import dev.beszel.mobile.ui.theme.BeszelTheme
import dev.beszel.mobile.ui.theme.dataSmall
import java.time.Instant
import java.time.LocalDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter

@Composable
fun AlertsScreen(state: AppUiState, contentPadding: PaddingValues) {
    val systemNames = remember(state.systems) { state.systems.associate { it.id to it.name } }
    val active = state.activeAlerts
    val metrics = BeszelTheme.metrics

    LazyColumn(
        modifier = Modifier.fillMaxSize().statusBarsPadding(),
        contentPadding = PaddingValues(
            start = 16.dp,
            end = 16.dp,
            top = contentPadding.calculateTopPadding() + 16.dp,
            bottom = contentPadding.calculateBottomPadding() + 24.dp,
        ),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        when {
            active.isEmpty() && state.alertHistory.isEmpty() -> {
                item {
                    EmptyState(
                        icon = Icons.Rounded.NotificationsNone,
                        title = stringResource(R.string.alerts_none_title),
                        message = stringResource(R.string.alerts_none_message),
                    )
                }
            }
            active.isEmpty() -> {
                item {
                    Surface(
                        modifier = Modifier.fillMaxWidth(),
                        shape = MaterialTheme.shapes.extraLarge,
                        color = metrics.healthy.copy(alpha = 0.10f),
                        border = BorderStroke(1.dp, metrics.healthy.copy(alpha = 0.3f)),
                    ) {
                        Row(Modifier.padding(20.dp), verticalAlignment = Alignment.CenterVertically) {
                            Icon(
                                Icons.Rounded.CheckCircle,
                                contentDescription = null,
                                tint = metrics.healthy,
                                modifier = Modifier.size(30.dp),
                            )
                            Spacer(Modifier.width(14.dp))
                            Column {
                                Text(stringResource(R.string.alerts_all_clear_title), style = MaterialTheme.typography.titleMedium)
                                Text(
                                    stringResource(R.string.alerts_all_clear_message),
                                    style = MaterialTheme.typography.bodySmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                            }
                        }
                    }
                }
            }
            else -> {
                item {
                    Column(Modifier.padding(top = 4.dp)) {
                        Text(stringResource(R.string.alerts_active_title), style = MaterialTheme.typography.titleLarge)
                        Text(
                            pluralStringResource(R.plurals.alerts_active_count, active.size, active.size),
                            style = MaterialTheme.typography.dataSmall,
                            color = metrics.critical,
                        )
                    }
                }
                items(active, key = { "active-" + it.id }) { alert ->
                    ActiveAlertCard(
                        alert = alert,
                        systemName = systemNames[alert.systemId],
                        criticalColor = metrics.critical,
                    )
                }
            }
        }

        if (state.alertHistory.isNotEmpty()) {
            item {
                Text(
                    stringResource(R.string.alerts_history_title),
                    style = MaterialTheme.typography.titleLarge,
                    modifier = Modifier.padding(top = 16.dp),
                )
            }
            items(state.alertHistory, key = { it.id }) { event ->
                HistoryTimelineRow(
                    event = event,
                    systemName = systemNames[event.systemId],
                    criticalColor = metrics.critical,
                    neutralColor = metrics.neutral,
                    healthyColor = metrics.healthy,
                )
            }
        }
    }
}

@Composable
private fun ActiveAlertCard(
    alert: AlertRecord,
    systemName: String?,
    criticalColor: Color,
) {
    val presentation = alertPresentation(alert)
    Surface(
        modifier = Modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.large,
        color = MaterialTheme.colorScheme.surfaceContainerLow,
        border = BorderStroke(1.dp, criticalColor.copy(alpha = 0.45f)),
    ) {
        Row(Modifier.padding(16.dp), verticalAlignment = Alignment.CenterVertically) {
            Surface(
                color = criticalColor.copy(alpha = 0.14f),
                contentColor = criticalColor,
                shape = CircleShape,
            ) {
                Icon(
                    Icons.Rounded.WarningAmber,
                    contentDescription = null,
                    modifier = Modifier.padding(9.dp).size(20.dp),
                )
            }
            Spacer(Modifier.width(14.dp))
            Column(Modifier.weight(1f)) {
                Text(
                    "${systemName ?: stringResource(R.string.alerts_unknown_system)} · ${presentation.title}",
                    style = MaterialTheme.typography.titleSmall,
                )
                Text(
                    presentation.description,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
private fun HistoryTimelineRow(
    event: AlertHistoryRecord,
    systemName: String?,
    criticalColor: Color,
    neutralColor: Color,
    healthyColor: Color,
) {
    val resolved = event.resolved != null
    Row(Modifier.fillMaxWidth()) {
        // Timeline rail: a dot per event with a connector to the next row.
        Column(
            Modifier.width(24.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Canvas(Modifier.size(24.dp)) {
                val center = androidx.compose.ui.geometry.Offset(size.width / 2f, size.height / 2f)
                drawCircle(
                    color = if (resolved) healthyColor else criticalColor,
                    radius = 5.dp.toPx(),
                    center = center,
                )
                if (!resolved) {
                    drawCircle(
                        color = if (resolved) healthyColor else criticalColor,
                        radius = 8.dp.toPx(),
                        center = center,
                        style = androidx.compose.ui.graphics.drawscope.Stroke(
                            width = 1.5.dp.toPx(),
                        ),
                    )
                }
                drawLine(
                    color = neutralColor.copy(alpha = 0.35f),
                    start = androidx.compose.ui.geometry.Offset(center.x, center.y + 10.dp.toPx()),
                    end = androidx.compose.ui.geometry.Offset(center.x, size.height + 26.dp.toPx()),
                    strokeWidth = 1.5.dp.toPx(),
                    cap = StrokeCap.Round,
                )
            }
        }
        Surface(
            modifier = Modifier.weight(1f),
            shape = RoundedCornerShape(topStart = 14.dp, bottomStart = 14.dp, topEnd = 4.dp, bottomEnd = 4.dp),
            color = MaterialTheme.colorScheme.surfaceContainerLow,
        ) {
            Row(
                Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 12.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Column(Modifier.weight(1f)) {
                    Text(
                        "${systemName ?: stringResource(R.string.alerts_unknown_system)} · ${event.name}",
                        style = MaterialTheme.typography.titleSmall,
                    )
                    Text(
                        stringResource(R.string.alerts_history_value, formatNumber(event.value)),
                        style = MaterialTheme.typography.dataSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Column(horizontalAlignment = Alignment.End) {
                    Text(
                        relativeTime(event.created),
                        style = MaterialTheme.typography.dataSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Text(
                        if (resolved) stringResource(R.string.alerts_resolved) else stringResource(R.string.alerts_active),
                        style = MaterialTheme.typography.labelSmall,
                        color = if (resolved) healthyColor else criticalColor,
                    )
                }
            }
        }
    }
}

private fun relativeTime(value: String): String = runCatching {
    val instant = runCatching { Instant.parse(value) }.getOrElse {
        val clean = value.substringBefore('.').replace('T', ' ')
        LocalDateTime.parse(clean, DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss"))
            .atZone(ZoneId.systemDefault()).toInstant()
    }
    DateUtils.getRelativeTimeSpanString(
        instant.toEpochMilli(),
        System.currentTimeMillis(),
        DateUtils.MINUTE_IN_MILLIS,
    ).toString()
}.getOrDefault(value)
