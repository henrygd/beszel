package alerts

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// handleSmartDeviceAlert sends alerts when a SMART device state changes from PASSED to FAILED.
// This is automatic and does not require user opt-in.
func (am *AlertManager) handleSmartDeviceAlert(e *core.RecordEvent) error {
	oldState := e.Record.Original().GetString("state")
	newState := e.Record.GetString("state")

	// Only alert when transitioning from PASSED to FAILED
	if oldState != "PASSED" || newState != "FAILED" {
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

	// Build alert message
	title := fmt.Sprintf("SMART failure on %s: %s \U0001F534", systemName, deviceName)
	var message string
	if model != "" {
		message = fmt.Sprintf("Disk %s (%s) SMART status changed to FAILED", deviceName, model)
	} else {
		message = fmt.Sprintf("Disk %s SMART status changed to FAILED", deviceName)
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

