package alerts

import (
	"fmt"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type alertInfo struct {
	systemName string
	alertData  CachedAlertData
	expireTime time.Time
	timer      *time.Timer
}

// Stop cancels all pending status alert timers.
func (am *AlertManager) Stop() {
	am.stopOnce.Do(func() {
		am.pendingAlerts.Range(func(key, value any) bool {
			info := value.(*alertInfo)
			if info.timer != nil {
				info.timer.Stop()
			}
			am.pendingAlerts.Delete(key)
			return true
		})
	})
}

// HandleStatusAlerts manages the logic when system status changes.
func (am *AlertManager) HandleStatusAlerts(newStatus string, systemRecord *core.Record) error {
	if newStatus != "up" && newStatus != "down" {
		return nil
	}

	alerts := am.alertsCache.GetAlertsByName(systemRecord.Id, "Status")
	if len(alerts) == 0 {
		return nil
	}

	systemName := systemRecord.GetString("name")
	if newStatus == "down" {
		am.handleSystemDown(systemName, alerts)
	} else {
		am.handleSystemUp(systemName, alerts)
	}
	return nil
}

// handleSystemDown manages the logic when a system status changes to "down". It schedules pending alerts for each alert record.
func (am *AlertManager) handleSystemDown(systemName string, alerts []CachedAlertData) {
	for _, alertData := range alerts {
		min := max(1, int(alertData.Min))
		am.schedulePendingStatusAlert(systemName, alertData, time.Duration(min)*time.Minute)
	}
}

// schedulePendingStatusAlert sets up a timer to send a "down" alert after the specified delay if the system is still down.
// It returns true if the alert was scheduled, or false if an alert was already pending for the given alert record.
func (am *AlertManager) schedulePendingStatusAlert(systemName string, alertData CachedAlertData, delay time.Duration) bool {
	alert := &alertInfo{
		systemName: systemName,
		alertData:  alertData,
		expireTime: time.Now().Add(delay),
	}

	storedAlert, loaded := am.pendingAlerts.LoadOrStore(alertData.Id, alert)
	if loaded {
		return false
	}

	stored := storedAlert.(*alertInfo)
	stored.timer = time.AfterFunc(time.Until(stored.expireTime), func() {
		am.processPendingAlert(alertData.Id)
	})
	return true
}

// handleSystemUp manages the logic when a system status changes to "up".
// It cancels any pending alerts and sends "up" alerts.
func (am *AlertManager) handleSystemUp(systemName string, alerts []CachedAlertData) {
	for _, alertData := range alerts {
		// If alert exists for record, delete and continue (down alert not sent)
		if am.cancelPendingAlert(alertData.Id) {
			continue
		}
		if !alertData.Triggered {
			continue
		}
		if err := am.sendStatusAlert("up", systemName, alertData); err != nil {
			am.hub.Logger().Error("Failed to send alert", "err", err)
		}
	}
}

// cancelPendingAlert stops the timer and removes the pending alert for the given alert ID. Returns true if a pending alert was found and cancelled.
func (am *AlertManager) cancelPendingAlert(alertID string) bool {
	value, loaded := am.pendingAlerts.LoadAndDelete(alertID)
	if !loaded {
		return false
	}

	info := value.(*alertInfo)
	if info.timer != nil {
		info.timer.Stop()
	}
	return true
}

// CancelPendingStatusAlerts cancels all pending status alert timers for a given system.
// This is called when a system is paused to prevent delayed alerts from firing.
func (am *AlertManager) CancelPendingStatusAlerts(systemID string) {
	am.pendingAlerts.Range(func(key, value any) bool {
		info := value.(*alertInfo)
		if info.alertData.SystemID == systemID {
			am.cancelPendingAlert(key.(string))
		}
		return true
	})
}

// processPendingAlert sends a "down" alert if the pending alert has expired and the system is still down.
func (am *AlertManager) processPendingAlert(alertID string) {
	value, loaded := am.pendingAlerts.LoadAndDelete(alertID)
	if !loaded {
		return
	}

	info := value.(*alertInfo)
	refreshedAlertData, ok := am.alertsCache.Refresh(info.alertData)
	if !ok || refreshedAlertData.Triggered {
		return
	}
	if err := am.sendStatusAlert("down", info.systemName, refreshedAlertData); err != nil {
		am.hub.Logger().Error("Failed to send alert", "err", err)
	}
}

// sendStatusAlert sends a status alert ("up" or "down") to the users associated with the alert records.
func (am *AlertManager) sendStatusAlert(alertStatus string, systemName string, alertData CachedAlertData) error {
	// Capture the active incident before resolving it. The alert update hook
	// fills in its resolved time, matching the duration shown in alert history.
	var historyID string
	if alertStatus == "up" {
		history, err := am.hub.FindRecordsByFilter("alerts_history", "alert_id={:alert_id} && resolved=null", "-created", 1, 0, dbx.Params{"alert_id": alertData.Id})
		if err != nil {
			am.hub.Logger().Error("Failed to load status alert history", "err", err)
		} else if len(history) > 0 {
			historyID = history[0].Id
		}
	}

	// Update trigger state for alert record before sending alert
	triggered := alertStatus == "down"
	if err := am.setAlertTriggered(alertData, triggered); err != nil {
		return err
	}

	var emoji string
	if alertStatus == "up" {
		emoji = "\u2705" // Green checkmark emoji
	} else {
		emoji = "\U0001F534" // Red alert emoji
	}

	title := fmt.Sprintf("Connection to %s is %s %v", systemName, alertStatus, emoji)
	message := strings.TrimSuffix(title, emoji)
	if historyID != "" {
		history, err := am.hub.FindRecordById("alerts_history", historyID)
		if err != nil {
			am.hub.Logger().Error("Failed to load resolved status alert history", "err", err)
		} else if duration := formatStatusAlertDuration(history.GetDateTime("created").Time(), history.GetDateTime("resolved").Time()); duration != "" {
			message = fmt.Sprintf("%s (duration %s)", strings.TrimSpace(message), duration)
		}
	}

	// Get system ID for the link
	systemID := alertData.SystemID

	return am.SendAlert(AlertMessageData{
		UserID:   alertData.UserID,
		SystemID: systemID,
		Title:    title,
		Message:  message,
		Link:     am.hub.MakeLink("system", systemID),
		LinkText: "View " + systemName,
	})
}

// formatStatusAlertDuration matches the duration displayed in alert history.
func formatStatusAlertDuration(created, resolved time.Time) string {
	if created.IsZero() || resolved.IsZero() || resolved.Before(created) {
		return ""
	}

	totalSeconds := int(resolved.Sub(created) / time.Second)
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	if seconds >= 58 {
		minutes++
		seconds = 0
	}
	if minutes >= 60 {
		hours++
		minutes = 0
	}

	var parts []string
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if hours == 0 && seconds > 0 {
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}
	return strings.Join(parts, " ")
}

// resolveStatusAlerts resolves any triggered status alerts that weren't resolved
// when system came up (https://github.com/henrygd/beszel/issues/1052).
func resolveStatusAlerts(app core.App) error {
	db := app.DB()
	// Find all active status alerts where the system is actually up
	var alertIds []string
	err := db.NewQuery(`
		SELECT a.id 
		FROM alerts a
		JOIN systems s ON a.system = s.id
		WHERE a.name = 'Status' 
		AND a.triggered = true
		AND s.status = 'up'
	`).Column(&alertIds)
	if err != nil {
		return err
	}
	// resolve all matching alert records
	for _, alertId := range alertIds {
		alert, err := app.FindRecordById("alerts", alertId)
		if err != nil {
			return err
		}
		alert.Set("triggered", false)
		err = app.Save(alert)
		if err != nil {
			return err
		}
	}
	return nil
}

// restorePendingStatusAlerts re-queues untriggered status alerts for systems that
// are still down after a hub restart. This rebuilds the lost in-memory timer state.
func (am *AlertManager) restorePendingStatusAlerts() error {
	type pendingStatusAlert struct {
		AlertID    string `db:"alert_id"`
		SystemID   string `db:"system_id"`
		SystemName string `db:"system_name"`
	}

	var pending []pendingStatusAlert
	err := am.hub.DB().NewQuery(`
		SELECT a.id AS alert_id, a.system AS system_id, s.name AS system_name
		FROM alerts a
		JOIN systems s ON a.system = s.id
		WHERE a.name = 'Status'
		AND a.triggered = false
		AND s.status = 'down'
	`).All(&pending)
	if err != nil {
		return err
	}

	// Make sure cache is populated before trying to restore pending alerts
	_ = am.alertsCache.PopulateFromDB(false)

	for _, item := range pending {
		alertData, ok := am.alertsCache.GetAlert(item.SystemID, item.AlertID)
		if !ok {
			continue
		}
		min := max(1, int(alertData.Min))
		am.schedulePendingStatusAlert(item.SystemName, alertData, time.Duration(min)*time.Minute)
	}

	return nil
}
