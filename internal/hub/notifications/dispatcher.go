package notifications

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

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
		return nil, nil
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
