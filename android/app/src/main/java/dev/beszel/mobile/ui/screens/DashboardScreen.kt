package dev.beszel.mobile.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.CheckCircle
import androidx.compose.material.icons.rounded.Error
import androidx.compose.material.icons.rounded.NotificationsActive
import androidx.compose.material.icons.rounded.Refresh
import androidx.compose.material.icons.rounded.SearchOff
import androidx.compose.material.icons.rounded.Warning
import androidx.compose.material3.AssistChip
import androidx.compose.material3.AssistChipDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import dev.beszel.mobile.AppUiState
import dev.beszel.mobile.data.alertPresentation
import dev.beszel.mobile.ui.components.BeszelMark
import dev.beszel.mobile.ui.components.SystemCard
import java.text.DateFormat
import java.util.Date

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DashboardScreen(
    state: AppUiState,
    contentPadding: androidx.compose.foundation.layout.PaddingValues,
    onRefresh: () -> Unit,
    onSystemClick: (String) -> Unit,
) {
    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                        BeszelMark(compact = true)
                        Column {
                            Text("Overview", fontWeight = FontWeight.Bold)
                            state.lastUpdated?.let {
                                Text(
                                    "Updated ${DateFormat.getTimeInstance(DateFormat.SHORT).format(Date(it))}",
                                    style = MaterialTheme.typography.labelSmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                            }
                        }
                    }
                },
                actions = {
                    IconButton(onClick = onRefresh, enabled = !state.isRefreshing) {
                        if (state.isRefreshing) CircularProgressIndicator(Modifier.size(22.dp), strokeWidth = 2.dp)
                        else Icon(Icons.Rounded.Refresh, contentDescription = "Refresh")
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.surface),
            )
        },
        contentWindowInsets = androidx.compose.foundation.layout.WindowInsets(0),
    ) { topPadding ->
		Box(Modifier.fillMaxSize(), contentAlignment = Alignment.TopCenter) {
		LazyVerticalGrid(
			columns = GridCells.Adaptive(310.dp),
			modifier = Modifier.fillMaxHeight().fillMaxWidth().widthIn(max = 1280.dp),
            contentPadding = androidx.compose.foundation.layout.PaddingValues(
                start = 16.dp,
                end = 16.dp,
                top = topPadding.calculateTopPadding() + 8.dp,
                bottom = contentPadding.calculateBottomPadding() + 24.dp,
            ),
            horizontalArrangement = Arrangement.spacedBy(14.dp),
            verticalArrangement = Arrangement.spacedBy(14.dp),
        ) {
            item(span = { GridItemSpan(maxLineSpan) }) {
                HealthSummary(state)
            }
            if (state.activeAlerts.isNotEmpty()) {
                item(span = { GridItemSpan(maxLineSpan) }) {
                    ActiveAlertSummary(state)
                }
            }
            item(span = { GridItemSpan(maxLineSpan) }) {
                Row(
                    modifier = Modifier.fillMaxWidth().padding(top = 10.dp, bottom = 2.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text("Systems", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f))
                    Text("${state.systems.size} total", color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
            if (state.systems.isEmpty()) {
                item(span = { GridItemSpan(maxLineSpan) }) {
                    EmptySystems()
                }
            } else {
                items(state.systems, key = { it.id }) { system ->
                    SystemCard(system = system, onClick = { onSystemClick(system.id) })
			}
		}
		}
	}
}
}

@Composable
private fun HealthSummary(state: AppUiState) {
	val online = state.systems.count { it.status == "up" }
	val offline = state.systems.count { it.status == "down" }
	val pending = state.systems.count { it.status == "pending" }
	val paused = state.systems.count { it.status == "paused" }
	val empty = state.systems.isEmpty()
	val needsAttention = offline > 0 || state.activeAlerts.isNotEmpty()
	val waitingForAgents = !empty && pending > 0
	val containerColor = when {
		needsAttention -> MaterialTheme.colorScheme.errorContainer
		waitingForAgents -> MaterialTheme.colorScheme.tertiaryContainer
		else -> MaterialTheme.colorScheme.primaryContainer
	}
	val contentColor = when {
		needsAttention -> MaterialTheme.colorScheme.onErrorContainer
		waitingForAgents -> MaterialTheme.colorScheme.onTertiaryContainer
		else -> MaterialTheme.colorScheme.onPrimaryContainer
	}
	Surface(
		modifier = Modifier.fillMaxWidth(),
		shape = MaterialTheme.shapes.extraLarge,
		color = containerColor,
		contentColor = contentColor,
    ) {
        Column(Modifier.padding(24.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
				Icon(
					when {
						empty -> Icons.Rounded.SearchOff
						needsAttention || waitingForAgents -> Icons.Rounded.Warning
						else -> Icons.Rounded.CheckCircle
					},
                    contentDescription = null,
                    modifier = Modifier.size(34.dp),
                )
                Spacer(Modifier.size(14.dp))
                Column(Modifier.weight(1f)) {
                    Text(
						when {
							empty -> "No systems connected"
							needsAttention -> "Your attention is needed"
							waitingForAgents -> "Agents awaiting connection"
							else -> "Everything looks good"
						},
                        style = MaterialTheme.typography.titleLarge,
                        fontWeight = FontWeight.Bold,
                    )
                    Text(
						if (empty) "Connect systems from the Beszel web dashboard"
						else "$online online · $pending pending · $offline offline · $paused paused · ${state.activeAlerts.size} alerts",
                        style = MaterialTheme.typography.bodyMedium,
                    )
                }
            }
        }
    }
}

@Composable
private fun ActiveAlertSummary(state: AppUiState) {
    val systems = state.systems.associateBy { it.id }
    Surface(
        modifier = Modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.large,
        color = MaterialTheme.colorScheme.errorContainer.copy(alpha = 0.72f),
        contentColor = MaterialTheme.colorScheme.onErrorContainer,
    ) {
        Column(Modifier.padding(18.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Rounded.NotificationsActive, contentDescription = null)
                Spacer(Modifier.size(10.dp))
                Text("Active alerts", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold)
            }
            Spacer(Modifier.height(10.dp))
            state.activeAlerts.take(3).forEach { alert ->
                val info = alertPresentation(alert)
                Text(
                    "${systems[alert.systemId]?.name ?: "System"} · ${info.title}: ${info.description}",
                    modifier = Modifier.padding(vertical = 3.dp),
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
            if (state.activeAlerts.size > 3) {
                Text("+${state.activeAlerts.size - 3} more", style = MaterialTheme.typography.labelLarge)
            }
        }
    }
}

@Composable
private fun EmptySystems() {
    Surface(shape = MaterialTheme.shapes.large, color = MaterialTheme.colorScheme.surfaceContainerLow) {
        Column(
            modifier = Modifier.fillMaxWidth().padding(36.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Icon(Icons.Rounded.SearchOff, contentDescription = null, modifier = Modifier.size(42.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant)
            Spacer(Modifier.height(12.dp))
            Text("No systems found", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold)
            Text("Add a system from your Beszel hub, then refresh.", color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}
