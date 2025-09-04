package alerts

import (
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

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
func (am *AlertManager) ProcessPendingAlerts() ([]*core.Record, error) {
	now := time.Now()
	var lastErr error
	var processedAlerts []*core.Record
	am.pendingAlerts.Range(func(key, value any) bool {
		info := value.(*alertInfo)
		if now.After(info.expireTime) {
			// Downtime delay has passed, process alert
			if err := am.sendStatusAlert("down", info.systemName, info.alertRecord); err != nil {
				lastErr = err
			}
			processedAlerts = append(processedAlerts, info.alertRecord)
			am.pendingAlerts.Delete(key)
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
