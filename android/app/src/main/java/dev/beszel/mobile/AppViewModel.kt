package dev.beszel.mobile

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import dev.beszel.mobile.data.AlertHistoryRecord
import dev.beszel.mobile.data.AlertRecord
import dev.beszel.mobile.data.ApiException
import dev.beszel.mobile.data.BeszelApi
import dev.beszel.mobile.data.ChartRange
import dev.beszel.mobile.data.Session
import dev.beszel.mobile.data.SessionStore
import dev.beszel.mobile.data.StatPoint
import dev.beszel.mobile.data.SystemRecord
import dev.beszel.mobile.data.ThemeMode
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

data class AppUiState(
    val isStarting: Boolean = true,
    val isLoggingIn: Boolean = false,
    val session: Session? = null,
    val systems: List<SystemRecord> = emptyList(),
    val alerts: List<AlertRecord> = emptyList(),
    val alertHistory: List<AlertHistoryRecord> = emptyList(),
    val isRefreshing: Boolean = false,
    val lastUpdated: Long? = null,
    val message: String? = null,
    val themeMode: ThemeMode = ThemeMode.SYSTEM,
    val dynamicColor: Boolean = true,
    val detailSystemId: String? = null,
    val chartRange: ChartRange = ChartRange.HOUR,
    val stats: List<StatPoint> = emptyList(),
    val statsLoading: Boolean = false,
    val statsError: String? = null,
) {
    val activeAlerts get() = alerts.filter(AlertRecord::triggered)
}

class AppViewModel(application: Application) : AndroidViewModel(application) {
    private val store = SessionStore(application)
    private val mutableState = MutableStateFlow(
        AppUiState(themeMode = store.themeMode, dynamicColor = store.dynamicColor),
    )
    val state: StateFlow<AppUiState> = mutableState.asStateFlow()

    private var api: BeszelApi? = null
    private var pollingJob: Job? = null
    private var statsJob: Job? = null

    init {
        restoreSession()
    }

    private fun restoreSession() {
        val session = store.loadSession()
        if (session == null) {
            mutableState.update { it.copy(isStarting = false) }
            return
        }
        mutableState.update { it.copy(session = session) }
        api = BeszelApi(session.hubUrl, session.token)
        viewModelScope.launch {
            try {
                val refreshedToken = api!!.refreshAuth()
                val refreshedSession = session.copy(token = refreshedToken)
                store.saveSession(refreshedSession)
                mutableState.update { it.copy(session = refreshedSession) }
                loadDashboard(showSpinner = false)
                startPolling()
            } catch (error: ApiException) {
                if (error.statusCode == 401 || error.statusCode == 403) clearSession()
                else mutableState.update { it.copy(isStarting = false, message = friendlyMessage(error)) }
            } catch (error: Exception) {
                mutableState.update { it.copy(isStarting = false, message = friendlyMessage(error)) }
                startPolling()
            }
        }
    }

    fun login(hubUrl: String, email: String, password: String) {
        if (mutableState.value.isLoggingIn) return
        mutableState.update { it.copy(isLoggingIn = true, message = null) }
        viewModelScope.launch {
            try {
                val client = BeszelApi(hubUrl)
                val session = client.login(email, password)
                api = client
                store.saveSession(session)
                mutableState.update { it.copy(session = session, isLoggingIn = false, isStarting = false) }
                loadDashboard(showSpinner = true)
                startPolling()
            } catch (error: Exception) {
                mutableState.update {
                    it.copy(isLoggingIn = false, isStarting = false, message = friendlyMessage(error))
                }
            }
        }
    }

    fun refresh() {
        viewModelScope.launch { loadDashboard(showSpinner = true) }
    }

    private suspend fun loadDashboard(showSpinner: Boolean) {
        val client = api ?: return
        if (mutableState.value.isRefreshing) return
        mutableState.update { it.copy(isRefreshing = showSpinner, message = null) }
        try {
            val dashboard = client.dashboard()
            mutableState.update {
                it.copy(
                    isStarting = false,
                    isRefreshing = false,
                    systems = dashboard.systems,
                    alerts = dashboard.alerts,
                    alertHistory = dashboard.history,
                    lastUpdated = System.currentTimeMillis(),
                )
            }
        } catch (error: ApiException) {
            if (error.statusCode == 401 || error.statusCode == 403) {
                clearSession("Please sign in again")
            } else {
                mutableState.update { it.copy(isStarting = false, isRefreshing = false, message = friendlyMessage(error)) }
            }
        } catch (error: Exception) {
            if (error is CancellationException) throw error
            mutableState.update { it.copy(isStarting = false, isRefreshing = false, message = friendlyMessage(error)) }
        }
    }

    fun openSystem(systemId: String) {
        mutableState.update {
            it.copy(
                detailSystemId = systemId,
                chartRange = ChartRange.HOUR,
                stats = emptyList(),
                statsError = null,
            )
        }
        loadStats(systemId, ChartRange.HOUR)
    }

    fun closeSystem() {
        statsJob?.cancel()
        mutableState.update { it.copy(detailSystemId = null, stats = emptyList(), statsLoading = false) }
    }

    fun selectChartRange(range: ChartRange) {
        val id = mutableState.value.detailSystemId ?: return
        mutableState.update { it.copy(chartRange = range) }
        loadStats(id, range)
    }

    private fun loadStats(systemId: String, range: ChartRange) {
        statsJob?.cancel()
        statsJob = viewModelScope.launch {
            mutableState.update { it.copy(statsLoading = true, statsError = null) }
            try {
                val result = api?.stats(systemId, range).orEmpty()
                if (mutableState.value.detailSystemId == systemId && mutableState.value.chartRange == range) {
                    mutableState.update { it.copy(stats = result, statsLoading = false) }
                }
            } catch (error: Exception) {
                if (error is CancellationException) throw error
                mutableState.update { it.copy(statsLoading = false, statsError = friendlyMessage(error)) }
            }
        }
    }

    fun setThemeMode(mode: ThemeMode) {
        store.themeMode = mode
        mutableState.update { it.copy(themeMode = mode) }
    }

    fun setDynamicColor(enabled: Boolean) {
        store.dynamicColor = enabled
        mutableState.update { it.copy(dynamicColor = enabled) }
    }

    fun clearMessage() = mutableState.update { it.copy(message = null) }

    fun logout() = clearSession()

    private fun clearSession(message: String? = null) {
        pollingJob?.cancel()
        statsJob?.cancel()
        api = null
        store.clearSession()
        mutableState.update {
            AppUiState(
                isStarting = false,
                message = message,
                themeMode = it.themeMode,
                dynamicColor = it.dynamicColor,
            )
        }
    }

    private fun startPolling() {
        pollingJob?.cancel()
        pollingJob = viewModelScope.launch {
            while (isActive) {
                delay(15_000)
                loadDashboard(showSpinner = false)
            }
        }
    }

    private fun friendlyMessage(error: Throwable): String {
        val raw = error.message.orEmpty()
        return when {
            raw.contains("Failed to connect", ignoreCase = true) ||
                raw.contains("Unable to resolve host", ignoreCase = true) -> "Could not reach this Beszel hub"
            raw.contains("trust anchor", ignoreCase = true) || raw.contains("certificate", ignoreCase = true) ->
                "The hub's HTTPS certificate could not be verified"
            raw.isBlank() -> "Something went wrong"
            else -> raw
        }
    }
}
