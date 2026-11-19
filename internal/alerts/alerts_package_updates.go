package alerts

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

const packageUpdateAlertName = "PackageUpdates"

// HandlePackageUpdateAlerts sends an alert when pending package updates remain
// available for the configured duration. The pending timestamp is persisted so
// the duration survives hub restarts.
func (am *AlertManager) HandlePackageUpdateAlerts(systemRecord *core.Record, data *system.CombinedData) error {
	if data == nil {
		return nil
	}
	alerts := am.alertsCache.GetAlertsByName(systemRecord.Id, packageUpdateAlertName)
	if len(alerts) == 0 {
		return nil
	}
	// An omitted field means the agent has not completed a package update check
	// or does not support it; it must not resolve an existing alert.
	if len(data.Info.PackageUpdates) == 0 {
		return nil
	}

	updateCount := int(data.Info.PackageUpdates[0])
	now := time.Now().UTC()
	if updated := systemRecord.GetDateTime("updated").Time().UTC(); !updated.IsZero() &&
		updated.After(now.Add(-time.Hour)) && updated.Before(now.Add(time.Hour)) {
		now = updated
	}
	var result error

	for _, alertData := range alerts {
		lockValue, _ := am.packageLocks.LoadOrStore(alertData.Id, &sync.Mutex{})
		lock := lockValue.(*sync.Mutex)
		lock.Lock()
		if updateCount > 0 {
			if alertData.Triggered {
				lock.Unlock()
				continue
			}
			pendingSince := alertData.PendingSince
			if pendingSince.IsZero() {
				var err error
				pendingSince, err = am.setPendingAlert(alertData, now)
				if err != nil {
					result = errors.Join(result, err)
					lock.Unlock()
					continue
				}
			}
			if pendingSince.IsZero() {
				lock.Unlock()
				continue
			}
			min := int(alertData.Min)
			if min > 0 && now.Before(pendingSince.Add(time.Duration(min)*time.Minute)) {
				lock.Unlock()
				continue
			}
			claimed, err := am.claimPendingPackageUpdateAlert(alertData)
			if err != nil {
				result = errors.Join(result, err)
				lock.Unlock()
				continue
			}
			if !claimed {
				lock.Unlock()
				continue
			}
			if err := am.sendPackageUpdateAlert(systemRecord, alertData, updateCount, true); err != nil {
				result = errors.Join(result, err)
			}
			lock.Unlock()
			continue
		}

		if err := am.clearPendingAlert(alertData); err != nil {
			result = errors.Join(result, err)
		}
		if alertData.Triggered {
			if err := am.sendPackageUpdateAlert(systemRecord, alertData, 0, false); err != nil {
				result = errors.Join(result, err)
			}
		}
		lock.Unlock()
	}
	return result
}

func (am *AlertManager) claimPendingPackageUpdateAlert(alertData CachedAlertData) (bool, error) {
	result, err := am.hub.DB().NewQuery(`
		UPDATE alerts
		SET triggered = true, pending_since = {:zero}
		WHERE id = {:id}
		AND triggered = false
		AND pending_since != {:zero}
	`).Bind(dbx.Params{
		"id":   alertData.Id,
		"zero": time.Time{},
	}).Execute()
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err == nil && rows == 1 {
		record, findErr := am.hub.FindRecordById("alerts", alertData.Id)
		if findErr != nil {
			return false, findErr
		}
		am.alertsCache.Update(record)
	}
	return rows == 1, err
}

func (am *AlertManager) sendPackageUpdateAlert(systemRecord *core.Record, alertData CachedAlertData, count int, triggered bool) error {
	if !triggered {
		if err := am.setAlertTriggered(alertData, false); err != nil {
			return err
		}
	}
	systemName := systemRecord.GetString("name")
	title := fmt.Sprintf("%s has pending package updates", systemName)
	updateLabel := "package updates"
	if count == 1 {
		updateLabel = "package update"
	}
	verb := "are"
	if count == 1 {
		verb = "is"
	}
	message := fmt.Sprintf("%d %s %s available.", count, updateLabel, verb)
	if !triggered {
		title = fmt.Sprintf("%s package updates resolved", systemName)
		message = "No package updates are available."
	}
	return am.SendAlert(AlertMessageData{
		UserID:   alertData.UserID,
		SystemID: systemRecord.Id,
		Title:    title,
		Message:  message,
		Link:     am.hub.MakeLink("system", systemRecord.Id),
		LinkText: "View " + systemName,
	})
}

func (am *AlertManager) CancelPendingPackageUpdateAlerts(systemID string) {
	for _, alertData := range am.alertsCache.GetAlertsByName(systemID, packageUpdateAlertName) {
		if err := am.clearPendingAlert(alertData); err != nil {
			am.hub.Logger().Error("Failed to clear pending package update alert", "err", err)
		}
	}
}
