// Package alerts handles alert management and delivery.
package alerts

import (
	"fmt"
	"net/mail"
	"net/url"
	"sync"
	"time"

	"github.com/nicholas-fedor/shoutrrr"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/mailer"
)

type hubLike interface {
	core.App
	MakeLink(parts ...string) string
}

type AlertManager struct {
	hub           hubLike
	stopOnce      sync.Once
	pendingAlerts sync.Map
	alertsCache   *AlertsCache
}

type AlertMessageData struct {
	UserID   string
	SystemID string
	Title    string
	Message  string
	Link     string
	LinkText string
}

type UserNotificationSettings struct {
	Emails   []string `json:"emails"`
	Webhooks []string `json:"webhooks"`
}

type SystemAlertFsStats struct {
	DiskTotal float64 `json:"d"`
	DiskUsed  float64 `json:"du"`
}

// Values pulled from system_stats.stats that are relevant to alerts.
type SystemAlertStats struct {
	Cpu          float64                       `json:"cpu"`
	Mem          float64                       `json:"mp"`
	Disk         float64                       `json:"dp"`
	Bandwidth    [2]uint64                     `json:"b"`
	GPU          map[string]SystemAlertGPUData `json:"g"`
	Temperatures map[string]float32            `json:"t"`
	LoadAvg      [3]float64                    `json:"la"`
	Battery      [2]uint8                      `json:"bat"`
	ExtraFs      map[string]SystemAlertFsStats `json:"efs"`
}

type SystemAlertGPUData struct {
	Usage float64 `json:"u"`
}

type SystemAlertData struct {
	systemRecord *core.Record
	alertData    CachedAlertData
	name         string
	unit         string
	val          float64
	threshold    float64
	triggered    bool
	time         time.Time
	count        uint8
	min          uint8
	mapSums      map[string]float32
	descriptor   string // override descriptor in notification body (for temp sensor, disk partition, etc)
}

// notification services that support title param
var supportsTitle = map[string]struct{}{
	"bark":       {},
	"discord":    {},
	"gotify":     {},
	"ifttt":      {},
	"join":       {},
	"lark":       {},
	"ntfy":       {},
	"opsgenie":   {},
	"pushbullet": {},
	"pushover":   {},
	"slack":      {},
	"teams":      {},
	"telegram":   {},
	"zulip":      {},
}

// NewAlertManager creates a new AlertManager instance.
func NewAlertManager(app hubLike) *AlertManager {
	am := &AlertManager{
		hub:         app,
		alertsCache: NewAlertsCache(app),
	}
	am.bindEvents()
	return am
}

// Bind events to the alerts collection lifecycle
func (am *AlertManager) bindEvents() {
	am.hub.OnRecordAfterUpdateSuccess("alerts").BindFunc(updateHistoryOnAlertUpdate)
	am.hub.OnRecordAfterDeleteSuccess("alerts").BindFunc(resolveHistoryOnAlertDelete)
	am.hub.OnRecordAfterUpdateSuccess("smart_devices").BindFunc(am.handleSmartDeviceAlert)

	am.hub.OnServe().BindFunc(func(e *core.ServeEvent) error {
		// Populate all alerts into cache on startup
		_ = am.alertsCache.PopulateFromDB(true)

		if err := resolveStatusAlerts(e.App); err != nil {
			e.App.Logger().Error("Failed to resolve stale status alerts", "err", err)
		}
		if err := am.restorePendingStatusAlerts(); err != nil {
			e.App.Logger().Error("Failed to restore pending status alerts", "err", err)
		}
		return e.Next()
	})
}

// IsNotificationSilenced checks if a notification should be silenced based on configured quiet hours
func (am *AlertManager) IsNotificationSilenced(userID, systemID string) bool {
	// Query for quiet hours windows that match this user and system
	// Include both global windows (system is null/empty) and system-specific windows
	var filter string
	var params dbx.Params

	if systemID == "" {
		// If no systemID provided, only check global windows
		filter = "user={:user} AND system=''"
		params = dbx.Params{"user": userID}
	} else {
		// Check both global and system-specific windows
		filter = "user={:user} AND (system='' OR system={:system})"
		params = dbx.Params{
			"user":   userID,
			"system": systemID,
		}
	}

	quietHourWindows, err := am.hub.FindAllRecords("quiet_hours", dbx.NewExp(filter, params))
	if err != nil || len(quietHourWindows) == 0 {
		return false
	}

	now := time.Now().UTC()

	for _, window := range quietHourWindows {
		windowType := window.GetString("type")
		start := window.GetDateTime("start").Time()
		end := window.GetDateTime("end").Time()

		if windowType == "daily" {
			// For daily recurring windows, extract just the time portion and compare
			// The start/end are stored as full datetime but we only care about HH:MM
			startHour, startMin, _ := start.Clock()
			endHour, endMin, _ := end.Clock()
			nowHour, nowMin, _ := now.Clock()

			// Convert to minutes since midnight for easier comparison
			startMinutes := startHour*60 + startMin
			endMinutes := endHour*60 + endMin
			nowMinutes := nowHour*60 + nowMin

			// Handle case where window crosses midnight
			if endMinutes < startMinutes {
				// Window crosses midnight (e.g., 23:00 - 01:00)
				if nowMinutes >= startMinutes || nowMinutes < endMinutes {
					return true
				}
			} else {
				// Normal case (e.g., 09:00 - 17:00)
				if nowMinutes >= startMinutes && nowMinutes < endMinutes {
					return true
				}
			}
		} else {
			// One-time window: check if current time is within the date range
			if (now.After(start) || now.Equal(start)) && now.Before(end) {
				return true
			}
		}
	}

	return false
}

// SendAlert sends an alert to the user
func (am *AlertManager) SendAlert(data AlertMessageData) error {
	// Check if alert is silenced
	if am.IsNotificationSilenced(data.UserID, data.SystemID) {
		am.hub.Logger().Info("Notification silenced", "user", data.UserID, "system", data.SystemID, "title", data.Title)
		return nil
	}

	// get user settings
	record, err := am.hub.FindFirstRecordByFilter(
		"user_settings", "user={:user}",
		dbx.Params{"user": data.UserID},
	)
	if err != nil {
		return err
	}
	// unmarshal user settings
	userAlertSettings := UserNotificationSettings{
		Emails:   []string{},
		Webhooks: []string{},
	}
	if err := record.UnmarshalJSONField("settings", &userAlertSettings); err != nil {
		am.hub.Logger().Error("Failed to unmarshal user settings", "err", err)
	}
	// send alerts via webhooks
	for _, webhook := range userAlertSettings.Webhooks {
		if err := am.SendShoutrrrAlert(webhook, data.Title, data.Message, data.Link, data.LinkText); err != nil {
			am.hub.Logger().Error("Failed to send shoutrrr alert", "err", err)
		}
	}
	// send alerts via email
	if len(userAlertSettings.Emails) == 0 {
		return nil
	}
	addresses := []mail.Address{}
	for _, email := range userAlertSettings.Emails {
		addresses = append(addresses, mail.Address{Address: email})
	}
	message := mailer.Message{
		To:      addresses,
		Subject: data.Title,
		Text:    data.Message + fmt.Sprintf("\n\n%s", data.Link),
		From: mail.Address{
			Address: am.hub.Settings().Meta.SenderAddress,
			Name:    am.hub.Settings().Meta.SenderName,
		},
	}
	err = am.hub.NewMailClient().Send(&message)
	if err != nil {
		return err
	}
	am.hub.Logger().Info("Sent email alert", "to", message.To, "subj", message.Subject)
	return nil
}

// SendShoutrrrAlert sends an alert via a Shoutrrr URL
func (am *AlertManager) SendShoutrrrAlert(notificationUrl, title, message, link, linkText string) error {
	// Parse the URL
	parsedURL, err := url.Parse(notificationUrl)
	if err != nil {
		return fmt.Errorf("error parsing URL: %v", err)
	}
	scheme := parsedURL.Scheme
	queryParams := parsedURL.Query()

	// Add title
	if _, ok := supportsTitle[scheme]; ok {
		queryParams.Add("title", title)
	} else if scheme == "mattermost" {
		// use markdown title for mattermost
		message = "##### " + title + "\n\n" + message
	} else if scheme == "generic" && queryParams.Has("template") {
		// add title as property if using generic with template json
		titleKey := queryParams.Get("titlekey")
		if titleKey == "" {
			titleKey = "title"
		}
		queryParams.Add("$"+titleKey, title)
	} else {
		// otherwise just add title to message
		message = title + "\n\n" + message
	}

	// Add link
	switch scheme {
	case "ntfy":
		queryParams.Add("Actions", fmt.Sprintf("view, %s, %s", linkText, link))
	case "lark":
		queryParams.Add("link", link)
	case "bark":
		queryParams.Add("url", link)
	default:
		message += "\n\n" + link
	}

	// Encode the modified query parameters back into the URL
	parsedURL.RawQuery = queryParams.Encode()
	// log.Println("URL after modification:", parsedURL.String())

	err = shoutrrr.Send(parsedURL.String(), message)

	if err == nil {
		am.hub.Logger().Info("Sent shoutrrr alert", "title", title)
	} else {
		am.hub.Logger().Error("Error sending shoutrrr alert", "err", err)
		return err
	}
	return nil
}

// setAlertTriggered updates the "triggered" status of an alert record in the database
func (am *AlertManager) setAlertTriggered(alert CachedAlertData, triggered bool) error {
	alertRecord, err := am.hub.FindRecordById("alerts", alert.Id)
	if err != nil {
		return err
	}
	alertRecord.Set("triggered", triggered)
	return am.hub.Save(alertRecord)
}
