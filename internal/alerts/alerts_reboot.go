package alerts

import (
	"fmt"

	"github.com/henrygd/beszel/internal/entities/system"

	"github.com/pocketbase/pocketbase/core"
)

// alertNameRebootRequired is the alerts.name value for the reboot required alert.
const alertNameRebootRequired = "RebootRequired"

// HandleRebootAlerts manages alerts for systems that need a reboot to finish
// applying updates.
//
// This is a binary state alert and fires on the first observation rather than
// using a delay. The agent checks during its package updates check (hourly by
// default), so a delay would only postpone the notification.
func (am *AlertManager) HandleRebootAlerts(systemRecord *core.Record, data *system.CombinedData) error {
	// No package updates counts means the agent has not finished a check yet (or
	// does not support it), so the reboot state is unknown. Without this, every
	// agent restart would send a recovery followed by a new alert.
	if data == nil || len(data.Info.PackageUpdates) == 0 {
		return nil
	}
	alerts := am.alertsCache.GetAlertsByName(systemRecord.Id, alertNameRebootRequired)
	if len(alerts) == 0 {
		return nil
	}

	triggered := data.Info.RebootRequired
	systemName := systemRecord.GetString("name")

	for _, alertData := range alerts {
		// Only notify on a change of state.
		if triggered == alertData.Triggered {
			continue
		}
		if err := am.sendRebootAlert(triggered, systemName, alertData); err != nil {
			am.hub.Logger().Error("Failed to send alert", "err", err)
		}
	}
	return nil
}

// sendRebootAlert sends a reboot required or resolved alert to the alert's user.
func (am *AlertManager) sendRebootAlert(triggered bool, systemName string, alertData CachedAlertData) error {
	// Update trigger state for alert record before sending alert
	if err := am.setAlertTriggered(alertData, triggered); err != nil {
		return err
	}

	var title, message string
	if triggered {
		title = fmt.Sprintf("Reboot required on %s %v", systemName, "\U0001F504") // Counterclockwise arrows emoji
		message = fmt.Sprintf("%s needs a reboot to finish applying updates.", systemName)
	} else {
		title = fmt.Sprintf("Reboot no longer required on %s %v", systemName, "✅") // Green checkmark emoji
		message = fmt.Sprintf("%s no longer needs a reboot.", systemName)
	}

	systemID := alertData.SystemID

	return am.SendAlert(AlertMessageData{
		UserID:   alertData.UserID,
		SystemID: systemID,
		Title:    title,
		Message:  message,
		Link:     am.hub.MakeLink("system", systemID),
		LinkText: "View " + systemName,
	})
}
