package alerts

import (
	"database/sql"
	"errors"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

func (am *AlertManager) registerGlobalAlertHooks() {
	am.hub.OnRecordAfterCreateSuccess("global_alerts").BindFunc(am.onGlobalAlertChange)
	am.hub.OnRecordAfterUpdateSuccess("global_alerts").BindFunc(am.onGlobalAlertChange)
	am.hub.OnRecordAfterDeleteSuccess("global_alerts").BindFunc(am.onGlobalAlertDelete)
	am.hub.OnRecordAfterCreateSuccess("systems").BindFunc(am.applyGlobalAlertsToNewSystem)
}

// onGlobalAlertChange syncs a created or updated global alert to all non-excluded systems.
// For updates, systems that were newly added to the exclusion list have their per-system alert deleted.
func (am *AlertManager) onGlobalAlertChange(e *core.RecordEvent) error {
	alertName := e.Record.GetString("name")
	value := e.Record.GetFloat("value")
	min := e.Record.GetInt("min")
	excluded := sliceToSet(e.Record.GetStringSlice("excluded_systems"))

	// On update, delete per-system alerts for systems that were newly excluded
	oldExcluded := sliceToSet(e.Record.Original().GetStringSlice("excluded_systems"))
	for systemID := range excluded {
		if !oldExcluded[systemID] {
			// newly excluded — remove their alert
			am.deletePerSystemAlert(e.App, systemID, alertName)
		}
	}

	systems, err := e.App.FindAllRecords("systems")
	if err != nil {
		e.App.Logger().Error("Global alert sync: failed to fetch systems", "err", err)
		return e.Next()
	}

	alertsCollection, err := e.App.FindCachedCollectionByNameOrId("alerts")
	if err != nil {
		e.App.Logger().Error("Global alert sync: failed to fetch alerts collection", "err", err)
		return e.Next()
	}

	for _, system := range systems {
		if excluded[system.Id] {
			continue
		}
		alertRecord, err := e.App.FindFirstRecordByFilter(alertsCollection,
			"system={:system} && name={:name}",
			dbx.Params{"system": system.Id, "name": alertName})

		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			e.App.Logger().Error("Global alert sync: failed to find alert", "err", err, "system", system.Id)
			continue
		}
		if alertRecord == nil {
			alertRecord = core.NewRecord(alertsCollection)
			alertRecord.Set("system", system.Id)
			alertRecord.Set("name", alertName)
		}
		alertRecord.Set("value", value)
		alertRecord.Set("min", min)
		if err := e.App.SaveNoValidate(alertRecord); err != nil {
			e.App.Logger().Error("Global alert sync: failed to save alert", "err", err, "system", system.Id)
		}
	}

	return e.Next()
}

// onGlobalAlertDelete removes all per-system alerts matching the deleted global alert name.
func (am *AlertManager) onGlobalAlertDelete(e *core.RecordEvent) error {
	alertName := e.Record.GetString("name")
	if _, err := e.App.DB().NewQuery("DELETE FROM alerts WHERE name={:name}").
		Bind(dbx.Params{"name": alertName}).Execute(); err != nil {
		e.App.Logger().Error("Global alert delete: failed to bulk delete", "err", err)
	}
	return e.Next()
}

// applyGlobalAlertsToNewSystem copies all active global alerts to a newly created system,
// skipping any global alerts that explicitly exclude this system.
func (am *AlertManager) applyGlobalAlertsToNewSystem(e *core.RecordEvent) error {
	systemID := e.Record.Id

	globalAlerts, err := e.App.FindAllRecords("global_alerts")
	if err != nil || len(globalAlerts) == 0 {
		return e.Next()
	}

	alertsCollection, err := e.App.FindCachedCollectionByNameOrId("alerts")
	if err != nil {
		e.App.Logger().Error("Global alert apply: failed to fetch alerts collection", "err", err)
		return e.Next()
	}

	for _, ga := range globalAlerts {
		if sliceToSet(ga.GetStringSlice("excluded_systems"))[systemID] {
			continue
		}
		alertRecord := core.NewRecord(alertsCollection)
		alertRecord.Set("system", systemID)
		alertRecord.Set("name", ga.GetString("name"))
		alertRecord.Set("value", ga.GetFloat("value"))
		alertRecord.Set("min", ga.GetInt("min"))
		if err := e.App.SaveNoValidate(alertRecord); err != nil {
			e.App.Logger().Error("Global alert apply: failed to create alert for new system",
				"err", err, "system", systemID, "name", ga.GetString("name"))
		}
	}

	return e.Next()
}

func (am *AlertManager) deletePerSystemAlert(app core.App, systemID, alertName string) {
	record, err := app.FindFirstRecordByFilter("alerts",
		"system={:system} && name={:name}",
		dbx.Params{"system": systemID, "name": alertName})
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			app.Logger().Error("Global alert: failed to find per-system alert", "err", err, "system", systemID)
		}
		return
	}
	if err := app.Delete(record); err != nil {
		app.Logger().Error("Global alert: failed to delete per-system alert", "err", err, "system", systemID)
	}
}

func sliceToSet(s []string) map[string]bool {
	m := make(map[string]bool, len(s))
	for _, v := range s {
		m[v] = true
	}
	return m
}

