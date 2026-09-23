package alerts

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"
	"time"

	"github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

const alertNameNetworkMonitorLoss = "NetworkMonitorLoss"

// networkMonitorAlertState is this alert type's persisted runtime state.
// Monitor IDs map to their open history entries independently of history retention.
type networkMonitorAlertState struct {
	Monitors map[string]string `json:"monitors"`
}

func (am *AlertManager) bindNetworkMonitorAlertEvents() {
	// Hidden fields are still writable through the record API unless protected.
	protectState := func(e *core.RecordRequestEvent) error {
		e.Record.Set("state", e.Record.Original().Get("state"))
		oldName, newName := e.Record.Original().GetString("name"), e.Record.GetString("name")
		if oldName != "" && (oldName == alertNameNetworkMonitorLoss || newName == alertNameNetworkMonitorLoss) &&
			(oldName != newName || e.Record.GetString("system") != e.Record.Original().GetString("system")) {
			return e.BadRequestError("Delete and recreate the alert to change its type or system", nil)
		}
		if e.Record.GetString("name") == alertNameNetworkMonitorLoss {
			if !e.HasSuperuserAuth() && (e.Auth == nil || !userHasSystem(e.App, e.Auth.Id, e.Record.GetString("system"))) {
				return e.ForbiddenError("You do not have access to this system", nil)
			}
			e.Record.Set("triggered", e.Record.Original().GetBool("triggered"))
			value := e.Record.GetFloat("value")
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value >= 100 {
				return e.BadRequestError("Monitor loss threshold must be at least 0 and below 100", nil)
			}
			e.Record.Set("min", 0)
		}
		return e.Next()
	}
	am.hub.OnRecordCreateRequest("alerts").BindFunc(protectState)
	am.hub.OnRecordUpdateRequest("alerts").BindFunc(protectState)
	cleanup := func(e *core.RecordEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		return am.evaluateNetworkMonitorAlerts(e.App, e.Record.GetString("system"), nil)
	}
	am.hub.OnRecordAfterDeleteSuccess("network_monitors").BindFunc(cleanup)
	am.hub.OnRecordAfterUpdateSuccess("network_monitors").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetBool("enabled") || !e.Record.Original().GetBool("enabled") {
			return e.Next()
		}
		return cleanup(e)
	})
}

// HandleNetworkMonitorAlerts runs after the full monitoring transaction commits,
// using its exact payload (dashboard requests can replace the cached payload).
// Omitted results and disconnected systems never imply recovery.
func (am *AlertManager) HandleNetworkMonitorAlerts(systemRecord *core.Record, results map[string]monitor.Result) error {
	if systemRecord.GetString("status") != "up" {
		return nil
	}
	alerts := am.alertsCache.GetAlertsByName(systemRecord.Id, alertNameNetworkMonitorLoss)
	if len(alerts) == 0 {
		return nil
	}
	monitors, err := am.networkMonitors.get(systemRecord.Id)
	if err != nil {
		return err
	}
	if !networkMonitorTransitionPending(alerts, monitors, results, time.Now()) {
		return nil
	}
	// The cache only predicts a transition. Reload and recheck under the DB
	// transaction before persisting, including current system/monitor status.
	return am.evaluateNetworkMonitorAlerts(am.hub, systemRecord.Id, results)
}

// networkMonitorTransitionPending does no IO and never mutates cached maps.
func networkMonitorTransitionPending(alerts []CachedAlertData, monitors map[string]int, results map[string]monitor.Result, now time.Time) bool {
	for _, alert := range alerts {
		if !alert.MonitorStatesValid || alert.Triggered != (len(alert.MonitorStates) > 0) {
			return true
		}
		for id := range alert.MonitorStates {
			if _, enabled := monitors[id]; !enabled {
				return true
			}
		}
		for id, result := range results {
			interval, enabled := monitors[id]
			if !enabled || !monitorResultReady(result, interval, now) {
				continue
			}
			_, active := alert.MonitorStates[id]
			if (result.PacketLoss1h > alert.Value) != active {
				return true
			}
		}
	}
	return false
}

func (am *AlertManager) evaluateNetworkMonitorAlerts(app core.App, systemID string, results map[string]monitor.Result) error {
	var messages []AlertMessageData
	err := app.RunInTransaction(func(tx core.App) error {
		// Read configuration inside the transaction so concurrent threshold changes,
		// disabling, and evaluations cannot overwrite each other's incident state.
		alerts, err := tx.FindAllRecords("alerts", dbx.HashExp{"system": systemID, "name": alertNameNetworkMonitorLoss})
		if err != nil || len(alerts) == 0 {
			return err
		}
		system, err := tx.FindRecordById("systems", systemID)
		if errors.Is(err, sql.ErrNoRows) {
			// System deletion cascades to its alerts.
			return nil
		}
		if err != nil {
			return err
		}
		monitors, err := tx.FindAllRecords("network_monitors", dbx.HashExp{"system": systemID, "enabled": true})
		if err != nil {
			return err
		}
		enabled := make(map[string]*core.Record, len(monitors))
		for _, m := range monitors {
			enabled[m.Id] = m
		}
		now := time.Now()
		for _, alert := range alerts {
			var state networkMonitorAlertState
			if err := alert.UnmarshalJSONField("state", &state); err != nil {
				return err
			}
			states := state.Monitors
			if states == nil {
				states = map[string]string{}
			}
			changed := false
			// Removing or disabling a monitor closes its incident silently.
			for id, historyID := range states {
				if _, ok := enabled[id]; !ok {
					if err := resolveMonitorIncident(tx, historyID, now); err != nil {
						return err
					}
					delete(states, id)
					changed = true
				}
			}
			if system.GetString("status") == "up" {
				for _, m := range monitors {
					result, ok := results[m.Id]
					if !ok || !monitorResultReady(result, m.GetInt("interval"), now) {
						continue
					}
					historyID, active := states[m.Id]
					triggered := result.PacketLoss1h > alert.GetFloat("value")
					if triggered == active {
						continue
					}
					label := m.GetString("target")
					if m.GetString("protocol") == "tcp" {
						label = net.JoinHostPort(label, strconv.Itoa(m.GetInt("port")))
					}
					if triggered {
						collection, err := tx.FindCachedCollectionByNameOrId("alerts_history")
						if err != nil {
							return err
						}
						history := core.NewRecord(collection)
						history.Load(map[string]any{
							"alert_id": alert.Id, "system": systemID,
							"name": alertNameNetworkMonitorLoss, "monitor_name": label, "value": result.PacketLoss1h,
						})
						if err := tx.Save(history); err != nil {
							return err
						}
						states[m.Id] = history.Id
					} else {
						if err := resolveMonitorIncident(tx, historyID, now); err != nil {
							return err
						}
						delete(states, m.Id)
					}
					changed = true
					state, comparison := "loss", "exceeds"
					if !triggered {
						state, comparison = "recovered", "is at or below"
					}
					messages = append(messages, AlertMessageData{
						SystemID: systemID,
						Title:    fmt.Sprintf("Network monitor %s on %s: %s", state, system.GetString("name"), label),
						Message:  fmt.Sprintf("%s on %s: loss over the past hour is %.2f%%, which %s the %.2f%% threshold.", label, system.GetString("name"), result.PacketLoss1h, comparison, alert.GetFloat("value")),
						Link:     am.hub.MakeLink("system", systemID), LinkText: "View " + system.GetString("name"),
					})
				}
			}
			if changed || alert.GetBool("triggered") != (len(states) > 0) {
				alert.Set("state", networkMonitorAlertState{Monitors: states})
				alert.Set("triggered", len(states) > 0)
				if err := tx.Save(alert); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Match other alert types: persist transitions before delivery, and respect
	// the user's existing notification destinations and quiet hours.
	for _, message := range messages {
		if err := am.SendAlert(message); err != nil {
			app.Logger().Error("Failed to send network monitor alert", "err", err)
		}
	}
	return nil
}

func monitorResultReady(result monitor.Result, interval int, now time.Time) bool {
	// Three completed attempts provide a short warm-up, including after an agent
	// restart.
	if result.SampleCount < 3 || result.LastProbeAt <= 0 || math.IsNaN(result.PacketLoss1h) || math.IsInf(result.PacketLoss1h, 0) || result.PacketLoss1h < 0 || result.PacketLoss1h > 100 {
		return false
	}
	// Never interpret an empty one-hour window as zero loss.
	maxAge := min(time.Hour, max(3*time.Duration(interval)*time.Second, 3*time.Minute))
	age := now.Sub(time.UnixMilli(result.LastProbeAt))
	return age >= -time.Minute && age <= maxAge
}

func resolveMonitorIncident(app core.App, id string, now time.Time) error {
	record, err := app.FindRecordById("alerts_history", id)
	if errors.Is(err, sql.ErrNoRows) {
		// History can be purged independently.
		return nil
	}
	if err != nil {
		return err
	}
	if !record.GetDateTime("resolved").IsZero() {
		return nil
	}
	record.Set("resolved", now.UTC())
	return app.Save(record)
}

func resolveNetworkMonitorHistory(app core.App, alertID string) error {
	records, err := app.FindAllRecords("alerts_history", dbx.HashExp{"alert_id": alertID, "resolved": ""})
	if err != nil {
		return err
	}
	for _, record := range records {
		record.Set("resolved", time.Now().UTC())
		if err := app.Save(record); err != nil {
			return err
		}
	}
	return nil
}
