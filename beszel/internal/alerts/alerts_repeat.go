package alerts

import (
	"fmt"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// checkAndSendRepeatingAlerts checks for triggered alerts that should send repeat notifications
func (am *AlertManager) checkAndSendRepeatingAlerts() {
	// Find all triggered alerts that have repeat configuration
	alertRecords, err := am.hub.FindAllRecords("alerts",
		dbx.NewExp("triggered=true AND repeat_interval>0"),
	)
	if err != nil || len(alertRecords) == 0 {
		return
	}

	now := time.Now().UTC()

	for _, alertRecord := range alertRecords {
		repeatInterval := int(alertRecord.GetFloat("repeat_interval"))
		maxRepeats := int(alertRecord.GetFloat("max_repeats"))
		repeatCount := int(alertRecord.GetFloat("repeat_count"))
		lastSentStr := alertRecord.GetString("last_sent")

		// Skip if we've reached max repeats (and max_repeats > 0)
		if maxRepeats > 0 && repeatCount >= maxRepeats {
			continue
		}

		// Parse last sent time
		var lastSent time.Time
		if lastSentStr != "" {
			lastSentDateTime, err := types.ParseDateTime(lastSentStr)
			if err == nil {
				lastSent = lastSentDateTime.Time()
			}
		}

		// Check if enough time has passed since last alert
		if !lastSent.IsZero() {
			nextSendTime := lastSent.Add(time.Duration(repeatInterval) * time.Minute)
			if now.Before(nextSendTime) {
				continue // Not time to send yet
			}
		}

		// Send the repeat alert
		am.sendRepeatingAlert(alertRecord)
	}
}

// sendRepeatingAlert sends a repeat alert and updates the tracking fields
func (am *AlertManager) sendRepeatingAlert(alertRecord *core.Record) {
	// Get system record for the alert
	systemRecord, err := am.hub.FindRecordById("systems", alertRecord.GetString("system"))
	if err != nil {
		am.hub.Logger().Error("Failed to find system for repeating alert", "err", err, "alert_id", alertRecord.Id)
		return
	}

	systemName := systemRecord.GetString("name")
	alertName := alertRecord.GetString("name")
	threshold := alertRecord.GetFloat("value")
	repeatCount := int(alertRecord.GetFloat("repeat_count"))

	// Format alert name for display
	titleAlertName := alertName
	if titleAlertName != "CPU" {
		titleAlertName = strings.ToLower(titleAlertName)
	}
	if alertName == "Disk" {
		alertName += " usage"
	}
	// Format LoadAvg5 and LoadAvg15
	if after, ok := strings.CutPrefix(alertName, "LoadAvg"); ok {
		alertName = after + "m Load"
	}

	subject := fmt.Sprintf("%s %s still above threshold (repeat %d)", systemName, titleAlertName, repeatCount+1)
	body := fmt.Sprintf("%s is still above the threshold of %.2f. This is repeat notification #%d.", alertName, threshold, repeatCount+1)

	// Update tracking fields
	alertRecord.Set("repeat_count", repeatCount+1)
	alertRecord.Set("last_sent", types.NowDateTime())

	if err := am.hub.Save(alertRecord); err != nil {
		am.hub.Logger().Error("Failed to update repeating alert record", "err", err, "alert_id", alertRecord.Id)
		return
	}

	// Send the alert
	am.SendAlert(AlertMessageData{
		UserID:   alertRecord.GetString("user"),
		Title:    subject,
		Message:  body,
		Link:     am.hub.MakeLink("system", systemName),
		LinkText: "View " + systemName,
	})

	am.hub.Logger().Info("Sent repeating alert", "system", systemName, "alert", alertName, "repeat_count", repeatCount+1)
}

// StartRepeatingAlertChecker starts a goroutine that periodically checks for repeating alerts
func (am *AlertManager) StartRepeatingAlertChecker() {
	ticker := time.NewTicker(1 * time.Minute) // Check every minute
	go func() {
		for {
			select {
			case <-ticker.C:
				am.checkAndSendRepeatingAlerts()
			case <-am.stopChan:
				ticker.Stop()
				return
			}
		}
	}()
}