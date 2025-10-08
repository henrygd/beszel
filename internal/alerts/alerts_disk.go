package alerts

import (
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/pocketbase/pocketbase/core"
)

// handleDiskAlerts processes disk alerts for filesystem-specific alerts
func (am *AlertManager) handleDiskAlerts(systemRecord *core.Record, alertRecord *core.Record, data *system.CombinedData, now time.Time, oldestTime time.Time, validAlerts *[]SystemAlertData) {
	// This function is now deprecated since we handle per-filesystem alerts directly in the main loop
	// Keep for backward compatibility but it won't be called for new filesystem-specific alerts
}

