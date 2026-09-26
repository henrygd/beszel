package alerts

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// handleNutDeviceAlert sends alerts when a NUT device health worsens.
// This is automatic and does not require user opt-in.
func (am *AlertManager) handleNutDeviceAlert(e *core.RecordEvent) error {
	oldHealth := e.Record.Original().GetString("state")
	newHealth := e.Record.GetString("state")

	if !shouldSendNutDeviceAlert(oldHealth, newHealth) {
		return e.Next()
	}

	systemID := e.Record.GetString("system")
	if systemID == "" {
		return e.Next()
	}

	systemRecord, err := e.App.FindRecordById("systems", systemID)
	if err != nil {
		e.App.Logger().Error("Failed to find system for NUT alert", "err", err, "systemID", systemID)
		return e.Next()
	}

	systemName := systemRecord.GetString("name")
	deviceName := e.Record.GetString("name")
	model := e.Record.GetString("model")
	healthLabel := nutHealthLabel(newHealth)

	title := fmt.Sprintf("NUT %s on %s: %s %s", healthLabel, systemName, deviceName, nutHealthEmoji(newHealth))
	var message string
	if model != "" {
		message = fmt.Sprintf("Device %s (%s) status changed to %s", deviceName, model, newHealth)
	} else {
		message = fmt.Sprintf("Device %s status changed to %s", deviceName, newHealth)
	}

	userIDs := systemRecord.GetStringSlice("users")
	if len(userIDs) == 0 {
		return e.Next()
	}

	for _, userID := range userIDs {
		if err := am.SendAlert(AlertMessageData{
			UserID:   userID,
			SystemID: systemID,
			Title:    title,
			Message:  message,
			Link:     am.hub.MakeLink("system", systemID),
			LinkText: "View " + systemName,
		}); err != nil {
			e.App.Logger().Error("Failed to send NUT alert", "err", err, "userID", userID)
		}
	}

	return e.Next()
}

func shouldSendNutDeviceAlert(oldHealth, newHealth string) bool {
	oldSeverity := nutHealthSeverity(oldHealth)
	newSeverity := nutHealthSeverity(newHealth)

	return oldSeverity >= 1 && newSeverity > oldSeverity
}

func nutHealthSeverity(health string) int {
	switch health {
	case "ONLINE":
		return 1
	case "ON_BATTERY":
		return 2
	case "LOW_BATTERY", "OVERLOAD":
		return 3
	case "FAULT":
		return 4
	default:
		return 0
	}
}

func nutHealthEmoji(health string) string {
	switch health {
	case "ON_BATTERY":
		return "\U0001F50B"
	case "LOW_BATTERY":
		return "\U0001F50A"
	case "OVERLOAD":
		return "\U0001F4A5"
	default:
		return "\U0001F534"
	}
}

func nutHealthLabel(health string) string {
	switch health {
	case "ON_BATTERY":
		return "on battery"
	case "LOW_BATTERY":
		return "low battery"
	case "OVERLOAD":
		return "overload"
	case "FAULT":
		return "fault"
	default:
		return health
	}
}