// Package alerts handles alert management and delivery.
package alerts

import (
	"beszel/internal/entities/system"
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/containrrr/shoutrrr"
	"github.com/goccy/go-json"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/mailer"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/spf13/cast"
)

type AlertManager struct {
	app core.App
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

func NewAlertManager(app core.App) *AlertManager {
	return &AlertManager{
		app: app,
	}
}

func (am *AlertManager) HandleSystemAlerts(systemRecord *core.Record, systemInfo system.Info, temperatures map[string]float64, extraFs map[string]*system.FsStats) error {
	// start := time.Now()
	// defer func() {
	// 	log.Println("alert stats took", time.Since(start))
	// }()
	alertRecords, err := am.app.FindAllRecords("alerts",
		dbx.NewExp("system={:system}", dbx.Params{"system": systemRecord.Id}),
	)
	if err != nil || len(alertRecords) == 0 {
		// log.Println("no alerts found for system")
		return nil
	}

	var validAlerts []SystemAlertData
	now := systemRecord.GetDateTime("updated").Time().UTC()
	oldestTime := now

	for _, alertRecord := range alertRecords {
		name := alertRecord.GetString("name")
		var val float64
		unit := "%"

		switch name {
		case "CPU":
			val = systemInfo.Cpu
		case "Memory":
			val = systemInfo.MemPct
		case "Bandwidth":
			val = systemInfo.Bandwidth
			unit = " MB/s"
		case "Disk":
			maxUsedPct := systemInfo.DiskPct
			for _, fs := range extraFs {
				usedPct := fs.DiskUsed / fs.DiskTotal * 100
				if usedPct > maxUsedPct {
					maxUsedPct = usedPct
				}
			}
			val = maxUsedPct
		case "Temperature":
			if temperatures == nil {
				continue
			}
			for _, temp := range temperatures {
				if temp > val {
					val = temp
				}
			}
			unit = "°C"
		}

		triggered := alertRecord.GetBool("triggered")
		threshold := alertRecord.GetFloat("value")

		// CONTINUE
		// IF alert is not triggered and curValue is less than threshold
		// OR alert is triggered and curValue is greater than threshold
		if (!triggered && val <= threshold) || (triggered && val > threshold) {
			// log.Printf("Skipping alert %s: val %f | threshold %f | triggered %v\n", name, val, threshold, triggered)
			continue
		}

		min := max(1, cast.ToUint8(alertRecord.Get("min")))
		// add time to alert time to make sure it's slighty after record creation
		time := now.Add(-time.Duration(min) * time.Minute)
		if time.Before(oldestTime) {
			oldestTime = time
		}

		validAlerts = append(validAlerts, SystemAlertData{
			systemRecord: systemRecord,
			alertRecord:  alertRecord,
			name:         name,
			unit:         unit,
			val:          val,
			threshold:    threshold,
			triggered:    triggered,
			time:         time,
			min:          min,
		})
	}

	systemStats := []struct {
		Stats   []byte         `db:"stats"`
		Created types.DateTime `db:"created"`
	}{}

	err = am.app.DB().
		Select("stats", "created").
		From("system_stats").
		Where(dbx.NewExp(
			"system={:system} AND type='1m' AND created > {:created}",
			dbx.Params{
				"system": systemRecord.Id,
				// subtract some time to give us a bit of buffer
				"created": oldestTime.Add(-time.Second * 90),
			},
		)).
		OrderBy("created").
		All(&systemStats)
	if err != nil {
		return err
	}

	// get oldest record creation time from first record in the slice
	oldestRecordTime := systemStats[0].Created.Time()
	// log.Println("oldestRecordTime", oldestRecordTime.String())

	// delete from validAlerts if time is older than oldestRecord
	for i := 0; i < len(validAlerts); i++ {
		if validAlerts[i].time.Before(oldestRecordTime) {
			// log.Println("deleting alert - time is older than oldestRecord", validAlerts[i].name, oldestRecordTime, validAlerts[i].time)
			validAlerts = append(validAlerts[:i], validAlerts[i+1:]...)
		}
	}

	if len(validAlerts) == 0 {
		// log.Println("no valid alerts found")
		return nil
	}

	var stats SystemAlertStats

	// we can skip the latest systemStats record since it's the current value
	for i := 0; i < len(systemStats); i++ {
		stat := systemStats[i]
		// subtract 10 seconds to give a small time buffer
		systemStatsCreation := stat.Created.Time().Add(-time.Second * 10)
		if err := json.Unmarshal(stat.Stats, &stats); err != nil {
			return err
		}
		// log.Println("stats", stats)
		for j := range validAlerts {
			alert := &validAlerts[j]
			// reset alert val on first iteration
			if i == 0 {
				alert.val = 0
			}
			// continue if system_stats is older than alert time range
			if systemStatsCreation.Before(alert.time) {
				continue
			}
			// add to alert value
			switch alert.name {
			case "CPU":
				alert.val += stats.Cpu
			case "Memory":
				alert.val += stats.Mem
			case "Bandwidth":
				alert.val += stats.NetSent + stats.NetRecv
			case "Disk":
				if alert.mapSums == nil {
					alert.mapSums = make(map[string]float32, len(extraFs)+1)
				}
				// add root disk
				if _, ok := alert.mapSums["root"]; !ok {
					alert.mapSums["root"] = 0.0
				}
				alert.mapSums["root"] += float32(stats.Disk)
				// add extra disks
				for key, fs := range extraFs {
					if _, ok := alert.mapSums[key]; !ok {
						alert.mapSums[key] = 0.0
					}
					alert.mapSums[key] += float32(fs.DiskUsed / fs.DiskTotal * 100)
				}
			case "Temperature":
				if alert.mapSums == nil {
					alert.mapSums = make(map[string]float32, len(stats.Temperatures))
				}
				for key, temp := range stats.Temperatures {
					if _, ok := alert.mapSums[key]; !ok {
						alert.mapSums[key] = float32(0)
					}
					alert.mapSums[key] += temp
				}
			default:
				continue
			}
			alert.count++
		}
	}
	// sum up vals for each alert
	for _, alert := range validAlerts {
		switch alert.name {
		case "Disk":
			maxPct := float32(0)
			for key, value := range alert.mapSums {
				sumPct := float32(value)
				if sumPct > maxPct {
					maxPct = sumPct
					alert.descriptor = fmt.Sprintf("Usage of %s", key)
				}
			}
			alert.val = float64(maxPct / float32(alert.count))
		case "Temperature":
			maxTemp := float32(0)
			for key, value := range alert.mapSums {
				sumTemp := float32(value) / float32(alert.count)
				if sumTemp > maxTemp {
					maxTemp = sumTemp
					alert.descriptor = fmt.Sprintf("Highest sensor %s", key)
				}
			}
			alert.val = float64(maxTemp)
		default:
			alert.val = alert.val / float64(alert.count)
		}
		minCount := float32(alert.min) / 1.2
		// log.Println("alert", alert.name, "val", alert.val, "threshold", alert.threshold, "triggered", alert.triggered)
		// log.Printf("%s: val %f | count %d | min-count %f | threshold %f\n", alert.name, alert.val, alert.count, minCount, alert.threshold)
		// pass through alert if count is greater than or equal to minCount
		if float32(alert.count) >= minCount {
			if !alert.triggered && alert.val > alert.threshold {
				alert.triggered = true
				go am.sendSystemAlert(alert)
			} else if alert.triggered && alert.val <= alert.threshold {
				alert.triggered = false
				go am.sendSystemAlert(alert)
			}
		}
	}
	return nil
}

func (am *AlertManager) sendSystemAlert(alert SystemAlertData) {
	// log.Printf("Sending alert %s: val %f | count %d | threshold %f\n", alert.name, alert.val, alert.count, alert.threshold)
	systemName := alert.systemRecord.GetString("name")

	// change Disk to Disk usage
	if alert.name == "Disk" {
		alert.name += " usage"
	}

	// make title alert name lowercase if not CPU
	titleAlertName := alert.name
	if titleAlertName != "CPU" {
		titleAlertName = strings.ToLower(titleAlertName)
	}

	var subject string
	if alert.triggered {
		subject = fmt.Sprintf("%s %s above threshold", systemName, titleAlertName)
	} else {
		subject = fmt.Sprintf("%s %s below threshold", systemName, titleAlertName)
	}
	minutesLabel := "minute"
	if alert.min > 1 {
		minutesLabel += "s"
	}
	if alert.descriptor == "" {
		alert.descriptor = alert.name
	}
	body := fmt.Sprintf("%s averaged %.2f%s for the previous %v %s.", alert.descriptor, alert.val, alert.unit, alert.min, minutesLabel)

	alert.alertRecord.Set("triggered", alert.triggered)
	if err := am.app.Save(alert.alertRecord); err != nil {
		// app.Logger().Error("failed to save alert record", "err", err.Error())
		return
	}
	// expand the user relation and send the alert
	if errs := am.app.ExpandRecord(alert.alertRecord, []string{"user"}, nil); len(errs) > 0 {
		// app.Logger().Error("failed to expand user relation", "errs", errs)
		return
	}
	if user := alert.alertRecord.ExpandedOne("user"); user != nil {
		am.sendAlert(AlertMessageData{
			UserID:   user.Id,
			Title:    subject,
			Message:  body,
			Link:     am.app.Settings().Meta.AppURL + "/system/" + url.PathEscape(systemName),
			LinkText: "View " + systemName,
		})
	}
}

// todo: allow x minutes downtime before sending alert
func (am *AlertManager) HandleStatusAlerts(newStatus string, oldSystemRecord *core.Record) error {
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
	alertRecords, err := am.app.FindAllRecords("alerts",
		dbx.HashExp{
			"system": oldSystemRecord.Id,
			"name":   "Status",
		},
	)
	if err != nil || len(alertRecords) == 0 {
		// log.Println("no alerts found for system")
		return nil
	}
	for _, alertRecord := range alertRecords {
		// expand the user relation
		if errs := am.app.ExpandRecord(alertRecord, []string{"user"}, nil); len(errs) > 0 {
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
		am.sendAlert(AlertMessageData{
			UserID:   user.Id,
			Title:    fmt.Sprintf("Connection to %s is %s %v", systemName, alertStatus, emoji),
			Message:  fmt.Sprintf("Connection to %s is %s", systemName, alertStatus),
			Link:     am.app.Settings().Meta.AppURL + "/system/" + url.PathEscape(systemName),
			LinkText: "View " + systemName,
		})
	}
	return nil
}

func (am *AlertManager) sendAlert(data AlertMessageData) {
	// get user settings
	record, err := am.app.FindFirstRecordByFilter(
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

// Contains checks if a string is present in a slice of strings
func sliceContains(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
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
	err := am.SendShoutrrrAlert(url, "Test Alert", "This is a notification from Beszel.", am.app.Settings().Meta.AppURL, "View Beszel")
	if err != nil {
		return e.JSON(200, map[string]string{"err": err.Error()})
	}
	return e.JSON(200, map[string]bool{"err": false})
}
