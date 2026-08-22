package dev.beszel.mobile.ui.screens

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
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.material.icons.rounded.Bolt
import androidx.compose.material.icons.rounded.Computer
import androidx.compose.material.icons.rounded.DeveloperBoard
import androidx.compose.material.icons.rounded.Memory
import androidx.compose.material.icons.rounded.NetworkCheck
import androidx.compose.material.icons.rounded.Refresh
import androidx.compose.material.icons.rounded.Storage
import androidx.compose.material.icons.rounded.Thermostat
import androidx.compose.material.icons.rounded.Timer
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import dev.beszel.mobile.AppUiState
import dev.beszel.mobile.data.ChartRange
import dev.beszel.mobile.data.SystemRecord
import dev.beszel.mobile.data.formatBytesPerSecond
import dev.beszel.mobile.data.formatUptime
import dev.beszel.mobile.ui.components.ChartMetric
import dev.beszel.mobile.ui.components.MetricChart
import dev.beszel.mobile.ui.components.MetricTile
import dev.beszel.mobile.ui.components.StatusBadge

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SystemDetailScreen(
    system: SystemRecord,
    state: AppUiState,
    onBack: () -> Unit,
    onRange: (ChartRange) -> Unit,
    onRefresh: () -> Unit,
) {
    var metric by rememberSaveable { mutableStateOf(ChartMetric.CPU) }
    val latest = state.stats.lastOrNull()
    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Rounded.ArrowBack, contentDescription = "Back")
                    }
                },
                title = {
                    Column {
                        Text(system.name, fontWeight = FontWeight.Bold, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        Text(
                            system.info.hostname.ifBlank { system.host },
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                        )
                    }
                },
                actions = {
                    IconButton(onClick = onRefresh) { Icon(Icons.Rounded.Refresh, contentDescription = "Refresh") }
                },
            )
        },
    ) { padding ->
        LazyColumn(
            modifier = Modifier.fillMaxSize(),
            contentPadding = androidx.compose.foundation.layout.PaddingValues(
                start = 16.dp,
                end = 16.dp,
                top = padding.calculateTopPadding() + 8.dp,
                bottom = padding.calculateBottomPadding() + 32.dp,
            ),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            item { SystemHero(system) }
            item {
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    MetricTile(
                        title = "CPU",
                        value = "${(latest?.cpu ?: system.info.cpu).toInt()}%",
                        icon = Icons.Rounded.DeveloperBoard,
                        supporting = system.info.cpuModel.ifBlank { "${system.info.cpuCores} cores" },
                        modifier = Modifier.weight(1f),
                    )
                    MetricTile(
                        title = "Memory",
                        value = "${(latest?.memory ?: system.info.memoryPercent).toInt()}%",
                        icon = Icons.Rounded.Memory,
                        supporting = "Current usage",
                        modifier = Modifier.weight(1f),
                        tint = MaterialTheme.colorScheme.secondary,
                    )
                }
            }
            item {
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    MetricTile(
                        title = "Disk",
                        value = "${(latest?.disk ?: system.info.diskPercent).toInt()}%",
                        icon = Icons.Rounded.Storage,
                        supporting = "Root filesystem",
                        modifier = Modifier.weight(1f),
                        tint = MaterialTheme.colorScheme.tertiary,
                    )
                    MetricTile(
                        title = "Network",
                        value = formatBytesPerSecond((latest?.networkDownBytes ?: 0.0) + (latest?.networkUpBytes ?: 0.0)),
                        icon = Icons.Rounded.NetworkCheck,
                        supporting = "Combined transfer",
                        modifier = Modifier.weight(1f),
                        tint = MaterialTheme.colorScheme.primary,
                    )
                }
            }
            item {
                Column {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text("History", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f))
                        StatusBadge(system.status)
                    }
                    LazyRow(
                        modifier = Modifier.padding(top = 12.dp),
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                    ) {
                        items(ChartRange.entries.size) { index ->
                            val range = ChartRange.entries[index]
                            FilterChip(
                                selected = state.chartRange == range,
                                onClick = { onRange(range) },
                                label = { Text(range.label) },
                            )
                        }
                    }
                }
            }
            item {
                Surface(
                    modifier = Modifier.fillMaxWidth(),
                    shape = MaterialTheme.shapes.large,
                    color = MaterialTheme.colorScheme.surfaceContainerLow,
                ) {
                    Column(Modifier.padding(18.dp)) {
                        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            ChartMetric.entries.forEach { option ->
                                FilterChip(
                                    selected = metric == option,
                                    onClick = { metric = option },
                                    label = { Text(option.label) },
                                )
                            }
                        }
                        Spacer(Modifier.height(8.dp))
                        when {
                            state.statsLoading -> Box(Modifier.fillMaxWidth().height(220.dp), contentAlignment = Alignment.Center) {
                                CircularProgressIndicator()
                            }
                            state.statsError != null -> Box(Modifier.fillMaxWidth().height(220.dp), contentAlignment = Alignment.Center) {
                                Text(state.statsError, color = MaterialTheme.colorScheme.error)
                            }
                            else -> MetricChart(
                                points = state.stats,
                                metric = metric,
                                modifier = Modifier.fillMaxWidth().height(220.dp),
                                lineColor = when (metric) {
                                    ChartMetric.CPU -> MaterialTheme.colorScheme.primary
                                    ChartMetric.MEMORY -> MaterialTheme.colorScheme.secondary
                                    ChartMetric.DISK -> MaterialTheme.colorScheme.tertiary
                                },
                            )
                        }
                    }
                }
            }
            item { HostInformation(system) }
        }
    }
}

@Composable
private fun SystemHero(system: SystemRecord) {
    Surface(
        modifier = Modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.extraLarge,
        color = MaterialTheme.colorScheme.primaryContainer,
        contentColor = MaterialTheme.colorScheme.onPrimaryContainer,
    ) {
        Column(Modifier.padding(24.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Rounded.Computer, contentDescription = null, modifier = Modifier.size(36.dp))
                Spacer(Modifier.size(14.dp))
                Column(Modifier.weight(1f)) {
                    Text(system.info.os.ifBlank { "Monitored system" }, style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold)
                    Text(system.info.kernel.ifBlank { system.host })
                }
            }
            Spacer(Modifier.height(18.dp))
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(24.dp)) {
                InfoValue(Icons.Rounded.Timer, "Uptime", formatUptime(system.info.uptimeSeconds), Modifier.weight(1f))
                InfoValue(Icons.Rounded.Bolt, "Load", system.info.loadAverage.firstOrNull()?.let { "%.2f".format(it) } ?: "—", Modifier.weight(1f))
                InfoValue(Icons.Rounded.Thermostat, "Temp", system.info.temperature?.let { "${it.toInt()}°C" } ?: "—", Modifier.weight(1f))
            }
        }
    }
}

@Composable
private fun InfoValue(icon: androidx.compose.ui.graphics.vector.ImageVector, label: String, value: String, modifier: Modifier = Modifier) {
    Column(modifier) {
        Icon(icon, contentDescription = null, modifier = Modifier.size(18.dp))
        Spacer(Modifier.height(6.dp))
        Text(value, fontWeight = FontWeight.Bold, maxLines = 1)
        Text(label, style = MaterialTheme.typography.labelSmall)
    }
}

@Composable
private fun HostInformation(system: SystemRecord) {
    Surface(shape = MaterialTheme.shapes.large, color = MaterialTheme.colorScheme.surfaceContainerLow) {
        Column(Modifier.fillMaxWidth().padding(18.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text("Host information", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold)
            DetailRow("Host", system.host + system.port.takeIf { it.isNotBlank() }?.let { ":$it" }.orEmpty())
            DetailRow("CPU", system.info.cpuModel.ifBlank { "${system.info.cpuCores} cores · ${system.info.cpuThreads} threads" })
            DetailRow("Agent", system.info.agentVersion.ifBlank { "Unknown" })
            DetailRow("Status", system.status.replaceFirstChar(Char::titlecase))
        }
    }
}

@Composable
private fun DetailRow(label: String, value: String) {
    Row(Modifier.fillMaxWidth()) {
        Text(label, modifier = Modifier.weight(0.35f), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Text(value, modifier = Modifier.weight(0.65f), style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.Medium)
    }
}
