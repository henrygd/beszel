package alerts

import (
	"errors"
	"fmt"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
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
	var result error

	for _, alertData := range alerts {
		if updateCount > 0 {
			if alertData.Triggered {
				continue
			}
			pendingSince, err := am.setPendingPackageUpdateAlert(alertData, now)
			if err != nil {
				result = errors.Join(result, err)
				continue
			}
			if pendingSince.IsZero() {
				continue
			}
			min := int(alertData.Min)
			if min > 0 && now.Before(pendingSince.Add(time.Duration(min)*time.Minute)) {
				continue
			}
			if err := am.sendPackageUpdateAlert(systemRecord, alertData, updateCount, true); err != nil {
				result = errors.Join(result, err)
			}
			continue
		}

		if err := am.clearPendingPackageUpdateAlert(alertData); err != nil {
			result = errors.Join(result, err)
		}
		if alertData.Triggered {
			if err := am.sendPackageUpdateAlert(systemRecord, alertData, 0, false); err != nil {
				result = errors.Join(result, err)
			}
		}
	}
	return result
}

func (am *AlertManager) setPendingPackageUpdateAlert(alertData CachedAlertData, since time.Time) (time.Time, error) {
	record, err := am.hub.FindRecordById("alerts", alertData.Id)
	if err != nil {
		return time.Time{}, err
	}
	if record.GetBool("triggered") {
		return time.Time{}, nil
	}
	if pendingSince := record.GetDateTime("pending_since").Time(); !pendingSince.IsZero() {
		return pendingSince, nil
	}
	record.Set("pending_since", since.Truncate(time.Millisecond))
	return since, am.hub.Save(record)
}

func (am *AlertManager) clearPendingPackageUpdateAlert(alertData CachedAlertData) error {
	if alertData.PendingSince.IsZero() {
		return nil
	}
	record, err := am.hub.FindRecordById("alerts", alertData.Id)
	if err != nil {
		return err
	}
	if record.GetDateTime("pending_since").Time().IsZero() {
		return nil
	}
	record.Set("pending_since", nil)
	return am.hub.Save(record)
}

func (am *AlertManager) sendPackageUpdateAlert(systemRecord *core.Record, alertData CachedAlertData, count int, triggered bool) error {
	if err := am.setAlertTriggered(alertData, triggered); err != nil {
		return err
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
