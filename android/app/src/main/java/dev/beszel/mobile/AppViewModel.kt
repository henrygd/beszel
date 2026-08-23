package dev.beszel.mobile

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import dev.beszel.mobile.data.AlertHistoryRecord
import dev.beszel.mobile.data.AlertRecord
import dev.beszel.mobile.data.ApiException
import dev.beszel.mobile.data.HubRepository
import dev.beszel.mobile.data.Session
import dev.beszel.mobile.data.SessionStore
import dev.beszel.mobile.data.SystemRecord
import dev.beszel.mobile.data.ThemeMode
import dev.beszel.mobile.data.computeFleetPulse
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
    val dynamicColor: Boolean = false,
    /** Fleet-wide CPU samples, one per poll, oldest first. Feeds the pulse header. */
    val pulse: List<Float> = emptyList(),
) {
    val activeAlerts get() = alerts.filter(AlertRecord::triggered)
    val isLoadingFleet get() = session != null && systems.isEmpty() && isStarting
}

private const val PULSE_WINDOW = 48

class AppViewModel(application: Application) : AndroidViewModel(application) {
    private val store = SessionStore(application)
    val repository = HubRepository()

    private val mutableState = MutableStateFlow(
        AppUiState(themeMode = store.themeMode, dynamicColor = store.dynamicColor),
    )
    val state: StateFlow<AppUiState> = mutableState.asStateFlow()

    private var pollingJob: Job? = null
    private val pulseBuffer = ArrayDeque<Float>()

    init {
        restoreSession()
    }

    private fun restoreSession() {
        val session = store.loadSession()
        if (session == null) {
            mutableState.update { it.copy(isStarting = false) }
            return
        }
        repository.attach(session)
        mutableState.update { it.copy(session = session) }
        viewModelScope.launch {
            try {
                val refreshedToken = repository.refreshAuth()
                val refreshed = repository.updateToken(refreshedToken)
                store.saveSession(refreshed)
                mutableState.update { it.copy(session = refreshed) }
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
                val session = repository.login(hubUrl, email, password)
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
        if (!repository.hasSession) return
        if (mutableState.value.isRefreshing) return
        mutableState.update { it.copy(isRefreshing = showSpinner, message = null) }
        try {
            val dashboard = repository.dashboard()
            val pulse = computeFleetPulse(dashboard.systems)
            if (pulse != null) {
                pulseBuffer.addLast(pulse)
                while (pulseBuffer.size > PULSE_WINDOW) pulseBuffer.removeFirst()
            }
            mutableState.update {
                it.copy(
                    isStarting = false,
                    isRefreshing = false,
                    systems = dashboard.systems,
                    alerts = dashboard.alerts,
                    alertHistory = dashboard.history,
                    lastUpdated = System.currentTimeMillis(),
                    pulse = pulseBuffer.toList(),
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
        repository.detach()
        pulseBuffer.clear()
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
