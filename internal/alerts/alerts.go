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

type userSettingsCacheEntry struct {
	settings  UserNotificationSettings
	timestamp time.Time
}

type parsedWebhook struct {
	url    *url.URL
	scheme string
}

type AlertManager struct {
	hub                hubLike
	alertQueue         chan alertTask
	stopChan           chan struct{}
	pendingAlerts      sync.Map
	templateService    *TemplateService
	userSettingsCache  sync.Map // map[string]userSettingsCacheEntry
	webhookCache       sync.Map // map[string]parsedWebhook
	settingsCacheTTL   time.Duration
}

type AlertMessageData struct {
	UserID   string
	Title    string
	Message  string
	Link     string
	LinkText string
}

type UserNotificationSettings struct {
	Emails   []string `json:"emails"`
	Webhooks []string `json:"webhooks"`
}

type SystemAlertStats struct {
	Cpu          float64            `json:"cpu"`
	Mem          float64            `json:"mp"`
	Disk         float64            `json:"dp"`
	NetSent      float64            `json:"ns"`
	NetRecv      float64            `json:"nr"`
	Temperatures map[string]float32 `json:"t"`
	LoadAvg      [3]float64         `json:"la"`
	SwapPct      float64            `json:"sp"`
}

type SystemAlertData struct {
	systemRecord *core.Record
	alertRecord  *core.Record
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
	"matrix":     {},
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
		hub:              app,
		alertQueue:       make(chan alertTask, 100), // Increased buffer for burst traffic
		stopChan:         make(chan struct{}),
		templateService:  NewTemplateService(app),
		settingsCacheTTL: 5 * time.Minute, // Cache settings for 5 minutes
	}
	am.bindEvents()
	go am.startWorker()
	am.StartRepeatingAlertChecker() // Start the repeating alert checker
	return am
}

// Bind events to the alerts collection lifecycle
func (am *AlertManager) bindEvents() {
	am.hub.OnRecordAfterUpdateSuccess("alerts").BindFunc(updateHistoryOnAlertUpdate)
	am.hub.OnRecordAfterDeleteSuccess("alerts").BindFunc(resolveHistoryOnAlertDelete)
}

// SendAlert sends an alert to the user
func (am *AlertManager) SendAlert(data AlertMessageData) error {
	// Get user settings from cache or database
	userAlertSettings, err := am.getUserSettings(data.UserID)
	if err != nil {
		return err
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

// getUserSettings retrieves user settings from cache or database
func (am *AlertManager) getUserSettings(userID string) (UserNotificationSettings, error) {
	// Check cache first
	if cached, ok := am.userSettingsCache.Load(userID); ok {
		entry := cached.(userSettingsCacheEntry)
		if time.Since(entry.timestamp) < am.settingsCacheTTL {
			return entry.settings, nil
		}
		// Cache expired, remove it
		am.userSettingsCache.Delete(userID)
	}

	// Fetch from database
	record, err := am.hub.FindFirstRecordByFilter(
		"user_settings", "user={:user}",
		dbx.Params{"user": userID},
	)
	if err != nil {
		return UserNotificationSettings{}, err
	}

	// unmarshal user settings
	userAlertSettings := UserNotificationSettings{
		Emails:   []string{},
		Webhooks: []string{},
	}
	if err := record.UnmarshalJSONField("settings", &userAlertSettings); err != nil {
		am.hub.Logger().Error("Failed to unmarshal user settings", "err", err)
		return userAlertSettings, err
	}

	// Cache the settings
	am.userSettingsCache.Store(userID, userSettingsCacheEntry{
		settings:  userAlertSettings,
		timestamp: time.Now(),
	})

	return userAlertSettings, nil
}

// parseWebhookURL parses and caches webhook URLs
func (am *AlertManager) parseWebhookURL(notificationUrl string) (parsedWebhook, error) {
	// Check cache first
	if cached, ok := am.webhookCache.Load(notificationUrl); ok {
		return cached.(parsedWebhook), nil
	}

	// Parse the URL
	parsedURL, err := url.Parse(notificationUrl)
	if err != nil {
		return parsedWebhook{}, fmt.Errorf("error parsing URL: %v", err)
	}

	webhook := parsedWebhook{
		url:    parsedURL,
		scheme: parsedURL.Scheme,
	}

	// Cache it
	am.webhookCache.Store(notificationUrl, webhook)
	return webhook, nil
}

// SendShoutrrrAlert sends an alert via a Shoutrrr URL
func (am *AlertManager) SendShoutrrrAlert(notificationUrl, title, message, link, linkText string) error {
	// Parse the URL (from cache if available)
	webhook, err := am.parseWebhookURL(notificationUrl)
	if err != nil {
		return err
	}

	// Create a copy of the URL to avoid modifying the cached version
	parsedURL := *webhook.url
	scheme := webhook.scheme
	queryParams := parsedURL.Query()

	// Use strings.Builder for efficient message building
	var msgBuilder strings.Builder
	msgBuilder.Grow(len(message) + len(title) + len(link) + 20)

	// Add title
	if _, ok := supportsTitle[scheme]; ok {
		queryParams.Add("title", title)
		msgBuilder.WriteString(message)
	} else if scheme == "mattermost" {
		// use markdown title for mattermost
		msgBuilder.WriteString("##### ")
		msgBuilder.WriteString(title)
		msgBuilder.WriteString("\n\n")
		msgBuilder.WriteString(message)
	} else if scheme == "generic" && queryParams.Has("template") {
		// add title as property if using generic with template json
		titleKey := queryParams.Get("titlekey")
		if titleKey == "" {
			titleKey = "title"
		}
		queryParams.Add("$"+titleKey, title)
		msgBuilder.WriteString(message)
	} else {
		// otherwise just add title to message
		msgBuilder.WriteString(title)
		msgBuilder.WriteString("\n\n")
		msgBuilder.WriteString(message)
	}

	// Add link
	if scheme == "ntfy" {
		queryParams.Add("Actions", fmt.Sprintf("view, %s, %s", linkText, link))
	} else if scheme == "lark" {
		queryParams.Add("link", link)
	} else if scheme == "bark" {
		queryParams.Add("url", link)
	} else {
		msgBuilder.WriteString("\n\n")
		msgBuilder.WriteString(link)
	}

	message = msgBuilder.String()

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

func (am *AlertManager) SendTestNotification(e *core.RequestEvent) error {
	var data struct {
		URL string `json:"url"`
	}
	err := e.BindBody(&data)
	if err != nil || data.URL == "" {
		return e.BadRequestError("URL is required", err)
	}
	err = am.SendShoutrrrAlert(data.URL, "Test Alert", "This is a notification from Beszel.", am.hub.Settings().Meta.AppURL, "View Beszel")
	if err != nil {
		return e.JSON(200, map[string]string{"err": err.Error()})
	}
	return e.JSON(200, map[string]bool{"err": false})
}
