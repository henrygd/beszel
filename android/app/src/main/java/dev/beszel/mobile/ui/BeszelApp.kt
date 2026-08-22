package dev.beszel.mobile.ui

import androidx.activity.compose.BackHandler
import androidx.compose.animation.Crossfade
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.WindowInsetsSides
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Dashboard
import androidx.compose.material.icons.rounded.Notifications
import androidx.compose.material.icons.rounded.Settings
import androidx.compose.material3.Badge
import androidx.compose.material3.BadgedBox
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationRail
import androidx.compose.material3.NavigationRailItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.unit.dp
import dev.beszel.mobile.AppUiState
import dev.beszel.mobile.AppViewModel
import dev.beszel.mobile.ui.components.LoadingState
import dev.beszel.mobile.ui.screens.AlertsScreen
import dev.beszel.mobile.ui.screens.DashboardScreen
import dev.beszel.mobile.ui.screens.LoginScreen
import dev.beszel.mobile.ui.screens.SettingsScreen
import dev.beszel.mobile.ui.screens.SystemDetailScreen

private enum class Destination(val label: String, val icon: ImageVector) {
    OVERVIEW("Overview", Icons.Rounded.Dashboard),
    ALERTS("Alerts", Icons.Rounded.Notifications),
    SETTINGS("Settings", Icons.Rounded.Settings),
}

@Composable
fun BeszelApp(state: AppUiState, viewModel: AppViewModel) {
    val snackbar = remember { SnackbarHostState() }
    LaunchedEffect(state.message) {
        state.message?.let {
            snackbar.showSnackbar(it)
            viewModel.clearMessage()
        }
    }

    Box(Modifier.fillMaxSize()) {
        when {
            state.isStarting -> LoadingState()
            state.session == null -> LoginScreen(
                isLoading = state.isLoggingIn,
                onLogin = viewModel::login,
            )
            else -> MainContent(state, viewModel)
        }
        SnackbarHost(
            hostState = snackbar,
            modifier = Modifier.align(Alignment.BottomCenter),
        )
    }
}

@Composable
private fun MainContent(state: AppUiState, viewModel: AppViewModel) {
    val detailId = state.detailSystemId
    val detailSystem = state.systems.firstOrNull { it.id == detailId }
    if (detailId != null && detailSystem != null) {
        BackHandler(onBack = viewModel::closeSystem)
        SystemDetailScreen(
            system = detailSystem,
            state = state,
            onBack = viewModel::closeSystem,
            onRange = viewModel::selectChartRange,
            onRefresh = {
                viewModel.refresh()
                viewModel.selectChartRange(state.chartRange)
            },
        )
        return
    }

    var destination by rememberSaveable { mutableStateOf(Destination.OVERVIEW) }
    BoxWithConstraints(Modifier.fillMaxSize()) {
        val useRail = maxWidth >= 600.dp
        Scaffold(
            contentWindowInsets = WindowInsets(0),
            bottomBar = {
                if (!useRail) {
                    AppNavigationBar(
                        selected = destination,
                        alertCount = state.activeAlerts.size,
                        onSelect = { destination = it },
                    )
                }
            },
        ) { scaffoldPadding ->
            Row(Modifier.fillMaxSize()) {
                if (useRail) {
                    AppNavigationRail(
                        selected = destination,
                        alertCount = state.activeAlerts.size,
                        onSelect = { destination = it },
                    )
                }
                Crossfade(targetState = destination, label = "primary navigation", modifier = Modifier.weight(1f)) { screen ->
                    when (screen) {
                        Destination.OVERVIEW -> DashboardScreen(
                            state = state,
                            contentPadding = scaffoldPadding,
                            onRefresh = viewModel::refresh,
                            onSystemClick = viewModel::openSystem,
                        )
                        Destination.ALERTS -> AlertsScreen(state, scaffoldPadding)
                        Destination.SETTINGS -> SettingsScreen(
                            state = state,
                            contentPadding = scaffoldPadding,
                            onThemeMode = viewModel::setThemeMode,
                            onDynamicColor = viewModel::setDynamicColor,
                            onLogout = viewModel::logout,
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun AppNavigationBar(
    selected: Destination,
    alertCount: Int,
    onSelect: (Destination) -> Unit,
) {
    NavigationBar(containerColor = MaterialTheme.colorScheme.surfaceContainer) {
        Destination.entries.forEach { destination ->
            NavigationBarItem(
                selected = selected == destination,
                onClick = { onSelect(destination) },
                icon = { DestinationIcon(destination, alertCount) },
                label = { Text(destination.label) },
            )
        }
    }
}

@Composable
private fun AppNavigationRail(
    selected: Destination,
    alertCount: Int,
    onSelect: (Destination) -> Unit,
) {
    NavigationRail(
        modifier = Modifier
            .fillMaxHeight()
            .windowInsetsPadding(WindowInsets.safeDrawing.only(WindowInsetsSides.Vertical)),
        containerColor = MaterialTheme.colorScheme.surfaceContainer,
    ) {
        Destination.entries.forEach { destination ->
            NavigationRailItem(
                selected = selected == destination,
                onClick = { onSelect(destination) },
                icon = { DestinationIcon(destination, alertCount) },
                label = { Text(destination.label) },
            )
        }
    }
}

@Composable
private fun DestinationIcon(destination: Destination, alertCount: Int) {
    if (destination == Destination.ALERTS && alertCount > 0) {
        BadgedBox(badge = { Badge { Text(alertCount.coerceAtMost(99).toString()) } }) {
            Icon(destination.icon, contentDescription = destination.label)
        }
    } else {
        Icon(destination.icon, contentDescription = destination.label)
    }
}
