package dev.beszel.mobile.ui.screens

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.material.icons.rounded.BatteryFull
import androidx.compose.material.icons.rounded.Bolt
import androidx.compose.material.icons.rounded.Computer
import androidx.compose.material.icons.rounded.DeveloperBoard
import androidx.compose.material.icons.rounded.DeviceThermostat
import androidx.compose.material.icons.rounded.Memory
import androidx.compose.material.icons.rounded.NetworkCheck
import androidx.compose.material.icons.rounded.Refresh
import androidx.compose.material.icons.rounded.Storage
import androidx.compose.material3.ExperimentalMaterial3Api
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
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import dev.beszel.mobile.R
import dev.beszel.mobile.SystemDetailUiState
import dev.beszel.mobile.data.ChartRange
import dev.beszel.mobile.data.SystemRecord
import dev.beszel.mobile.data.formatBytesPerSecond
import dev.beszel.mobile.data.formatUptime
import dev.beszel.mobile.ui.charts.ChartSeries
import dev.beszel.mobile.ui.charts.LineChart
import dev.beszel.mobile.ui.components.AnimatedNumber
import dev.beszel.mobile.ui.components.ErrorState
import dev.beszel.mobile.ui.components.SegmentedControl
import dev.beszel.mobile.ui.components.ShimmerLine
import dev.beszel.mobile.ui.components.SplashState
import dev.beszel.mobile.ui.components.StatusDot
import dev.beszel.mobile.ui.components.statusColor
import dev.beszel.mobile.ui.components.statusLabel
import dev.beszel.mobile.ui.components.shimmerBrush
import dev.beszel.mobile.ui.theme.BeszelTheme
import dev.beszel.mobile.ui.theme.dataMedium
import dev.beszel.mobile.ui.theme.dataSmall
import dev.beszel.mobile.ui.theme.metricValueLarge
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import kotlin.math.roundToInt

enum class DetailMetric(val label: String) {
    CPU("CPU"), MEMORY("Memory"), DISK("Disk"), NETWORK("Network"),
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SystemDetailScreen(
    system: SystemRecord?,
    detailState: SystemDetailUiState,
    onBack: () -> Unit,
    onRange: (ChartRange) -> Unit,
    onRetryStats: () -> Unit,
    onRefresh: () -> Unit,
) {
    if (system == null) {
        SplashState()
        return
    }
    val metrics = BeszelTheme.metrics
    var metric by rememberSaveable { mutableStateOf(DetailMetric.CPU) }
    val stats = detailState.stats
    val latest = stats.lastOrNull()

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Rounded.ArrowBack, contentDescription = stringResource(R.string.action_back))
                    }
                },
                title = {
                    Column {
                        Text(
                            system.name,
                            style = MaterialTheme.typography.titleMedium,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                        )
                        Text(
                            system.info.hostname.ifBlank { system.host },
                            style = MaterialTheme.typography.dataSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                        )
                    }
                },
                actions = {
                    IconButton(onClick = {
                        onRefresh()
                        onRetryStats()
                    }) {
                        Icon(Icons.Rounded.Refresh, contentDescription = stringResource(R.string.action_refresh))
                    }
                },
            )
        },
    ) { padding ->
        LazyColumn(
            modifier = Modifier.fillMaxSize(),
            contentPadding = PaddingValues(
                start = 16.dp,
                end = 16.dp,
                top = padding.calculateTopPadding() + 8.dp,
                bottom = padding.calculateBottomPadding() + 32.dp,
            ),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            item { SystemHeader(system) }
            item {
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    MetricTile(
                        label = stringResource(R.string.metric_cpu),
                        valuePercent = latest?.cpu ?: system.info.cpu,
                        color = metrics.cpu,
                        icon = Icons.Rounded.DeveloperBoard,
                        supporting = system.info.cpuModel.ifBlank {
                            if (system.info.cpuCores > 0) "${system.info.cpuCores} cores" else ""
                        },
                        modifier = Modifier.weight(1f),
                    )
                    MetricTile(
                        label = stringResource(R.string.metric_memory),
                        valuePercent = latest?.memory ?: system.info.memoryPercent,
                        color = metrics.memory,
                        icon = Icons.Rounded.Memory,
                        supporting = stringResource(R.string.metric_current_usage),
                        modifier = Modifier.weight(1f),
                    )
                }
            }
            item {
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    MetricTile(
                        label = stringResource(R.string.metric_disk),
                        valuePercent = latest?.disk ?: system.info.diskPercent,
                        color = metrics.disk,
                        icon = Icons.Rounded.Storage,
                        supporting = stringResource(R.string.metric_root_filesystem),
                        modifier = Modifier.weight(1f),
                    )
                    NetworkTile(
                        downBytesPerSecond = latest?.networkDownBytes,
                        upBytesPerSecond = latest?.networkUpBytes,
                        color = metrics.network,
                        modifier = Modifier.weight(1f),
                    )
                }
            }
            item {
                HistoryCard(
                    system = system,
                    detailState = detailState,
                    metric = metric,
                    onMetricChange = { metric = it },
                    onRange = onRange,
                    onRetry = onRetryStats,
                )
            }
            item { HostInformation(system) }
        }
    }
}

@Composable
private fun SystemHeader(system: SystemRecord) {
    val metrics = BeszelTheme.metrics
    val statusColor = statusColor(system.status)
    Surface(
        modifier = Modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.extraLarge,
        color = MaterialTheme.colorScheme.surfaceContainerLow,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
    ) {
        Column(Modifier.padding(20.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                StatusDot(color = statusColor, pulse = system.isUp)
                Spacer(Modifier.width(10.dp))
                Text(
                    statusLabel(system.status),
                    style = MaterialTheme.typography.titleMedium,
                    color = statusColor,
                )
                Spacer(Modifier.weight(1f))
                Text(
                    system.info.os.ifBlank { stringResource(R.string.detail_monitored_system) },
                    style = MaterialTheme.typography.dataSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
            if (system.info.kernel.isNotBlank()) {
                Text(
                    system.info.kernel,
                    style = MaterialTheme.typography.dataSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.padding(top = 4.dp),
                )
            }
            Spacer(Modifier.height(16.dp))
            Row(
                Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(10.dp),
            ) {
                HeaderChip(Icons.Rounded.Computer, "up ${formatUptime(system.info.uptimeSeconds)}", metrics.healthy)
                system.info.loadAverage.firstOrNull()?.let {
                    HeaderChip(Icons.Rounded.Bolt, stringResource(R.string.detail_load_format, it), metrics.warning)
                }
                system.info.temperature?.let {
                    HeaderChip(Icons.Rounded.DeviceThermostat, "${it.roundToInt()}°C", metrics.temperature)
                }
                system.info.gpuPercent?.let {
                    HeaderChip(Icons.Rounded.Memory, "GPU ${it.roundToInt()}%", metrics.gpu)
                }
                system.info.batteryPercent?.let {
                    HeaderChip(Icons.Rounded.BatteryFull, "BAT ${it.roundToInt()}%", metrics.battery)
                }
            }
        }
    }
}

@Composable
private fun HeaderChip(icon: ImageVector, text: String, color: Color) {
    Surface(shape = CircleShape, color = color.copy(alpha = 0.12f), contentColor = color) {
        Row(
            Modifier.padding(horizontal = 10.dp, vertical = 6.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            Icon(icon, contentDescription = null, modifier = Modifier.size(14.dp))
            Text(text, style = MaterialTheme.typography.dataSmall)
        }
    }
}

@Composable
private fun MetricTile(
    label: String,
    valuePercent: Float,
    color: Color,
    icon: ImageVector,
    supporting: String,
    modifier: Modifier = Modifier,
) {
    Surface(
        modifier = modifier,
        shape = MaterialTheme.shapes.large,
        color = MaterialTheme.colorScheme.surfaceContainerLow,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
    ) {
        Column(Modifier.padding(16.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Surface(
                    color = color.copy(alpha = 0.14f),
                    contentColor = color,
                    shape = RoundedCornerShape(10.dp),
                ) {
                    Icon(icon, contentDescription = null, modifier = Modifier.padding(7.dp).size(18.dp))
                }
                Spacer(Modifier.width(8.dp))
                Text(label, style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            Spacer(Modifier.height(14.dp))
            AnimatedNumber(
                value = valuePercent.coerceIn(0f, 100f),
                format = { "${it.roundToInt()}%" },
                style = MaterialTheme.typography.metricValueLarge,
            )
            Spacer(Modifier.height(12.dp))
            Box(Modifier.fillMaxWidth().height(5.dp)) {
                Spacer(Modifier.fillMaxWidth().height(5.dp).background(color.copy(alpha = 0.14f), CircleShape))
                val fraction = (valuePercent.coerceIn(0f, 100f) / 100f).coerceIn(0f, 1f)
                Spacer(
                    Modifier
                        .fillMaxWidth(fraction)
                        .height(5.dp)
                        .background(color, CircleShape),
                )
            }
            if (supporting.isNotBlank()) {
                Spacer(Modifier.height(10.dp))
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

@Composable
private fun NetworkTile(
    downBytesPerSecond: Double?,
    upBytesPerSecond: Double?,
    color: Color,
    modifier: Modifier = Modifier,
) {
    val total = (downBytesPerSecond ?: 0.0) + (upBytesPerSecond ?: 0.0)
    Surface(
        modifier = modifier,
        shape = MaterialTheme.shapes.large,
        color = MaterialTheme.colorScheme.surfaceContainerLow,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
    ) {
        Column(Modifier.padding(16.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Surface(
                    color = color.copy(alpha = 0.14f),
                    contentColor = color,
                    shape = RoundedCornerShape(10.dp),
                ) {
                    Icon(Icons.Rounded.NetworkCheck, contentDescription = null, modifier = Modifier.padding(7.dp).size(18.dp))
                }
                Spacer(Modifier.width(8.dp))
                Text(
                    stringResource(R.string.metric_network),
                    style = MaterialTheme.typography.labelLarge,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Spacer(Modifier.height(14.dp))
            Text(
                formatBytesPerSecond(total),
                style = MaterialTheme.typography.metricValueLarge,
            )
            Spacer(Modifier.height(12.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                Column {
                    Text(stringResource(R.string.network_in), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    Text(
                        formatBytesPerSecond(downBytesPerSecond ?: 0.0),
                        style = MaterialTheme.typography.dataMedium,
                    )
                }
                Column {
                    Text(stringResource(R.string.network_out), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    Text(
                        formatBytesPerSecond(upBytesPerSecond ?: 0.0),
                        style = MaterialTheme.typography.dataMedium,
                    )
                }
            }
        }
    }
}

@Composable
private fun HistoryCard(
    system: SystemRecord,
    detailState: SystemDetailUiState,
    metric: DetailMetric,
    onMetricChange: (DetailMetric) -> Unit,
    onRange: (ChartRange) -> Unit,
    onRetry: () -> Unit,
) {
    val metrics = BeszelTheme.metrics
    val stats = detailState.stats
    val range = detailState.range

    Surface(
        modifier = Modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.extraLarge,
        color = MaterialTheme.colorScheme.surfaceContainerLow,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
    ) {
        Column(Modifier.padding(18.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
            Text(stringResource(R.string.detail_history_title), style = MaterialTheme.typography.titleMedium)
            SegmentedControl(
                options = ChartRange.entries.toList(),
                selected = range,
                onSelect = onRange,
                label = { it.label },
            )
            SegmentedControl(
                options = DetailMetric.entries.toList(),
                selected = metric,
                onSelect = onMetricChange,
                label = { it.label },
            )

            if (metric == DetailMetric.NETWORK) {
                Row(horizontalArrangement = Arrangement.spacedBy(14.dp)) {
                    LegendDot(metrics.network, stringResource(R.string.network_in))
                    LegendDot(metrics.networkAlt, stringResource(R.string.network_out))
                }
            }

            Box(Modifier.fillMaxWidth().height(240.dp)) {
                when {
                    detailState.isLoading -> ChartSkeleton()
                    detailState.error != null -> ErrorState(message = detailState.error, onRetry = onRetry)
                    stats.size < 2 -> EmptyChart()
                    else -> {
                        val timestamps = stats.map { it.timestamp }
                        val series = when (metric) {
                            DetailMetric.CPU -> listOf(ChartSeries(stats.map { it.cpu }, metrics.cpu, "CPU"))
                            DetailMetric.MEMORY -> listOf(ChartSeries(stats.map { it.memory }, metrics.memory, stringResource(R.string.metric_memory)))
                            DetailMetric.DISK -> listOf(ChartSeries(stats.map { it.disk }, metrics.disk, stringResource(R.string.metric_disk)))
                            DetailMetric.NETWORK -> listOf(
                                ChartSeries(stats.map { it.networkDownBytes.toFloat() }, metrics.network, stringResource(R.string.network_in)),
                                ChartSeries(stats.map { it.networkUpBytes.toFloat() }, metrics.networkAlt, stringResource(R.string.network_out)),
                            )
                        }
                        val percentFormat: (Float) -> String = { "${it.roundToInt()}%" }
                        val bytesFormat: (Float) -> String = { formatBytesPerSecond(it.toDouble()) }
                        val showWeek = range.hours >= 48
                        val timeFormat = remember(range) {
                            SimpleDateFormat(if (showWeek) "EEE HH:mm" else "HH:mm", Locale.getDefault())
                        }
                        LineChart(
                            timestamps = timestamps,
                            series = series,
                            valueFormat = if (metric == DetailMetric.NETWORK) bytesFormat else percentFormat,
                            timeFormat = { timeFormat.format(Date(it)) },
                            yMax = if (metric == DetailMetric.NETWORK) null else 100f,
                            chartDescription = stringResource(
                                R.string.detail_chart_description,
                                metric.label,
                            ),
                            modifier = Modifier.fillMaxSize(),
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun LegendDot(color: Color, label: String) {
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
        Box(Modifier.size(8.dp).background(color, CircleShape))
        Text(label, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
private fun ChartSkeleton() {
    Column(Modifier.fillMaxSize(), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        ShimmerLine(240.dp, height = 14.dp)
        ShimmerLine(160.dp, height = 14.dp)
        Box(
            Modifier
                .weight(1f)
                .fillMaxWidth()
                .background(shimmerBrush(), MaterialTheme.shapes.extraSmall),
        )
        ShimmerLine(200.dp, height = 14.dp)
    }
}

@Composable
private fun EmptyChart() {
    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Text(
            stringResource(R.string.detail_chart_empty),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun HostInformation(system: SystemRecord) {
    Surface(
        modifier = Modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.extraLarge,
        color = MaterialTheme.colorScheme.surfaceContainerLow,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
    ) {
        Column(Modifier.padding(18.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(stringResource(R.string.detail_host_info), style = MaterialTheme.typography.titleMedium)
            DetailRow(stringResource(R.string.detail_host), system.host + system.port.takeIf { it.isNotBlank() }?.let { ":$it" }.orEmpty())
            DetailRow(
                stringResource(R.string.metric_cpu),
                system.info.cpuModel.ifBlank {
                    if (system.info.cpuCores > 0) {
                        "${system.info.cpuCores} cores · ${system.info.cpuThreads} threads"
                    } else {
                        ""
                    }
                },
            )
            DetailRow(stringResource(R.string.detail_agent), system.info.agentVersion.ifBlank { stringResource(R.string.detail_agent_unknown) })
            DetailRow(stringResource(R.string.detail_status), statusLabel(system.status))
        }
    }
}

@Composable
private fun DetailRow(label: String, value: String) {
    Row(Modifier.fillMaxWidth()) {
        Text(
            label,
            modifier = Modifier.weight(0.35f),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Text(
            value,
            modifier = Modifier.weight(0.65f),
            style = MaterialTheme.typography.dataMedium,
        )
    }
}
