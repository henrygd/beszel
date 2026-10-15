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

const (
	alertNameNetworkMonitorLoss = "NetworkMonitorLoss"
	alertNameNetworkMonitorCert = "NetworkMonitorCert"
	// maxCertAlertDays is the largest certificate expiry threshold in days.
	maxCertAlertDays = 365
)

// networkMonitorAlertState is this alert type's persisted runtime state.
// Monitor IDs map to their open history entries independently of history retention.
type networkMonitorAlertState struct {
	Monitors map[string]string `json:"monitors"`
}

// monitorObservation is one monitor's current value for an alert threshold.
type monitorObservation struct {
	triggered bool
	// value is stored in alert history: loss percentage or days left.
	value float64
	// certExpires is the certificate expiry in Unix milliseconds (certificate alerts only).
	certExpires int64
}

// isNetworkMonitorAlert reports whether an alert tracks incidents per network monitor.
func isNetworkMonitorAlert(name string) bool {
	return name == alertNameNetworkMonitorLoss || name == alertNameNetworkMonitorCert
}

// networkMonitorThresholdError describes why a threshold is invalid, or returns "" if it is valid.
func networkMonitorThresholdError(name string, value float64) string {
	switch name {
	case alertNameNetworkMonitorLoss:
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value >= 100 {
			return "Monitor loss threshold must be at least 0 and below 100"
		}
	case alertNameNetworkMonitorCert:
		if math.IsNaN(value) || value < 1 || value > maxCertAlertDays || value != math.Trunc(value) {
			return fmt.Sprintf("Certificate expiry threshold must be a whole number of days from 1 to %d", maxCertAlertDays)
		}
	}
	return ""
}

func (am *AlertManager) bindNetworkMonitorAlertEvents() {
	// Hidden fields are still writable through the record API unless protected.
	protectState := func(e *core.RecordRequestEvent) error {
		e.Record.Set("state", e.Record.Original().Get("state"))
		oldName, newName := e.Record.Original().GetString("name"), e.Record.GetString("name")
		if oldName != "" && (isNetworkMonitorAlert(oldName) || isNetworkMonitorAlert(newName)) &&
			(oldName != newName || e.Record.GetString("system") != e.Record.Original().GetString("system")) {
			return e.BadRequestError("Delete and recreate the alert to change its type or system", nil)
		}
		if isNetworkMonitorAlert(newName) {
			e.Record.Set("triggered", e.Record.Original().GetBool("triggered"))
			if msg := networkMonitorThresholdError(newName, e.Record.GetFloat("value")); msg != "" {
				return e.BadRequestError(msg, nil)
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
	// Certificate info is stored by direct SQL without record hooks, so drop the
	// cached expiry times when a result carries a different expiry. The agent
	// resends unchanged info hourly for certificates that are about to expire.
	for id, result := range results {
		if result.Cert != nil && am.networkMonitors.certChanged(systemRecord.Id, id, result.Cert.Expires) {
			am.networkMonitors.invalidate(systemRecord.Id)
			break
		}
	}
	if systemRecord.GetString("status") != "up" {
		return nil
	}
	alerts := am.alertsCache.GetAlertsByName(systemRecord.Id, alertNameNetworkMonitorLoss)
	alerts = append(alerts, am.alertsCache.GetAlertsByName(systemRecord.Id, alertNameNetworkMonitorCert)...)
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
func networkMonitorTransitionPending(alerts []CachedAlertData, monitors map[string]cachedMonitor, results map[string]monitor.Result, now time.Time) bool {
	for _, alert := range alerts {
		if !alert.MonitorStatesValid || alert.Triggered != (len(alert.MonitorStates) > 0) {
			return true
		}
		for id := range alert.MonitorStates {
			if _, enabled := monitors[id]; !enabled {
				return true
			}
		}
		if alert.Name == alertNameNetworkMonitorCert {
			// Expiry depends on the clock, so check stored certificates rather than results.
			for id, m := range monitors {
				if m.certExpires <= 0 {
					continue
				}
				_, active := alert.MonitorStates[id]
				if certExpiresWithin(m.certExpires, alert.Value, now) != active {
					return true
				}
			}
			continue
		}
		for id, result := range results {
			m, enabled := monitors[id]
			if !enabled || !monitorResultReady(result, m.interval, now) {
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
		alerts, err := tx.FindAllRecords("alerts", dbx.HashExp{"system": systemID, "name": []any{alertNameNetworkMonitorLoss, alertNameNetworkMonitorCert}})
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
			if !userHasSystem(tx, alert.GetString("user"), systemID) {
				continue
			}
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
				name, threshold := alert.GetString("name"), alert.GetFloat("value")
				for _, m := range monitors {
					var obs monitorObservation
					var ok bool
					if name == alertNameNetworkMonitorCert {
						obs, ok = observeMonitorCert(m, threshold, now)
					} else {
						obs, ok = observeMonitorLoss(results[m.Id], m, threshold, now)
					}
					if !ok {
						continue
					}
					historyID, active := states[m.Id]
					if obs.triggered == active {
						continue
					}
					label := m.GetString("target")
					if m.GetString("protocol") == "tcp" {
						label = net.JoinHostPort(label, strconv.Itoa(m.GetInt("port")))
					}
					if obs.triggered {
						collection, err := tx.FindCachedCollectionByNameOrId("alerts_history")
						if err != nil {
							return err
						}
						history := core.NewRecord(collection)
						history.Load(map[string]any{
							"alert_id": alert.Id, "user": alert.GetString("user"), "system": systemID,
							"name": name, "monitor_name": label, "value": obs.value,
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
					title, message := networkMonitorAlertMessage(name, obs, label, system.GetString("name"), threshold)
					messages = append(messages, AlertMessageData{
						UserID: alert.GetString("user"), SystemID: systemID,
						Title: title, Message: message,
						Link: am.hub.MakeLink("system", systemID), LinkText: "View " + system.GetString("name"),
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

// observeMonitorLoss compares a fresh result's one hour loss with the threshold.
func observeMonitorLoss(result monitor.Result, m *core.Record, threshold float64, now time.Time) (monitorObservation, bool) {
	if !monitorResultReady(result, m.GetInt("interval"), now) {
		return monitorObservation{}, false
	}
	return monitorObservation{triggered: result.PacketLoss1h > threshold, value: result.PacketLoss1h}, true
}

// observeMonitorCert compares the stored certificate expiry with a threshold in days.
// Monitors without certificate info (non-HTTPS or never checked) are skipped.
func observeMonitorCert(m *core.Record, threshold float64, now time.Time) (monitorObservation, bool) {
	var cert monitor.CertInfo
	if err := m.UnmarshalJSONField("certInfo", &cert); err != nil || cert.Expires <= 0 {
		return monitorObservation{}, false
	}
	triggered := certExpiresWithin(cert.Expires, threshold, now)
	return monitorObservation{triggered: triggered, value: float64(certDaysLeft(cert.Expires, now)), certExpires: cert.Expires}, true
}

// certExpiresWithin reports whether a certificate expires within the given number of days.
// It compares exact times, so a threshold of 14 does not trigger with 14 days and 23 hours left.
func certExpiresWithin(expires int64, days float64, now time.Time) bool {
	return expires <= now.Add(time.Duration(days*float64(24*time.Hour))).UnixMilli()
}

// certDaysLeft returns whole days until expiry for display, matching the days shown in the UI.
// It is negative once the certificate has expired.
func certDaysLeft(expires int64, now time.Time) int {
	return int(math.Floor(float64(expires-now.UnixMilli()) / float64(time.Hour.Milliseconds()*24)))
}

func networkMonitorAlertMessage(name string, obs monitorObservation, label, systemName string, threshold float64) (title, message string) {
	if name == alertNameNetworkMonitorCert {
		days := int(obs.value)
		date := time.UnixMilli(obs.certExpires).UTC().Format(time.DateOnly)
		switch {
		case days < 0:
			return fmt.Sprintf("Certificate expired on %s: %s", systemName, label),
				fmt.Sprintf("%s on %s: certificate expired on %s.", label, systemName, date)
		case obs.triggered:
			return fmt.Sprintf("Certificate expiring on %s: %s", systemName, label),
				fmt.Sprintf("%s on %s: certificate expires in %s (%s), which is within the %d-day threshold.", label, systemName, pluralDays(days), date, int(threshold))
		default:
			return fmt.Sprintf("Certificate expiry recovered on %s: %s", systemName, label),
				fmt.Sprintf("%s on %s: certificate expires in %s (%s), which is beyond the %d-day threshold.", label, systemName, pluralDays(days), date, int(threshold))
		}
	}
	state, comparison := "loss", "exceeds"
	if !obs.triggered {
		state, comparison = "recovered", "is at or below"
	}
	return fmt.Sprintf("Network monitor %s on %s: %s", state, systemName, label),
		fmt.Sprintf("%s on %s: loss over the past hour is %.2f%%, which %s the %.2f%% threshold.", label, systemName, obs.value, comparison, threshold)
}

func pluralDays(days int) string {
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
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
