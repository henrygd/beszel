package alerts

import (
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// On triggered alert record delete, set matching alert history record to resolved
func resolveHistoryOnAlertDelete(e *core.RecordEvent) error {
	if !e.Record.GetBool("triggered") {
		return e.Next()
	}
	_ = resolveAlertHistoryRecord(e.App, e.Record.Id)
	return e.Next()
}

// On alert record update, update alert history record
func updateHistoryOnAlertUpdate(e *core.RecordEvent) error {
	original := e.Record.Original()
	new := e.Record

	originalTriggered := original.GetBool("triggered")
	newTriggered := new.GetBool("triggered")

	// no need to update alert history if triggered state has not changed
	if originalTriggered == newTriggered {
		return e.Next()
	}

	// if new state is triggered, create new alert history record
	if newTriggered {
		_, _ = createAlertHistoryRecord(e.App, new)
		return e.Next()
	}

	// if new state is not triggered, check for matching alert history record and set it to resolved
	_ = resolveAlertHistoryRecord(e.App, new.Id)
	return e.Next()
}

// resolveAlertHistoryRecord sets the resolved field to the current time
func resolveAlertHistoryRecord(app core.App, alertRecordID string) error {
	alertHistoryRecord, err := app.FindFirstRecordByFilter("alerts_history", "alert_id={:alert_id} && resolved=null", dbx.Params{"alert_id": alertRecordID})
	if err != nil || alertHistoryRecord == nil {
		return err
	}
	alertHistoryRecord.Set("resolved", time.Now().UTC())
	err = app.Save(alertHistoryRecord)
	if err != nil {
		app.Logger().Error("Failed to resolve alert history", "err", err)
	}
	return err
}

// createAlertHistoryRecord creates a new alert history record
func createAlertHistoryRecord(app core.App, alertRecord *core.Record) (alertHistoryRecord *core.Record, err error) {
	alertHistoryCollection, err := app.FindCachedCollectionByNameOrId("alerts_history")
	if err != nil {
		return nil, err
	}
	alertHistoryRecord = core.NewRecord(alertHistoryCollection)
	alertHistoryRecord.Set("alert_id", alertRecord.Id)
	alertHistoryRecord.Set("user", alertRecord.GetString("user"))
	alertHistoryRecord.Set("system", alertRecord.GetString("system"))
	alertHistoryRecord.Set("name", alertRecord.GetString("name"))
	alertHistoryRecord.Set("value", alertRecord.GetFloat("value"))
	err = app.Save(alertHistoryRecord)
	if err != nil {
		app.Logger().Error("Failed to save alert history", "err", err)
	}
	return alertHistoryRecord, err
}

// On triggered container alert record delete, set matching alert history record to resolved
func resolveHistoryOnContainerAlertDelete(e *core.RecordEvent) error {
	if !e.Record.GetBool("triggered") {
		return e.Next()
	}
	_ = resolveContainerAlertHistoryRecord(e.App, e.Record.Id)
	return e.Next()
}

// On container alert record update, update alert history record
func updateHistoryOnContainerAlertUpdate(e *core.RecordEvent) error {
	original := e.Record.Original()
	new := e.Record

	originalTriggered := original.GetBool("triggered")
	newTriggered := new.GetBool("triggered")

	// no need to update alert history if triggered state has not changed
	if originalTriggered == newTriggered {
		return e.Next()
	}

	// if new state is triggered, create new alert history record
	if newTriggered {
		_, _ = createContainerAlertHistoryRecord(e.App, new)
		return e.Next()
	}

	// if new state is not triggered, check for matching alert history record and set it to resolved
	_ = resolveContainerAlertHistoryRecord(e.App, new.Id)
	return e.Next()
}

// resolveContainerAlertHistoryRecord sets the resolved field to the current time
func resolveContainerAlertHistoryRecord(app core.App, alertRecordID string) error {
	alertHistoryRecord, err := app.FindFirstRecordByFilter("alerts_history", "alert_id={:alert_id} && resolved=null", dbx.Params{"alert_id": alertRecordID})
	if err != nil || alertHistoryRecord == nil {
		return err
	}
	alertHistoryRecord.Set("resolved", time.Now().UTC())
	err = app.Save(alertHistoryRecord)
	if err != nil {
		app.Logger().Error("Failed to resolve container alert history", "err", err)
	}
	return err
}

// createContainerAlertHistoryRecord creates a new container alert history record
func createContainerAlertHistoryRecord(app core.App, alertRecord *core.Record) (alertHistoryRecord *core.Record, err error) {
	alertHistoryCollection, err := app.FindCachedCollectionByNameOrId("alerts_history")
	if err != nil {
		return nil, err
	}
	alertHistoryRecord = core.NewRecord(alertHistoryCollection)
	alertHistoryRecord.Set("alert_id", alertRecord.Id)
	alertHistoryRecord.Set("user", alertRecord.GetString("user"))
	alertHistoryRecord.Set("system", alertRecord.GetString("system"))
	alertHistoryRecord.Set("container", alertRecord.GetString("container"))
	alertHistoryRecord.Set("name", alertRecord.GetString("name"))
	alertHistoryRecord.Set("value", alertRecord.GetFloat("value"))
	err = app.Save(alertHistoryRecord)
	if err != nil {
		app.Logger().Error("Failed to save container alert history", "err", err)
	}
	return alertHistoryRecord, err
}
