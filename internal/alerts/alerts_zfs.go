package alerts

import (
	"fmt"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// handleZfsPoolAlert sends alerts when a ZFS pool health state worsens and
// resolves the alert history entry when the pool recovers. Like the SMART
// hook, this is automatic and does not require user opt-in.
func (am *AlertManager) handleZfsPoolAlert(e *core.RecordEvent) error {
	return am.handleZfsPoolHealthAlert(e, e.Record.Original().GetString("health"))
}

func (am *AlertManager) handleZfsPoolCreateAlert(e *core.RecordEvent) error {
	return am.handleZfsPoolHealthAlert(e, "")
}

func (am *AlertManager) handleZfsPoolHealthAlert(e *core.RecordEvent, oldHealth string) error {
	newHealth := e.Record.GetString("health")
	oldSeverity := zfsPoolSeverity(oldHealth)
	newSeverity := zfsPoolSeverity(newHealth)

	systemID := e.Record.GetString("system")
	if systemID == "" {
		return e.Next()
	}

	systemRecord, err := e.App.FindRecordById("systems", systemID)
	if err != nil {
		e.App.Logger().Error("Failed to find system for ZFS alert", "err", err, "systemID", systemID)
		return e.Next()
	}

	// Pool recovered to a healthy state: resolve any open history entries.
	if newSeverity == 1 && oldSeverity > 1 {
		resolveAllAlertHistoryRecords(e.App, e.Record.Id)
		return e.Next()
	}

	if !shouldSendZfsPoolAlert(oldSeverity, newSeverity) {
		return e.Next()
	}

	systemName := systemRecord.GetString("name")
	poolName := e.Record.GetString("name")

	title := fmt.Sprintf("ZFS pool %s on %s: %s", newHealth, systemName, poolName)
	message := fmt.Sprintf("ZFS pool %s (%s) was first observed as %s", poolName, systemName, newHealth)
	if oldSeverity > 0 {
		message = fmt.Sprintf("ZFS pool %s (%s) health changed from %s to %s", poolName, systemName, oldHealth, newHealth)
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
			e.App.Logger().Error("Failed to send ZFS alert", "err", err, "userID", userID)
		}
		_ = createZfsPoolHistoryRecord(e.App, userID, systemID, e.Record.Id, poolName)
	}

	return e.Next()
}

// resolveZfsPoolHistoryOnDelete resolves open alert history entries when a
// pool record is deleted (manually or because the pool disappeared), so the
// UI does not keep showing an ongoing alert for a pool that no longer exists.
func resolveZfsPoolHistoryOnDelete(e *core.RecordEvent) error {
	resolveAllAlertHistoryRecords(e.App, e.Record.Id)
	return e.Next()
}

// shouldSendZfsPoolAlert reports whether a health transition warrants an alert.
// First observations of unhealthy pools and worsening transitions are reported.
func shouldSendZfsPoolAlert(oldSeverity, newSeverity int) bool {
	return newSeverity > 1 && (oldSeverity == 0 || newSeverity > oldSeverity)
}

// zfsPoolSeverity ranks pool health states: healthy (1), degraded (2),
// failed/unavailable (3), unknown (0).
func zfsPoolSeverity(health string) int {
	switch health {
	case "ONLINE":
		return 1
	case "DEGRADED":
		return 2
	case "FAULTED", "OFFLINE", "UNAVAIL", "REMOVED", "SUSPENDED":
		return 3
	default:
		return 0
	}
}

// createZfsPoolHistoryRecord logs a pool health alert in the alerts history so
// it is visible in the UI without creating an editable alert configuration.
func createZfsPoolHistoryRecord(app core.App, userID, systemID, alertID, poolName string) error {
	collection, err := app.FindCachedCollectionByNameOrId("alerts_history")
	if err != nil {
		return err
	}
	record := core.NewRecord(collection)
	record.Set("user", userID)
	record.Set("system", systemID)
	record.Set("alert_id", alertID)
	record.Set("name", "ZFS Pool: "+poolName)
	return app.Save(record)
}

// resolveAllAlertHistoryRecords resolves every open history entry for an alert
// record id (one per system user).
func resolveAllAlertHistoryRecords(app core.App, alertID string) {
	records, err := app.FindRecordsByFilter(
		"alerts_history",
		"alert_id={:alert_id} && resolved=null",
		"", 0, 0,
		dbx.Params{"alert_id": alertID},
	)
	if err != nil || len(records) == 0 {
		return
	}
	now := time.Now().UTC()
	for _, record := range records {
		record.Set("resolved", now)
		if err := app.Save(record); err != nil {
			app.Logger().Error("Failed to resolve ZFS alert history", "err", err, "recordId", record.Id)
		}
	}
}
