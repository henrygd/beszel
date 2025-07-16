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
