package dev.beszel.mobile.ui

import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInHorizontally
import androidx.compose.animation.slideOutHorizontally
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
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.NavGraph.Companion.findStartDestination
import androidx.navigation.NavHostController
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import dev.beszel.mobile.AppUiState
import dev.beszel.mobile.AppViewModel
import dev.beszel.mobile.SystemDetailViewModel
import dev.beszel.mobile.ui.components.SplashState
import dev.beszel.mobile.ui.screens.AlertsScreen
import dev.beszel.mobile.ui.screens.FleetScreen
import dev.beszel.mobile.ui.screens.LoginScreen
import dev.beszel.mobile.ui.screens.SettingsScreen
import dev.beszel.mobile.ui.screens.SystemDetailScreen
import dev.beszel.mobile.ui.theme.BeszelMotion

enum class TopLevelDestination(val route: String, val label: String, val icon: ImageVector) {
    FLEET("fleet", "Fleet", Icons.Rounded.Dashboard),
    ALERTS("alerts", "Alerts", Icons.Rounded.Notifications),
    SETTINGS("settings", "Settings", Icons.Rounded.Settings),
}

const val SYSTEM_DETAIL_ROUTE = "system/{systemId}"

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
            state.isStarting -> SplashState()
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
    val navController = rememberNavController()
    val backStackEntry by navController.currentBackStackEntryAsState()
    val currentRoute = backStackEntry?.destination?.route
    val isTopLevel = TopLevelDestination.entries.any { it.route == currentRoute }

    BoxWithConstraints(Modifier.fillMaxSize()) {
        val useRail = maxWidth >= 600.dp
        Scaffold(
            contentWindowInsets = WindowInsets(0),
            bottomBar = {
                if (!useRail && isTopLevel) {
                    AppNavigationBar(
                        selectedRoute = currentRoute,
                        alertCount = state.activeAlerts.size,
                        onSelect = { destination -> navController.navigateToTopLevel(destination) },
                    )
                }
            },
        ) { scaffoldPadding ->
            Row(Modifier.fillMaxSize()) {
                if (useRail && isTopLevel) {
                    AppNavigationRail(
                        selectedRoute = currentRoute,
                        alertCount = state.activeAlerts.size,
                        onSelect = { destination -> navController.navigateToTopLevel(destination) },
                    )
                }
                NavHost(
                    navController = navController,
                    startDestination = TopLevelDestination.FLEET.route,
                    modifier = Modifier.weight(1f),
                    enterTransition = { fadeIn(tween(220, easing = BeszelMotion.emphasizedDecelerate)) },
                    exitTransition = { fadeOut(tween(160, easing = BeszelMotion.emphasizedAccelerate)) },
                    popEnterTransition = { fadeIn(tween(220, easing = BeszelMotion.emphasizedDecelerate)) },
                    popExitTransition = { fadeOut(tween(160, easing = BeszelMotion.emphasizedAccelerate)) },
                ) {
                    composable(TopLevelDestination.FLEET.route) {
                        FleetScreen(
                            state = state,
                            contentPadding = scaffoldPadding,
                            onRefresh = viewModel::refresh,
                            onSystemClick = { systemId -> navController.navigate("system/$systemId") },
                        )
                    }
                    composable(TopLevelDestination.ALERTS.route) {
                        AlertsScreen(state = state, contentPadding = scaffoldPadding)
                    }
                    composable(TopLevelDestination.SETTINGS.route) {
                        SettingsScreen(
                            state = state,
                            contentPadding = scaffoldPadding,
                            onThemeMode = viewModel::setThemeMode,
                            onDynamicColor = viewModel::setDynamicColor,
                            onLogout = viewModel::logout,
                        )
                    }
                    composable(
                        route = SYSTEM_DETAIL_ROUTE,
                        arguments = listOf(navArgument("systemId") { type = NavType.StringType }),
                        enterTransition = {
                            slideInHorizontally(tween(300, easing = BeszelMotion.emphasizedDecelerate)) { it / 4 } +
                                fadeIn(tween(300))
                        },
                        exitTransition = { fadeOut(tween(160)) },
                        popEnterTransition = { fadeIn(tween(220)) },
                        popExitTransition = {
                            slideOutHorizontally(tween(220, easing = BeszelMotion.emphasizedAccelerate)) { it / 4 } +
                                fadeOut(tween(220))
                        },
                    ) { entry ->
                        val systemId = entry.arguments?.getString("systemId").orEmpty()
                        val system = state.systems.firstOrNull { it.id == systemId }
                        val detailViewModel: SystemDetailViewModel = viewModel(
                            factory = SystemDetailViewModel.Factory(viewModel.repository, systemId),
                        )
                        val detailUiState by detailViewModel.state.collectAsStateWithLifecycle()
                        SystemDetailScreen(
                            system = system,
                            detailState = detailUiState,
                            onBack = { navController.popBackStack() },
                            onRange = detailViewModel::selectRange,
                            onRetryStats = detailViewModel::retry,
                            onRefresh = viewModel::refresh,
                        )
                    }
                }
            }
        }
    }
}

private fun NavHostController.navigateToTopLevel(destination: TopLevelDestination) {
    navigate(destination.route) {
        popUpTo(graph.findStartDestination().id) { saveState = true }
        launchSingleTop = true
        restoreState = true
    }
}

@Composable
private fun AppNavigationBar(
    selectedRoute: String?,
    alertCount: Int,
    onSelect: (TopLevelDestination) -> Unit,
) {
    NavigationBar(containerColor = MaterialTheme.colorScheme.surfaceContainer) {
        TopLevelDestination.entries.forEach { destination ->
            NavigationBarItem(
                selected = selectedRoute == destination.route,
                onClick = { onSelect(destination) },
                icon = { DestinationIcon(destination, alertCount) },
                label = { Text(destination.label) },
            )
        }
    }
}

@Composable
private fun AppNavigationRail(
    selectedRoute: String?,
    alertCount: Int,
    onSelect: (TopLevelDestination) -> Unit,
) {
    NavigationRail(
        modifier = Modifier
            .fillMaxHeight()
            .windowInsetsPadding(WindowInsets.safeDrawing.only(WindowInsetsSides.Vertical)),
        containerColor = MaterialTheme.colorScheme.surfaceContainer,
    ) {
        TopLevelDestination.entries.forEach { destination ->
            NavigationRailItem(
                selected = selectedRoute == destination.route,
                onClick = { onSelect(destination) },
                icon = { DestinationIcon(destination, alertCount) },
                label = { Text(destination.label) },
            )
        }
    }
}

@Composable
private fun DestinationIcon(destination: TopLevelDestination, alertCount: Int) {
    if (destination == TopLevelDestination.ALERTS && alertCount > 0) {
        BadgedBox(badge = { Badge { Text(alertCount.coerceAtMost(99).toString()) } }) {
            Icon(destination.icon, contentDescription = destination.label)
        }
    } else {
        Icon(destination.icon, contentDescription = destination.label)
    }
}
