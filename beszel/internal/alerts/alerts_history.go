package alerts

import (
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

func (am *AlertManager) RecordAlertHistory(alert SystemAlertData) {
	// Get alert, user, system, name, value
	alertId := alert.alertRecord.Id
	userId := ""
	if errs := am.hub.ExpandRecord(alert.alertRecord, []string{"user"}, nil); len(errs) == 0 {
		if user := alert.alertRecord.ExpandedOne("user"); user != nil {
			userId = user.Id
		}
	}
	systemId := alert.systemRecord.Id
	name := alert.name
	value := alert.val
	now := time.Now().UTC()

	if alert.triggered {
		// Create new alerts_history record
		collection, err := am.hub.FindCollectionByNameOrId("alerts_history")
		if err == nil {
			history := core.NewRecord(collection)
			history.Set("alert", alertId)
			history.Set("user", userId)
			history.Set("system", systemId)
			history.Set("name", name)
			history.Set("value", value)
			history.Set("state", "active")
			history.Set("created_date", now)
			history.Set("solved_date", nil)
			_ = am.hub.Save(history)
		}
	} else {
		// Find latest active alerts_history record for this alert and set to solved
		record, err := am.hub.FindFirstRecordByFilter(
			"alerts_history",
			"alert={:alert} && state='active'",
			dbx.Params{"alert": alertId},
		)
		if err == nil && record != nil {
			record.Set("state", "solved")
			record.Set("solved_date", now)
			_ = am.hub.Save(record)
		}
	}
}

// DeleteOldAlertHistory deletes alerts_history records older than the given retention duration
func (am *AlertManager) DeleteOldAlertHistory(retention time.Duration) {
	now := time.Now().UTC()
	cutoff := now.Add(-retention)
	_, err := am.hub.DB().NewQuery(
		"DELETE FROM alerts_history WHERE solved_date IS NOT NULL AND solved_date < {:cutoff}",
	).Bind(dbx.Params{"cutoff": cutoff}).Execute()
	if err != nil {
		am.hub.Logger().Error("failed to delete old alerts_history records", "error", err)
	}
}

// Helper to get retention duration from user settings
func getAlertHistoryRetention(settings map[string]interface{}) time.Duration {
	retStr, _ := settings["alertHistoryRetention"].(string)
	switch retStr {
	case "1m":
		return 30 * 24 * time.Hour
	case "3m":
		return 90 * 24 * time.Hour
	case "6m":
		return 180 * 24 * time.Hour
	case "1y":
		return 365 * 24 * time.Hour
	default:
		return 90 * 24 * time.Hour // default 3 months
	}
}

// CleanUpAllAlertHistory deletes old alerts_history records for each user based on their retention setting
func (am *AlertManager) CleanUpAllAlertHistory() {
	records, err := am.hub.FindAllRecords("user_settings")
	if err != nil {
		return
	}
	for _, record := range records {
		var settings map[string]interface{}
		if err := record.UnmarshalJSONField("settings", &settings); err != nil {
			continue
		}
		am.DeleteOldAlertHistory(getAlertHistoryRetention(settings))
	}
}
