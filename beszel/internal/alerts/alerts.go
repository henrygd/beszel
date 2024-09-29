// Package alerts handles alert management and delivery.
package alerts

import (
	"beszel/internal/entities/system"
	"fmt"
	"net/mail"
	"net/url"

	"github.com/containrrr/shoutrrr"
	"github.com/labstack/echo/v5"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/tools/mailer"
)

type AlertManager struct {
	app *pocketbase.PocketBase
}

type AlertData struct {
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

func NewAlertManager(app *pocketbase.PocketBase) *AlertManager {
	return &AlertManager{
		app: app,
	}
}

func (am *AlertManager) HandleSystemInfoAlerts(systemRecord *models.Record, systemInfo system.Info) {
	alertRecords, err := am.app.Dao().FindRecordsByExpr("alerts",
		dbx.NewExp("system={:system}", dbx.Params{"system": systemRecord.GetId()}),
	)
	if err != nil || len(alertRecords) == 0 {
		// log.Println("no alerts found for system")
		return
	}
	// log.Println("found alerts", len(alertRecords))
	for _, alertRecord := range alertRecords {
		name := alertRecord.GetString("name")
		switch name {
		case "CPU", "Memory", "Disk":
			if name == "CPU" {
				am.handleSlidingValueAlert(systemRecord, alertRecord, name, systemInfo.Cpu)
			} else if name == "Memory" {
				am.handleSlidingValueAlert(systemRecord, alertRecord, name, systemInfo.MemPct)
			} else if name == "Disk" {
				am.handleSlidingValueAlert(systemRecord, alertRecord, name, systemInfo.DiskPct)
			}
		}
	}
}

func (am *AlertManager) handleSlidingValueAlert(systemRecord *models.Record, alertRecord *models.Record, name string, curValue float64) {
	triggered := alertRecord.GetBool("triggered")
	threshold := alertRecord.GetFloat("value")
	// fmt.Println(name, curValue, "threshold", threshold, "triggered", triggered)
	var subject string
	var body string
	var systemName string
	if !triggered && curValue > threshold {
		alertRecord.Set("triggered", true)
		systemName = systemRecord.GetString("name")
		subject = fmt.Sprintf("%s usage above threshold on %s", name, systemName)
		body = fmt.Sprintf("%s usage on %s is %.1f%%.", name, systemName, curValue)
	} else if triggered && curValue <= threshold {
		alertRecord.Set("triggered", false)
		systemName = systemRecord.GetString("name")
		subject = fmt.Sprintf("%s usage below threshold on %s", name, systemName)
		body = fmt.Sprintf("%s usage on %s is below threshold at %.1f%%.", name, systemName, curValue)
	} else {
		// fmt.Println(name, "not triggered")
		return
	}
	if err := am.app.Dao().SaveRecord(alertRecord); err != nil {
		// app.Logger().Error("failed to save alert record", "err", err.Error())
		return
	}
	// expand the user relation and send the alert
	if errs := am.app.Dao().ExpandRecord(alertRecord, []string{"user"}, nil); len(errs) > 0 {
		// app.Logger().Error("failed to expand user relation", "errs", errs)
		return
	}
	if user := alertRecord.ExpandedOne("user"); user != nil {
		am.sendAlert(AlertData{
			UserID:   user.GetId(),
			Title:    subject,
			Message:  body,
			Link:     am.app.Settings().Meta.AppUrl + "/system/" + url.QueryEscape(systemName),
			LinkText: "View " + systemName,
		})
	}
}

func (am *AlertManager) HandleStatusAlerts(newStatus string, oldSystemRecord *models.Record) error {
	var alertStatus string
	switch newStatus {
	case "up":
		if oldSystemRecord.GetString("status") == "down" {
			alertStatus = "up"
		}
	case "down":
		if oldSystemRecord.GetString("status") == "up" {
			alertStatus = "down"
		}
	}
	if alertStatus == "" {
		return nil
	}
	// check if use
	alertRecords, err := am.app.Dao().FindRecordsByExpr("alerts",
		dbx.HashExp{
			"system": oldSystemRecord.GetId(),
			"name":   "Status",
		},
	)
	if err != nil || len(alertRecords) == 0 {
		// log.Println("no alerts found for system")
		return nil
	}
	for _, alertRecord := range alertRecords {
		// expand the user relation
		if errs := am.app.Dao().ExpandRecord(alertRecord, []string{"user"}, nil); len(errs) > 0 {
			return fmt.Errorf("failed to expand: %v", errs)
		}
		user := alertRecord.ExpandedOne("user")
		if user == nil {
			return nil
		}
		emoji := "\U0001F534"
		if alertStatus == "up" {
			emoji = "\u2705"
		}
		// send alert
		systemName := oldSystemRecord.GetString("name")
		am.sendAlert(AlertData{
			UserID:   user.GetId(),
			Title:    fmt.Sprintf("Connection to %s is %s %v", systemName, alertStatus, emoji),
			Message:  fmt.Sprintf("Connection to %s is %s", systemName, alertStatus),
			Link:     am.app.Settings().Meta.AppUrl + "/system/" + url.QueryEscape(systemName),
			LinkText: "View " + systemName,
		})
	}
	return nil
}

func (am *AlertManager) sendAlert(data AlertData) {
	// get user settings
	record, err := am.app.Dao().FindFirstRecordByFilter(
		"user_settings", "user={:user}",
		dbx.Params{"user": data.UserID},
	)
	if err != nil {
		am.app.Logger().Error("Failed to get user settings", "err", err.Error())
		return
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
		// log.Println("No email addresses found")
		return
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
	if err := am.app.NewMailClient().Send(&message); err != nil {
		am.app.Logger().Error("Failed to send alert: ", "err", err.Error())
	} else {
		am.app.Logger().Info("Sent email alert", "to", message.To, "subj", message.Subject)
	}
}

// SendShoutrrrAlert sends an alert via a Shoutrrr URL
func (am *AlertManager) SendShoutrrrAlert(notificationUrl, title, message, link, linkText string) error {
	// services that support title param
	supportsTitle := []string{"bark", "discord", "gotify", "ifttt", "join", "matrix", "ntfy", "opsgenie", "pushbullet", "pushover", "slack", "teams", "telegram", "zulip"}

	// Parse the URL
	parsedURL, err := url.Parse(notificationUrl)
	if err != nil {
		return fmt.Errorf("error parsing URL: %v", err)
	}
	scheme := parsedURL.Scheme
	queryParams := parsedURL.Query()

	// Add title
	if sliceContains(supportsTitle, scheme) {
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

// Contains checks if a string is present in a slice of strings
func sliceContains(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}

func (am *AlertManager) SendTestNotification(c echo.Context) error {
	requestData := apis.RequestInfo(c)
	if requestData.AuthRecord == nil {
		return apis.NewForbiddenError("Forbidden", nil)
	}
	url := c.QueryParam("url")
	// log.Println("url", url)
	if url == "" {
		return c.JSON(200, map[string]string{"err": "URL is required"})
	}
	err := am.SendShoutrrrAlert(url, "Test Alert", "This is a notification from Beszel.", am.app.Settings().Meta.AppUrl, "View Beszel")
	if err != nil {
		return c.JSON(200, map[string]string{"err": err.Error()})
	}
	return c.JSON(200, map[string]bool{"err": false})
}
