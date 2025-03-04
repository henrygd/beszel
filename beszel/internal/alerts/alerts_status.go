package alerts

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type alertTask struct {
	action      string // "schedule" or "cancel"
	systemName  string
	alertRecord *core.Record
	delay       time.Duration
}

type alertInfo struct {
	systemName  string
	alertRecord *core.Record
	expireTime  time.Time
}

// startWorker is a long-running goroutine that processes alert tasks
// every x seconds. It must be running to process status alerts.
func (am *AlertManager) startWorker() {
	tick := time.Tick(15 * time.Second)
	for {
		select {
		case <-am.stopChan:
			return
		case task := <-am.alertQueue:
			switch task.action {
			case "schedule":
				am.pendingAlerts.Store(task.alertRecord.Id, &alertInfo{
					systemName:  task.systemName,
					alertRecord: task.alertRecord,
					expireTime:  time.Now().Add(task.delay),
				})
			case "cancel":
				am.pendingAlerts.Delete(task.alertRecord.Id)
			}
		case <-tick:
			// Check for expired alerts every tick
			now := time.Now()
			for key, value := range am.pendingAlerts.Range {
				info := value.(*alertInfo)
				if now.After(info.expireTime) {
					// Downtime delay has passed, process alert
					am.sendStatusAlert("down", info.systemName, info.alertRecord)
					am.pendingAlerts.Delete(key)
				}
			}
		}
	}
}

// StopWorker shuts down the AlertManager.worker goroutine
func (am *AlertManager) StopWorker() {
	close(am.stopChan)
}

// HandleStatusAlerts manages the logic when system status changes.
func (am *AlertManager) HandleStatusAlerts(newStatus string, systemRecord *core.Record) error {
	if newStatus != "up" && newStatus != "down" {
		return nil
	}

	alertRecords, err := am.getSystemStatusAlerts(systemRecord.Id)
	if err != nil {
		return err
	}
	if len(alertRecords) == 0 {
		return nil
	}

	systemName := systemRecord.GetString("name")
	if newStatus == "down" {
		am.handleSystemDown(systemName, alertRecords)
	} else {
		am.handleSystemUp(systemName, alertRecords)
	}
	return nil
}

// getSystemStatusAlerts retrieves all "Status" alert records for a given system ID.
func (am *AlertManager) getSystemStatusAlerts(systemID string) ([]*core.Record, error) {
	alertRecords, err := am.app.FindAllRecords("alerts", dbx.HashExp{
		"system": systemID,
		"name":   "Status",
	})
	if err != nil {
		return nil, err
	}
	return alertRecords, nil
}

// Schedules delayed "down" alerts for each alert record.
func (am *AlertManager) handleSystemDown(systemName string, alertRecords []*core.Record) {
	for _, alertRecord := range alertRecords {
		// Continue if alert is already scheduled
		if _, exists := am.pendingAlerts.Load(alertRecord.Id); exists {
			continue
		}
		// Schedule by adding to queue
		min := max(1, alertRecord.GetInt("min"))
		am.alertQueue <- alertTask{
			action:      "schedule",
			systemName:  systemName,
			alertRecord: alertRecord,
			delay:       time.Duration(min) * time.Minute,
		}
	}
}

// handleSystemUp manages the logic when a system status changes to "up".
// It cancels any pending alerts and sends "up" alerts.
func (am *AlertManager) handleSystemUp(systemName string, alertRecords []*core.Record) {
	for _, alertRecord := range alertRecords {
		alertRecordID := alertRecord.Id
		// If alert exists for record, delete and continue (down alert not sent)
		if _, exists := am.pendingAlerts.Load(alertRecordID); exists {
			am.alertQueue <- alertTask{
				action:      "cancel",
				alertRecord: alertRecord,
			}
			continue
		}
		// No alert scheduled for this record, send "up" alert
		if err := am.sendStatusAlert("up", systemName, alertRecord); err != nil {
			am.app.Logger().Error("Failed to send alert", "err", err.Error())
		}
	}
}

// sendStatusAlert sends a status alert ("up" or "down") to the users associated with the alert records.
func (am *AlertManager) sendStatusAlert(alertStatus string, systemName string, alertRecord *core.Record) error {
	var emoji string
	if alertStatus == "up" {
		emoji = "\u2705" // Green checkmark emoji
	} else {
		emoji = "\U0001F534" // Red alert emoji
	}

	title := fmt.Sprintf("Connection to %s is %s %v", systemName, alertStatus, emoji)
	message := strings.TrimSuffix(title, emoji)

	if errs := am.app.ExpandRecord(alertRecord, []string{"user"}, nil); len(errs) > 0 {
		return errs["user"]
	}
	user := alertRecord.ExpandedOne("user")
	if user == nil {
		return nil
	}

	return am.SendAlert(AlertMessageData{
		UserID:   user.Id,
		Title:    title,
		Message:  message,
		Link:     am.app.Settings().Meta.AppURL + "/system/" + url.PathEscape(systemName),
		LinkText: "View " + systemName,
	})
}
