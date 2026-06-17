//go:build testing

package alerts

import (
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

func NewTestAlertManagerWithoutWorker(app hubLike) *AlertManager {
	return &AlertManager{
		hub:         app,
		alertsCache: NewAlertsCache(app),
	}
}

// GetSystemAlertsCache returns the internal system alerts cache.
func (am *AlertManager) GetSystemAlertsCache() *AlertsCache {
	return am.alertsCache
}

func (am *AlertManager) GetAlertManager() *AlertManager {
	return am
}

func (am *AlertManager) GetPendingAlerts() *sync.Map {
	return &am.pendingAlerts
}

func (am *AlertManager) GetPendingAlertsCount() int {
	count := 0
	am.pendingAlerts.Range(func(key, value any) bool {
		count++
		return true
	})
	return count
}

// ProcessPendingAlerts manually processes all expired alerts (for testing)
func (am *AlertManager) ProcessPendingAlerts() ([]CachedAlertData, error) {
	now := time.Now()
	var lastErr error
	var processedAlerts []CachedAlertData
	am.pendingAlerts.Range(func(key, value any) bool {
		info := value.(*alertInfo)
		if now.After(info.expireTime) {
			if info.timer != nil {
				info.timer.Stop()
			}
			am.processPendingAlert(key.(string))
			processedAlerts = append(processedAlerts, info.alertData)
		}
		return true
	})
	return processedAlerts, lastErr
}

// ForceExpirePendingAlerts sets all pending alerts to expire immediately (for testing)
func (am *AlertManager) ForceExpirePendingAlerts() {
	now := time.Now()
	am.pendingAlerts.Range(func(key, value any) bool {
		info := value.(*alertInfo)
		info.expireTime = now.Add(-time.Second) // Set to 1 second ago
		return true
	})
}

func (am *AlertManager) ResetPendingAlertTimer(alertID string, delay time.Duration) bool {
	value, loaded := am.pendingAlerts.Load(alertID)
	if !loaded {
		return false
	}

	info := value.(*alertInfo)
	if info.timer != nil {
		info.timer.Stop()
	}
	info.expireTime = time.Now().Add(delay)
	info.timer = time.AfterFunc(delay, func() {
		am.processPendingAlert(alertID)
	})
	return true
}

func ResolveStatusAlerts(app core.App) error {
	return resolveStatusAlerts(app)
}

func (am *AlertManager) RestorePendingStatusAlerts() error {
	return am.restorePendingStatusAlerts()
}

func (am *AlertManager) SetAlertTriggered(alert CachedAlertData, triggered bool) error {
	return am.setAlertTriggered(alert, triggered)
}

func IsInternalURL(rawURL string) (bool, error) {
	return isInternalURL(rawURL)
}
