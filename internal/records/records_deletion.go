package records

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// Delete old records
func (rm *RecordManager) DeleteOldRecords() {
	rm.app.RunInTransaction(func(txApp core.App) error {
		err := deleteOldSystemStats(txApp)
		if err != nil {
			slog.Error("Error deleting old system stats", "err", err)
		}
		err = deleteOldContainerRecords(txApp)
		if err != nil {
			slog.Error("Error deleting old container records", "err", err)
		}
		err = deleteOldSystemdServiceRecords(txApp)
		if err != nil {
			slog.Error("Error deleting old systemd service records", "err", err)
		}
		err = deleteOldAlertsHistory(txApp, 200, 250)
		if err != nil {
			slog.Error("Error deleting old alerts history", "err", err)
		}
		err = deleteOldQuietHours(txApp)
		if err != nil {
			slog.Error("Error deleting old quiet hours", "err", err)
		}
		return nil
	})
}

// Delete old alerts history records
func deleteOldAlertsHistory(app core.App, countToKeep, countBeforeDeletion int) error {
	db := app.DB()
	var users []struct {
		Id string `db:"user"`
	}
	err := db.NewQuery("SELECT user, COUNT(*) as count FROM alerts_history GROUP BY user HAVING count > {:countBeforeDeletion}").Bind(dbx.Params{"countBeforeDeletion": countBeforeDeletion}).All(&users)
	if err != nil {
		return err
	}
	for _, user := range users {
		_, err = db.NewQuery("DELETE FROM alerts_history WHERE user = {:user} AND id NOT IN (SELECT id FROM alerts_history WHERE user = {:user} ORDER BY created DESC LIMIT {:countToKeep})").Bind(dbx.Params{"user": user.Id, "countToKeep": countToKeep}).Execute()
		if err != nil {
			return err
		}
	}
	return nil
}

// Deletes system_stats records older than what is displayed in the UI
func deleteOldSystemStats(app core.App) error {
	// Collections to process
	collections := [2]string{"system_stats", "container_stats"}

	// Record types and their retention periods
	type RecordDeletionData struct {
		recordType string
		retention  time.Duration
	}
	recordData := []RecordDeletionData{
		{recordType: "1m", retention: time.Hour},             // 1 hour
		{recordType: "10m", retention: 12 * time.Hour},       // 12 hours
		{recordType: "20m", retention: 24 * time.Hour},       // 1 day
		{recordType: "120m", retention: 7 * 24 * time.Hour},  // 7 days
		{recordType: "480m", retention: 30 * 24 * time.Hour}, // 30 days
	}

	now := time.Now().UTC()

	for _, collection := range collections {
		// Build the WHERE clause
		var conditionParts []string
		var params dbx.Params = make(map[string]any)
		for i := range recordData {
			rd := recordData[i]
			// Create parameterized condition for this record type
			dateParam := fmt.Sprintf("date%d", i)
			conditionParts = append(conditionParts, fmt.Sprintf("(type = '%s' AND created < {:%s})", rd.recordType, dateParam))
			params[dateParam] = now.Add(-rd.retention)
		}
		// Combine conditions with OR
		conditionStr := strings.Join(conditionParts, " OR ")
		// Construct and execute the full raw query
		rawQuery := fmt.Sprintf("DELETE FROM %s WHERE %s", collection, conditionStr)
		if _, err := app.DB().NewQuery(rawQuery).Bind(params).Execute(); err != nil {
			return fmt.Errorf("failed to delete from %s: %v", collection, err)
		}
	}
	return nil
}

// Deletes systemd service records that haven't been updated in the last 20 minutes
func deleteOldSystemdServiceRecords(app core.App) error {
	now := time.Now().UTC()
	twentyMinutesAgo := now.Add(-20 * time.Minute)

	// Delete systemd service records where updated < twentyMinutesAgo
	_, err := app.DB().NewQuery("DELETE FROM systemd_services WHERE updated < {:updated}").Bind(dbx.Params{"updated": twentyMinutesAgo.UnixMilli()}).Execute()
	if err != nil {
		return fmt.Errorf("failed to delete old systemd service records: %v", err)
	}

	return nil
}

// Deletes container records that haven't been updated in the last 10 minutes
func deleteOldContainerRecords(app core.App) error {
	now := time.Now().UTC()
	tenMinutesAgo := now.Add(-10 * time.Minute)

	// Delete container records where updated < tenMinutesAgo
	_, err := app.DB().NewQuery("DELETE FROM containers WHERE updated < {:updated}").Bind(dbx.Params{"updated": tenMinutesAgo.UnixMilli()}).Execute()
	if err != nil {
		return fmt.Errorf("failed to delete old container records: %v", err)
	}

	return nil
}

// Deletes old quiet hours records where end date has passed
func deleteOldQuietHours(app core.App) error {
	now := time.Now().UTC()
	_, err := app.DB().NewQuery("DELETE FROM quiet_hours WHERE type = 'one-time' AND end < {:now}").Bind(dbx.Params{"now": now}).Execute()
	if err != nil {
		return err
	}

	return nil
}
