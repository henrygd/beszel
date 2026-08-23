package dev.beszel.mobile.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Check
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.Dns
import androidx.compose.material.icons.rounded.Search
import androidx.compose.material.icons.rounded.SearchOff
import androidx.compose.material.icons.rounded.Sort
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import dev.beszel.mobile.AppUiState
import dev.beszel.mobile.R
import dev.beszel.mobile.data.FleetFilter
import dev.beszel.mobile.data.FleetSort
import dev.beszel.mobile.data.filterSystems
import dev.beszel.mobile.ui.components.EmptyState
import dev.beszel.mobile.ui.components.FleetPulseHeader
import dev.beszel.mobile.ui.components.SystemCard
import dev.beszel.mobile.ui.components.SystemCardSkeleton
import dev.beszel.mobile.ui.theme.dataSmall

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun FleetScreen(
    state: AppUiState,
    contentPadding: PaddingValues,
    onRefresh: () -> Unit,
    onSystemClick: (String) -> Unit,
) {
    var query by rememberSaveable { mutableStateOf("") }
    var filter by rememberSaveable { mutableStateOf(FleetFilter.ALL) }
    var sort by rememberSaveable { mutableStateOf(FleetSort.STATUS) }
    var searchVisible by rememberSaveable { mutableStateOf(false) }
    var sortMenuOpen by remember { mutableStateOf(false) }
    val haptics = LocalHapticFeedback.current

    val alertsBySystem = remember(state.alerts) {
        state.activeAlerts.groupBy { it.systemId }.mapValues { (_, value) -> value.size }
    }
    val visibleSystems = filterSystems(state.systems, state.alerts, query, filter, sort)

    PullToRefreshBox(
        isRefreshing = state.isRefreshing,
        onRefresh = {
            haptics.performHapticFeedback(HapticFeedbackType.TextHandleMove)
            onRefresh()
        },
        modifier = Modifier.fillMaxSize(),
    ) {
        LazyVerticalGrid(
            columns = GridCells.Adaptive(340.dp),
            modifier = Modifier
                .fillMaxSize()
                .statusBarsPadding(),
            contentPadding = PaddingValues(
                start = 16.dp,
                end = 16.dp,
                top = 12.dp,
                bottom = contentPadding.calculateBottomPadding() + 24.dp,
            ),
            horizontalArrangement = Arrangement.spacedBy(14.dp),
            verticalArrangement = Arrangement.spacedBy(14.dp),
        ) {
            if (state.systems.isNotEmpty()) {
                item(span = { GridItemSpan(maxLineSpan) }) {
                    Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.TopCenter) {
                        Row(Modifier.widthIn(max = 1280.dp).fillMaxWidth()) {
                            FleetPulseHeader(
                                pulse = state.pulse,
                                onlineCount = state.systems.count { it.status == "up" },
                                downCount = state.systems.count { it.status == "down" },
                                pendingCount = state.systems.count { it.status == "pending" },
                                pausedCount = state.systems.count { it.status == "paused" },
                                alertCount = state.activeAlerts.size,
                                lastUpdated = state.lastUpdated,
                                modifier = Modifier.weight(1f),
                            )
                        }
                    }
                }
            }

            item(span = { GridItemSpan(maxLineSpan) }) {
                FleetToolbar(
                    totalCount = state.systems.size,
                    visibleCount = visibleSystems.size,
                    searchVisible = searchVisible,
                    sort = sort,
                    sortMenuOpen = sortMenuOpen,
                    onSearchToggle = { searchVisible = it },
                    onSortSelect = { sort = it; sortMenuOpen = false },
                    onSortMenuDismiss = { sortMenuOpen = false },
                    onSortMenuOpen = { sortMenuOpen = true },
                )
            }

            if (searchVisible) {
                item(span = { GridItemSpan(maxLineSpan) }) {
                    OutlinedTextField(
                        value = query,
                        onValueChange = { query = it },
                        modifier = Modifier.fillMaxWidth(),
                        placeholder = { Text(stringResource(R.string.fleet_search_hint)) },
                        singleLine = true,
                        leadingIcon = { Icon(Icons.Rounded.Search, contentDescription = null) },
                        trailingIcon = {
                            IconButton(onClick = { query = ""; searchVisible = false }) {
                                Icon(Icons.Rounded.Close, contentDescription = stringResource(R.string.action_close))
                            }
                        },
                    )
                }
            }

            item(span = { GridItemSpan(maxLineSpan) }) {
                dev.beszel.mobile.ui.components.SegmentedControl(
                    options = FleetFilter.entries,
                    selected = filter,
                    onSelect = { filter = it },
                    label = { filterLabel(it) },
                    modifier = Modifier.fillMaxWidth(),
                )
            }

            when {
                state.isLoadingFleet -> {
                    items(6) {
                        SystemCardSkeleton()
                    }
                }
                state.systems.isEmpty() -> {
                    item(span = { GridItemSpan(maxLineSpan) }) {
                        EmptyState(
                            icon = Icons.Rounded.Dns,
                            title = stringResource(R.string.fleet_empty_title),
                            message = stringResource(R.string.fleet_empty_message),
                            actionLabel = stringResource(R.string.action_refresh),
                            onAction = onRefresh,
                        )
                    }
                }
                visibleSystems.isEmpty() -> {
                    item(span = { GridItemSpan(maxLineSpan) }) {
                        EmptyState(
                            icon = Icons.Rounded.SearchOff,
                            title = stringResource(R.string.fleet_no_matches_title),
                            message = stringResource(R.string.fleet_no_matches_message),
                        )
                    }
                }
                else -> {
                    items(visibleSystems, key = { it.id }, span = { GridItemSpan(1) }) { system ->
                        SystemCard(
                            system = system,
                            alertCount = alertsBySystem[system.id] ?: 0,
                            onClick = { onSystemClick(system.id) },
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun filterLabel(filter: FleetFilter): String = when (filter) {
    FleetFilter.ALL -> stringResource(R.string.fleet_filter_all)
    FleetFilter.ALERTING -> stringResource(R.string.fleet_filter_alerting)
    FleetFilter.DOWN -> stringResource(R.string.fleet_filter_down)
}

@Composable
private fun FleetToolbar(
    totalCount: Int,
    visibleCount: Int,
    searchVisible: Boolean,
    sort: FleetSort,
    sortMenuOpen: Boolean,
    onSearchToggle: (Boolean) -> Unit,
    onSortSelect: (FleetSort) -> Unit,
    onSortMenuDismiss: () -> Unit,
    onSortMenuOpen: () -> Unit,
) {
    Row(
        modifier = Modifier.fillMaxWidth().padding(top = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(Modifier.weight(1f)) {
            Text(
                stringResource(R.string.fleet_systems_title),
                style = MaterialTheme.typography.titleLarge,
            )
            Text(
                pluralStringResource(R.plurals.fleet_systems_count, visibleCount, visibleCount, totalCount),
                style = MaterialTheme.typography.dataSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        if (!searchVisible) {
            IconButton(onClick = { onSearchToggle(true) }) {
                Icon(Icons.Rounded.Search, contentDescription = stringResource(R.string.action_search))
            }
        }
        Box {
            IconButton(onClick = onSortMenuOpen) {
                Icon(Icons.Rounded.Sort, contentDescription = stringResource(R.string.action_sort))
            }
            DropdownMenu(expanded = sortMenuOpen, onDismissRequest = onSortMenuDismiss) {
                FleetSort.entries.forEach { option ->
                    DropdownMenuItem(
                        text = { Text(sortLabel(option)) },
                        onClick = { onSortSelect(option) },
                        leadingIcon = if (sort == option) {
                            { Icon(Icons.Rounded.Check, contentDescription = null, Modifier.size(16.dp)) }
                        } else null,
                    )
                }
            }
        }
    }
}

@Composable
private fun sortLabel(sort: FleetSort): String = when (sort) {
    FleetSort.NAME -> stringResource(R.string.sort_name)
    FleetSort.STATUS -> stringResource(R.string.sort_status)
    FleetSort.CPU -> stringResource(R.string.sort_cpu)
}
