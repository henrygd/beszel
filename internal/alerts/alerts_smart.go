package alerts

import (
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// handleSmartDeviceAlert sends alerts when a SMART device state worsens into WARNING/FAILED.
// This is automatic and does not require user opt-in.
func (am *AlertManager) handleSmartDeviceAlert(e *core.RecordEvent) error {
	oldState := e.Record.Original().GetString("state")
	newState := e.Record.GetString("state")

	if !shouldSendSmartDeviceAlert(oldState, newState) {
		return e.Next()
	}

	systemID := e.Record.GetString("system")
	if systemID == "" {
		return e.Next()
	}

	// Fetch the system record to get the name and users
	systemRecord, err := e.App.FindRecordById("systems", systemID)
	if err != nil {
		e.App.Logger().Error("Failed to find system for SMART alert", "err", err, "systemID", systemID)
		return e.Next()
	}

	systemName := systemRecord.GetString("name")
	deviceName := e.Record.GetString("name")
	model := e.Record.GetString("model")
	statusLabel := smartStateLabel(newState)

	// Build alert message
	title := fmt.Sprintf("SMART %s on %s: %s %s", statusLabel, systemName, deviceName, smartStateEmoji(newState))
	var message string
	if model != "" {
		message = fmt.Sprintf("Disk %s (%s) SMART status changed to %s", deviceName, model, newState)
	} else {
		message = fmt.Sprintf("Disk %s SMART status changed to %s", deviceName, newState)
	}

	// Get users associated with the system
	userIDs := systemRecord.GetStringSlice("users")
	if len(userIDs) == 0 {
		return e.Next()
	}

	// Send alert to each user
	for _, userID := range userIDs {
		if err := am.SendAlert(AlertMessageData{
			UserID:   userID,
			SystemID: systemID,
			Title:    title,
			Message:  message,
			Link:     am.hub.MakeLink("system", systemID),
			LinkText: "View " + systemName,
		}); err != nil {
			e.App.Logger().Error("Failed to send SMART alert", "err", err, "userID", userID)
		}
	}

	return e.Next()
}

func shouldSendSmartDeviceAlert(oldState, newState string) bool {
	oldSeverity := smartStateSeverity(oldState)
	newSeverity := smartStateSeverity(newState)

	// Ignore unknown states and recoveries; only alert on worsening transitions
	// from known-good/degraded states into WARNING/FAILED.
	return oldSeverity >= 1 && newSeverity > oldSeverity
}

func smartStateSeverity(state string) int {
	switch state {
	case "PASSED":
		return 1
	case "WARNING":
		return 2
	case "FAILED":
		return 3
	default:
		return 0
	}
}

func smartStateEmoji(state string) string {
	switch state {
	case "WARNING":
		return "\U0001F7E0"
	default:
		return "\U0001F534"
	}
}

func smartStateLabel(state string) string {
	switch state {
	case "FAILED":
		return "failure"
	default:
		return strings.ToLower(state)
	}
}
