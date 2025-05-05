// Package alerts handles alert management and delivery.
package alerts

import (
	"fmt"
	"net/mail"
	"net/url"
	"sync"
	"time"

	"github.com/containrrr/shoutrrr"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/mailer"
)

type AlertManager struct {
	app           core.App
	alertQueue    chan alertTask
	stopChan      chan struct{}
	pendingAlerts sync.Map
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
func NewAlertManager(app core.App) *AlertManager {
	am := &AlertManager{
		app:        app,
		alertQueue: make(chan alertTask),
		stopChan:   make(chan struct{}),
	}
	go am.startWorker()
	return am
}

func (am *AlertManager) SendAlert(data AlertMessageData) error {
	// get user settings
	record, err := am.app.FindFirstRecordByFilter(
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
		am.app.Logger().Error("Failed to unmarshal user settings", "err", err.Error())
	}
	// send alerts via webhooks
	for _, webhook := range userAlertSettings.Webhooks {
		if err := am.SendShoutrrrAlert(webhook, data.Title, data.Message, data.Link, data.LinkText); err != nil {
			am.app.Logger().Error("Failed to send shoutrrr alert", "err", err.Error())
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
			Address: am.app.Settings().Meta.SenderAddress,
			Name:    am.app.Settings().Meta.SenderName,
		},
	}
	err = am.app.NewMailClient().Send(&message)
	if err != nil {
		return err
	}
	am.app.Logger().Info("Sent email alert", "to", message.To, "subj", message.Subject)
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
	if scheme == "ntfy" {
		// if ntfy, add link to actions
		queryParams.Add("Actions", fmt.Sprintf("view, %s, %s", linkText, link))
	} else {
		// else add link directly to the message
		message += "\n\n" + link
	}

	// Encode the modified query parameters back into the URL
	parsedURL.RawQuery = queryParams.Encode()
	// log.Println("URL after modification:", parsedURL.String())

	err = shoutrrr.Send(parsedURL.String(), message)

	if err == nil {
		am.app.Logger().Info("Sent shoutrrr alert", "title", title)
	} else {
		am.app.Logger().Error("Error sending shoutrrr alert", "err", err.Error())
		return err
	}
	return nil
}

func (am *AlertManager) SendTestNotification(e *core.RequestEvent) error {
	info, _ := e.RequestInfo()
	if info.Auth == nil {
		return apis.NewForbiddenError("Forbidden", nil)
	}
	url := e.Request.URL.Query().Get("url")
	// log.Println("url", url)
	if url == "" {
		return e.JSON(200, map[string]string{"err": "URL is required"})
	}
	err := am.SendShoutrrrAlert(url, "Test Alert", "This is a notification from CMonitor.", am.app.Settings().Meta.AppURL, "View CMonitor")
	if err != nil {
		return e.JSON(200, map[string]string{"err": err.Error()})
	}
	return e.JSON(200, map[string]bool{"err": false})
}
