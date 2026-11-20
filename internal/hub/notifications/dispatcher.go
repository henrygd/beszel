package notifications

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/henrygd/beszel/internal/entities/notification"
	"github.com/henrygd/beszel/internal/hub/notifications/providers"
)

// Dispatcher manages notification sending for monitor events
type Dispatcher struct {
	app       core.App
	mu        sync.RWMutex
	providers map[string]notification.Provider
}

// NewDispatcher creates a new notification dispatcher
func NewDispatcher(app core.App) *Dispatcher {
	return &Dispatcher{
		app:       app,
		providers: make(map[string]notification.Provider),
	}
}

// SendNotification sends a notification for a monitor event
func (d *Dispatcher) SendNotification(monitorRecord *monitor.Monitor, heartbeat *monitor.Heartbeat, isRecovery bool) {
	// Honor per-monitor toggles
	if isRecovery && !monitorRecord.NotifyOnRecover {
		return
	}
	if !isRecovery && !monitorRecord.NotifyOnDown {
		return
	}
	if d.inQuietHours(monitorRecord) {
		d.logNotificationEvent("", monitorRecord.ID, "skipped", "quiet hours", "")
		return
	}

	// Get linked notifications for this monitor
	notifications, err := d.getMonitorNotifications(monitorRecord.ID)
	if err != nil {
		log.Printf("[notification-dispatcher] Failed to get notifications: %v", err)
		return
	}

	if len(notifications) == 0 {
		return
	}

	// Build the message
	msg := d.buildMessage(monitorRecord, heartbeat, isRecovery)

	// Send to each notification provider
	for _, n := range notifications {
		if !n.Active {
			continue
		}

		provider, err := d.getProvider(n)
		if err != nil {
			log.Printf("[notification-dispatcher] Failed to get provider: %v", err)
			d.logNotificationEvent(n.ID, monitorRecord.ID, "failed", "", err.Error())
			continue
		}

		if err := provider.Send(msg); err != nil {
			log.Printf("[notification-dispatcher] Failed to send notification: %v", err)
			d.logNotificationEvent(n.ID, monitorRecord.ID, "failed", "", err.Error())
		} else {
			log.Printf("[notification-dispatcher] Sent notification via %s for monitor %s", n.Type, monitorRecord.Name)
			d.logNotificationEvent(n.ID, monitorRecord.ID, "sent", "", "")
		}
	}
}

// getMonitorNotifications retrieves all notifications linked to a monitor
func (d *Dispatcher) getMonitorNotifications(monitorID string) ([]*notification.Notification, error) {
	// Find monitor_notification records for this monitor
	records, err := d.app.FindAllRecords("monitor_notifications",
		dbx.HashExp{"monitor": monitorID},
	)
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return d.getDefaultNotifications(monitorID)
	}

	var notifications []*notification.Notification
	for _, record := range records {
		notificationID := record.GetString("notification")
		notifRecord, err := d.app.FindRecordById("notifications", notificationID)
		if err != nil {
			continue
		}

		notif := &notification.Notification{
			ID:        notifRecord.Id,
			Name:      notifRecord.GetString("name"),
			Type:      notifRecord.GetString("type"),
			IsDefault: notifRecord.GetBool("is_default"),
			Active:    notifRecord.GetBool("active"),
		}

		// Parse settings from JSON
		if settingsJSON := notifRecord.GetString("settings"); settingsJSON != "" {
			var settings map[string]interface{}
			if err := json.Unmarshal([]byte(settingsJSON), &settings); err == nil {
				notif.Settings = settings
			}
		}

		notifications = append(notifications, notif)
	}

	return notifications, nil
}

// getProvider creates a provider instance for a notification config
func (d *Dispatcher) getProvider(n *notification.Notification) (notification.Provider, error) {
	d.mu.RLock()
	if provider, ok := d.providers[n.ID]; ok {
		d.mu.RUnlock()
		return provider, nil
	}
	d.mu.RUnlock()

	var provider notification.Provider

	switch n.Type {
	case notification.ProviderEmail:
		settings := n.GetSettings().(notification.EmailSettings)
		provider = providers.NewEmailProvider(settings)
	case notification.ProviderWebhook:
		settings := n.GetSettings().(notification.WebhookSettings)
		provider = providers.NewWebhookProvider(settings)
	case notification.ProviderDiscord:
		settings := n.GetSettings().(notification.DiscordSettings)
		provider = providers.NewDiscordProvider(settings)
	case notification.ProviderSlack:
		settings := n.GetSettings().(notification.SlackSettings)
		provider = providers.NewSlackProvider(settings)
	case notification.ProviderTelegram:
		settings := n.GetSettings().(notification.TelegramSettings)
		provider = providers.NewTelegramProvider(settings)
	case notification.ProviderGotify:
		settings := n.GetSettings().(notification.GotifySettings)
		provider = providers.NewGotifyProvider(settings)
	case notification.ProviderPushover:
		settings := n.GetSettings().(notification.PushoverSettings)
		provider = providers.NewPushoverProvider(settings)
	default:
		return nil, fmt.Errorf("unknown provider type: %s", n.Type)
	}

	if err := provider.Validate(); err != nil {
		return nil, err
	}

	d.mu.Lock()
	d.providers[n.ID] = provider
	d.mu.Unlock()

	return provider, nil
}

// buildMessage creates a notification message from monitor data
func (d *Dispatcher) buildMessage(m *monitor.Monitor, h *monitor.Heartbeat, isRecovery bool) *notification.NotificationMessage {
	status := "DOWN"
	if isRecovery {
		status = "UP"
	}

	title := fmt.Sprintf("%s is %s", m.Name, status)
	body := fmt.Sprintf("Monitor %s is %s.", m.Name, status)

	if !isRecovery && h.Msg != "" {
		body = fmt.Sprintf("Monitor %s is %s. Error: %s", m.Name, status, h.Msg)
	}

	return &notification.NotificationMessage{
		Title:       title,
		Body:        body,
		MonitorName: m.Name,
		MonitorURL:  d.getMonitorURL(m),
		Status:      status,
		Timestamp:   h.Time,
		Ping:        h.Ping,
		Message:     h.Msg,
	}
}

// getMonitorURL returns the URL or hostname for display
func (d *Dispatcher) getMonitorURL(m *monitor.Monitor) string {
	if m.URL != "" {
		return m.URL
	}
	if m.Hostname != "" {
		if m.Port > 0 {
			return fmt.Sprintf("%s:%d", m.Hostname, m.Port)
		}
		return m.Hostname
	}
	return ""
}

// logNotificationEvent logs a notification event to the database
func (d *Dispatcher) logNotificationEvent(notificationID, monitorID, status, message, errMsg string) {
	collection, findErr := d.app.FindCollectionByNameOrId("notification_events")
	if findErr != nil {
		return
	}

	record := core.NewRecord(collection)
	record.Set("notification", notificationID)
	record.Set("monitor", monitorID)
	record.Set("status", status)
	record.Set("message", message)
	record.Set("error", errMsg)

	if saveErr := d.app.Save(record); saveErr != nil {
		log.Printf("[notification-dispatcher] Failed to log notification event: %v", saveErr)
	}
}

// ClearCache clears the provider cache (call when settings change)
func (d *Dispatcher) ClearCache() {
	d.mu.Lock()
	d.providers = make(map[string]notification.Provider)
	d.mu.Unlock()
}

// Check if we need to send notification for this heartbeat
func (d *Dispatcher) ShouldNotify(m *monitor.Monitor, heartbeat *monitor.Heartbeat) (bool, bool) {
	// Check if this is a status change
	isDown := heartbeat.Status == monitor.StatusDown
	isRecovery := heartbeat.Status == monitor.StatusUp && m.Status == monitor.StatusDown

	// Only notify on down (after retries) or recovery
	return isDown && m.Status == monitor.StatusDown, isRecovery
}

// getDefaultNotifications returns the user's is_default providers when the
// monitor has no explicit links.
func (d *Dispatcher) getDefaultNotifications(monitorID string) ([]*notification.Notification, error) {
	monitorRecord, err := d.app.FindRecordById("monitors", monitorID)
	if err != nil {
		return nil, err
	}
	records, err := d.app.FindAllRecords("notifications",
		dbx.HashExp{"user": monitorRecord.GetString("user"), "is_default": true, "active": true},
	)
	if err != nil {
		return nil, err
	}
	var notifications []*notification.Notification
	for _, r := range records {
		n := &notification.Notification{
			ID:        r.Id,
			Name:      r.GetString("name"),
			Type:      r.GetString("type"),
			IsDefault: true,
			Active:    true,
		}
		if settingsJSON := r.GetString("settings"); settingsJSON != "" {
			var settings map[string]interface{}
			if err := json.Unmarshal([]byte(settingsJSON), &settings); err == nil {
				n.Settings = settings
			}
		}
		notifications = append(notifications, n)
	}
	return notifications, nil
}

// inQuietHours reports whether the current local time falls inside the
// monitor's configured quiet window (supports overnight ranges).
func (d *Dispatcher) inQuietHours(m *monitor.Monitor) bool {
	if !m.QuietHoursEnabled || m.QuietHoursStart == "" || m.QuietHoursEnd == "" {
		return false
	}
	start, err1 := time.Parse("15:04", m.QuietHoursStart)
	end, err2 := time.Parse("15:04", m.QuietHoursEnd)
	if err1 != nil || err2 != nil {
		return false
	}
	now, _ := time.Parse("15:04", time.Now().Format("15:04"))
	if start.Before(end) {
		return !now.Before(start) && now.Before(end)
	}
	// overnight window, e.g. 22:00-06:00
	return !now.Before(start) || now.Before(end)
}

// EvaluateThresholds fires performance notifications on every heartbeat when
// the monitor opts in. Cooldown: one notification per monitor+kind per hour.
func (d *Dispatcher) EvaluateThresholds(m *monitor.Monitor, heartbeat *monitor.Heartbeat, uptime24h float64) {
	if d.inQuietHours(m) {
		return
	}
	if m.NotifyOnResponseTime && m.ResponseTimeThreshold > 0 && heartbeat.Ping > m.ResponseTimeThreshold {
		if d.cooledDown(m.ID, "response_time") {
			msg := &notification.NotificationMessage{
				Title:       fmt.Sprintf("Slow response: %s", m.Name),
				Body:        fmt.Sprintf("Response time %dms exceeds threshold %dms", heartbeat.Ping, m.ResponseTimeThreshold),
				MonitorName: m.Name,
				MonitorURL:  d.getMonitorURL(m),
				Status:      string(heartbeat.Status),
				Ping:        heartbeat.Ping,
				Timestamp:   heartbeat.Time,
			}
			d.dispatchToDefaults(m, msg, "response_time")
		}
	}
	if m.NotifyOnUptimeDrop && m.UptimeThreshold > 0 && uptime24h < m.UptimeThreshold {
		if d.cooledDown(m.ID, "uptime_drop") {
			msg := &notification.NotificationMessage{
				Title:       fmt.Sprintf("Uptime dropped: %s", m.Name),
				Body:        fmt.Sprintf("24h uptime %.2f%% is below threshold %.2f%%", uptime24h, m.UptimeThreshold),
				MonitorName: m.Name,
				MonitorURL:  d.getMonitorURL(m),
				Status:      string(heartbeat.Status),
				Ping:        heartbeat.Ping,
				Timestamp:   heartbeat.Time,
			}
			d.dispatchToDefaults(m, msg, "uptime_drop")
		}
	}
}

// cooledDown returns true if no notification event of this kind fired in the
// last hour for the monitor.
func (d *Dispatcher) cooledDown(monitorID, kind string) bool {
	cutoff := time.Now().Add(-time.Hour).UTC().Format("2006-01-02 15:04:05.000Z")
	recs, err := d.app.FindAllRecords("notification_events",
		dbx.NewExp("monitor = {:monitor} && message = {:kind} && created >= {:cutoff}",
			dbx.Params{"monitor": monitorID, "kind": kind, "cutoff": cutoff}),
	)
	return err != nil || len(recs) == 0
}

// dispatchToDefaults sends a message through linked+default providers.
func (d *Dispatcher) dispatchToDefaults(m *monitor.Monitor, msg *notification.NotificationMessage, kind string) {
	notifications, err := d.getMonitorNotifications(m.ID)
	if err != nil || len(notifications) == 0 {
		return
	}
	for _, n := range notifications {
		if !n.Active {
			continue
		}
		provider, err := d.getProvider(n)
		if err != nil {
			d.logNotificationEvent(n.ID, m.ID, "failed", kind, err.Error())
			continue
		}
		if err := provider.Send(msg); err != nil {
			d.logNotificationEvent(n.ID, m.ID, "failed", kind, err.Error())
		} else {
			d.logNotificationEvent(n.ID, m.ID, "sent", kind, "")
		}
	}

}
