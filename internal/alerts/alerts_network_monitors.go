package alerts

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net"
	"slices"
	"strconv"
	"time"

	"github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

const (
	alertNameNetworkMonitorLoss = "NetworkMonitorLoss"
	alertNameNetworkMonitorCert = "NetworkMonitorCert"
	// alertNameNetworkMonitorDown triggers after a number of consecutive failed probes.
	alertNameNetworkMonitorDown = "NetworkMonitorDown"
	// alertNameNetworkMonitorLatency triggers when the one hour average response time exceeds a threshold in milliseconds.
	alertNameNetworkMonitorLatency = "NetworkMonitorLatency"
	// maxCertAlertDays is the largest certificate expiry threshold in days.
	maxCertAlertDays = 365
	// maxDownAlertChecks is the largest number of consecutive failed probes for a down alert.
	maxDownAlertChecks = 20
	// maxLatencyAlertMs is the largest response time threshold in milliseconds.
	maxLatencyAlertMs = 10_000
)

// networkMonitorAlertNames lists every alert type that tracks incidents per network monitor.
var networkMonitorAlertNames = []string{
	alertNameNetworkMonitorLoss, alertNameNetworkMonitorCert,
	alertNameNetworkMonitorDown, alertNameNetworkMonitorLatency,
}

// networkMonitorAlertState is this alert type's persisted runtime state.
// Monitor IDs map to their open history entries independently of history retention.
type networkMonitorAlertState struct {
	Monitors map[string]string `json:"monitors"`
}

// monitorObservation is one monitor's current value for an alert threshold.
type monitorObservation struct {
	triggered bool
	// value is stored in alert history: loss percentage, days left, failed
	// probes in a row, or average response time in milliseconds.
	value float64
	// certExpires is the certificate expiry in Unix milliseconds (certificate alerts only).
	certExpires int64
}

// isNetworkMonitorAlert reports whether an alert tracks incidents per network monitor.
func isNetworkMonitorAlert(name string) bool {
	return slices.Contains(networkMonitorAlertNames, name)
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
	case alertNameNetworkMonitorDown:
		if math.IsNaN(value) || value < 1 || value > maxDownAlertChecks || value != math.Trunc(value) {
			return fmt.Sprintf("Down threshold must be a whole number of failed checks from 1 to %d", maxDownAlertChecks)
		}
	case alertNameNetworkMonitorLatency:
		if math.IsNaN(value) || value < 1 || value > maxLatencyAlertMs {
			return fmt.Sprintf("Response time threshold must be from 1 to %d ms", maxLatencyAlertMs)
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
	alerts := am.alertsCache.GetSystemAlerts(systemRecord.Id)
	alerts = slices.DeleteFunc(alerts, func(a CachedAlertData) bool { return !isNetworkMonitorAlert(a.Name) })
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
			cutoff := certExpiryCutoff(alert.Value, now)
			for id, m := range monitors {
				if m.certExpires <= 0 {
					continue
				}
				_, active := alert.MonitorStates[id]
				if (m.certExpires <= cutoff) != active {
					return true
				}
			}
			continue
		}
		for id, result := range results {
			m, enabled := monitors[id]
			if !enabled {
				continue
			}
			obs, ok := observeMonitorResult(alert.Name, result, m.interval, alert.Value, now)
			if !ok {
				continue
			}
			_, active := alert.MonitorStates[id]
			if obs.triggered != active {
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
		alerts, err := tx.FindAllRecords("alerts", dbx.HashExp{"system": systemID, "name": networkMonitorAlertNamesAny()})
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
						obs, ok = observeMonitorResult(name, results[m.Id], m.GetInt("interval"), threshold, now)
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

// networkMonitorAlertNamesAny returns networkMonitorAlertNames for use in a dbx.HashExp IN clause.
func networkMonitorAlertNamesAny() []any {
	names := make([]any, len(networkMonitorAlertNames))
	for i, name := range networkMonitorAlertNames {
		names[i] = name
	}
	return names
}

// observeMonitorResult compares a fresh agent result with a loss, down, or
// response time threshold. It reports false when the result can't be judged yet.
func observeMonitorResult(name string, result monitor.Result, interval int, threshold float64, now time.Time) (monitorObservation, bool) {
	switch name {
	case alertNameNetworkMonitorLoss:
		if !monitorResultReady(result, interval, now) {
			return monitorObservation{}, false
		}
		return monitorObservation{triggered: result.PacketLoss1h > threshold, value: result.PacketLoss1h}, true
	case alertNameNetworkMonitorDown:
		// No warm-up: a target that is down from the first probe should still alert.
		// Agents that predate the streak report zero and are skipped.
		if result.Streak == 0 || !monitorResultFresh(result, interval, now) {
			return monitorObservation{}, false
		}
		failed := float64(max(-result.Streak, 0))
		return monitorObservation{triggered: failed >= threshold, value: failed}, true
	case alertNameNetworkMonitorLatency:
		// Without a successful probe in the hour there is no response time to judge;
		// down and loss alerts cover that case.
		if !monitorResultReady(result, interval, now) || result.PacketLoss1h >= 100 || result.AvgResponse1h <= 0 {
			return monitorObservation{}, false
		}
		ms := math.Round(float64(result.AvgResponse1h)/10) / 100
		return monitorObservation{triggered: ms > threshold, value: ms}, true
	}
	return monitorObservation{}, false
}

// observeMonitorCert compares the stored certificate expiry with a threshold in days.
// Monitors without certificate info (non-HTTPS or never checked) are skipped.
func observeMonitorCert(m *core.Record, threshold float64, now time.Time) (monitorObservation, bool) {
	var cert monitor.CertInfo
	if err := m.UnmarshalJSONField("certInfo", &cert); err != nil || cert.Expires <= 0 {
		return monitorObservation{}, false
	}
	triggered := cert.Expires <= certExpiryCutoff(threshold, now)
	return monitorObservation{triggered: triggered, value: float64(certDaysLeft(cert.Expires, now)), certExpires: cert.Expires}, true
}

// certExpiryCutoff returns the latest expiry, in Unix milliseconds, that is within the given
// number of days. It uses exact times, so a threshold of 14 does not trigger with 14 days and
// 23 hours left.
func certExpiryCutoff(days float64, now time.Time) int64 {
	return now.Add(time.Duration(days * float64(24*time.Hour))).UnixMilli()
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
	if name == alertNameNetworkMonitorDown {
		if obs.triggered {
			return fmt.Sprintf("Network monitor down on %s: %s", systemName, label),
				fmt.Sprintf("%s on %s: the last %s failed.", label, systemName, pluralChecks(int(obs.value)))
		}
		return fmt.Sprintf("Network monitor up on %s: %s", systemName, label),
			fmt.Sprintf("%s on %s: the latest check succeeded.", label, systemName)
	}
	state, comparison := "loss", "exceeds"
	if !obs.triggered {
		state, comparison = "recovered", "is at or below"
	}
	if name == alertNameNetworkMonitorLatency {
		state = "slow"
		if !obs.triggered {
			state = "recovered"
		}
		return fmt.Sprintf("Network monitor %s on %s: %s", state, systemName, label),
			fmt.Sprintf("%s on %s: average response time over the past hour is %.2f ms, which %s the %g ms threshold.", label, systemName, obs.value, comparison, threshold)
	}
	return fmt.Sprintf("Network monitor %s on %s: %s", state, systemName, label),
		fmt.Sprintf("%s on %s: loss over the past hour is %.2f%%, which %s the %.2f%% threshold.", label, systemName, obs.value, comparison, threshold)
}

func pluralChecks(checks int) string {
	if checks == 1 {
		return "check"
	}
	return fmt.Sprintf("%d checks", checks)
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
	if result.SampleCount < 3 || math.IsNaN(result.PacketLoss1h) || math.IsInf(result.PacketLoss1h, 0) || result.PacketLoss1h < 0 || result.PacketLoss1h > 100 {
		return false
	}
	// Never interpret an empty one-hour window as zero loss.
	return monitorResultFresh(result, interval, now)
}

// monitorResultFresh reports whether the result's latest probe is recent for the monitor's interval.
func monitorResultFresh(result monitor.Result, interval int, now time.Time) bool {
	if result.LastProbeAt <= 0 {
		return false
	}
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
