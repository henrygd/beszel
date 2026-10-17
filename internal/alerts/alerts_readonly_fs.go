package alerts

import (
	"fmt"
	"strings"

	"github.com/henrygd/beszel/internal/entities/system"

	"github.com/pocketbase/pocketbase/core"
)

// alertNameFilesystemReadOnly is the alerts.name value for unexpected RO mounts.
const alertNameFilesystemReadOnly = "FilesystemReadOnly"

// HandleFilesystemReadOnlyAlerts manages binary alerts when a watched host
// filesystem is unexpectedly read-only. Fires immediately on state change.
func (am *AlertManager) HandleFilesystemReadOnlyAlerts(systemRecord *core.Record, data *system.CombinedData) error {
	if data == nil {
		return nil
	}
	alerts := am.alertsCache.GetAlertsByName(systemRecord.Id, alertNameFilesystemReadOnly)
	if len(alerts) == 0 {
		return nil
	}

	roNames := data.Info.ReadOnlyFilesystems
	if len(roNames) == 0 {
		// Rebuild from flags when the agent omitted the list.
		if data.Info.RootReadOnly {
			label := data.Info.RootDiskName
			if label == "" {
				label = "root"
			}
			roNames = append(roNames, label)
		}
		for name, ro := range data.Info.ExtraFsReadOnly {
			if ro {
				roNames = append(roNames, name)
			}
		}
	}

	triggered := len(roNames) > 0
	systemName := systemRecord.GetString("name")

	for _, alertData := range alerts {
		if triggered == alertData.Triggered {
			continue
		}
		if err := am.sendFilesystemReadOnlyAlert(triggered, systemName, alertData, roNames); err != nil {
			am.hub.Logger().Error("Failed to send filesystem read-only alert", "err", err)
		}
	}
	return nil
}

func (am *AlertManager) sendFilesystemReadOnlyAlert(triggered bool, systemName string, alertData CachedAlertData, names []string) error {
	if err := am.setAlertTriggered(alertData, triggered); err != nil {
		return err
	}

	var title, message string
	if triggered {
		title = fmt.Sprintf("%s filesystem read-only", systemName)
		message = fmt.Sprintf("Unexpected read-only filesystem(s) on %s: %s", systemName, formatReadOnlyList(names))
	} else {
		title = fmt.Sprintf("%s filesystems writable again", systemName)
		message = fmt.Sprintf("Monitored filesystems on %s are writable again.", systemName)
	}

	return am.SendAlert(AlertMessageData{
		UserID:   alertData.UserID,
		SystemID: alertData.SystemID,
		Title:    title,
		Message:  message,
		Link:     am.hub.MakeLink("system", alertData.SystemID),
		LinkText: "View " + systemName,
	})
}

func formatReadOnlyList(names []string) string {
	if len(names) == 0 {
		return "unknown"
	}
	if len(names) <= maxListedServices {
		return strings.Join(names, ", ")
	}
	remaining := len(names) - maxListedServices
	return fmt.Sprintf("%s and %d more", strings.Join(names[:maxListedServices], ", "), remaining)
}
