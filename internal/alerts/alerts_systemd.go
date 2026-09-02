package alerts

import (
	"fmt"
	"strings"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/entities/systemd"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// alertNameSystemdFailed is the alerts.name value for the failed systemd services alert.
const alertNameSystemdFailed = "SystemdFailed"

// maxListedServices caps how many service names are listed in a notification body.
const maxListedServices = 10

// HandleSystemdAlerts manages alerts for systemd services in the failed state.
//
// This is a binary state alert and fires on the first observation of a failed
// service rather than using a delay. The agent only refreshes systemd state every
// 10 minutes, so a shorter delay could never observe new data before expiring, and
// that poll interval already hides services that fail and restart quickly.
func (am *AlertManager) HandleSystemdAlerts(systemRecord *core.Record) error {
	alerts := am.alertsCache.GetAlertsByName(systemRecord.Id, alertNameSystemdFailed)
	if len(alerts) == 0 {
		return nil
	}

	// State is read from the systemd_services snapshot rather than the update payload.
	// The payload is not a reliable source here: realtime dashboard subscriptions fetch
	// from the agent with a shorter cache time, and the agent omits systemd services from
	// those responses, overwriting the cached payload roughly once a second while a system
	// is being viewed. The snapshot table is only written by the full update cycle.
	total, failed, err := am.queryServiceStates(systemRecord.Id)
	if err != nil {
		return err
	}
	if total == 0 {
		// No rows normally means no systemd data for this system (agent without
		// systemd, or not yet reported), which must not be treated as a recovery.
		// Read info only in this ambiguous case. The record being saved is used
		// instead of data because dashboard polling can replace the system's
		// in-memory payload concurrently.
		var currentInfo system.Info
		if err := systemRecord.UnmarshalJSONField("info", &currentInfo); err != nil ||
			len(currentInfo.Services) == 0 || currentInfo.Services[0] != 0 {
			return nil
		}
	}

	systemName := systemRecord.GetString("name")

	for _, alertData := range alerts {
		triggered := len(failed) > 0
		// Only notify on a change of state, so a service that stays failed across
		// cycles doesn't re-notify every update.
		if triggered == alertData.Triggered {
			continue
		}
		if err := am.sendSystemdAlert(triggered, systemName, alertData, failed); err != nil {
			am.hub.Logger().Error("Failed to send alert", "err", err)
		}
	}
	return nil
}

// queryServiceStates returns the number of services reported in the most recent update
// for a system, and the names of those in the failed state.
//
// Rows are restricted to the latest update because systemd_services is upserted, never
// pruned on change: a service that no longer exists on the host stops being reported and
// its row keeps its last known state until the retention sweep removes it. Every row
// written in one cycle shares a single updated timestamp, so the newest timestamp
// identifies exactly the services the agent last reported.
func (am *AlertManager) queryServiceStates(systemID string) (total int, failed []string, err error) {
	var rows []struct {
		Name  string               `db:"name"`
		State systemd.ServiceState `db:"state"`
	}
	err = am.hub.DB().
		Select("name", "state").
		From("systemd_services").
		Where(dbx.NewExp(
			"system={:system} AND updated=(SELECT MAX(updated) FROM systemd_services WHERE system={:system})",
			dbx.Params{"system": systemID},
		)).
		OrderBy("name").
		All(&rows)
	if err != nil {
		return 0, nil, err
	}
	for _, row := range rows {
		if row.State == systemd.StatusFailed {
			failed = append(failed, row.Name)
		}
	}
	return len(rows), failed, nil
}

// sendSystemdAlert sends a failed or recovered systemd services alert to the alert's user.
func (am *AlertManager) sendSystemdAlert(triggered bool, systemName string, alertData CachedAlertData, failed []string) error {
	// Update trigger state for alert record before sending alert
	if err := am.setAlertTriggered(alertData, triggered); err != nil {
		return err
	}

	var title, message string
	if triggered {
		title = fmt.Sprintf("Failed services on %s %v", systemName, "\U0001F534") // Red alert emoji
		message = fmt.Sprintf("%s on %s: %s", pluralizeServices(len(failed)), systemName, formatServiceList(failed))
	} else {
		title = fmt.Sprintf("Services recovered on %s %v", systemName, "✅") // Green checkmark emoji
		message = fmt.Sprintf("No services are in the failed state on %s.", systemName)
	}

	systemID := alertData.SystemID

	return am.SendAlert(AlertMessageData{
		UserID:   alertData.UserID,
		SystemID: systemID,
		Title:    title,
		Message:  message,
		Link:     am.hub.MakeLink("system", systemID),
		LinkText: "View " + systemName,
	})
}

// pluralizeServices returns a count label like "1 failed service" or "3 failed services".
func pluralizeServices(count int) string {
	if count == 1 {
		return "1 failed service"
	}
	return fmt.Sprintf("%d failed services", count)
}

// formatServiceList joins service names, truncating long lists.
func formatServiceList(names []string) string {
	if len(names) <= maxListedServices {
		return strings.Join(names, ", ")
	}
	remaining := len(names) - maxListedServices
	return fmt.Sprintf("%s and %d more", strings.Join(names[:maxListedServices], ", "), remaining)
}

// resolveSystemdAlerts resolves triggered systemd alerts for systems that no longer
// have any failed services. This clears stale state left by a hub restart.
func resolveSystemdAlerts(app core.App) error {
	db := app.DB()
	var alertIds []string
	err := db.NewQuery(`
		SELECT a.id
		FROM alerts a
		JOIN systems sys ON sys.id = a.system
		WHERE a.name = {:name}
		AND a.triggered = true
		AND (
			EXISTS (
				SELECT 1 FROM systemd_services cur
				WHERE cur.system = a.system
				AND cur.updated = (SELECT MAX(updated) FROM systemd_services WHERE system = a.system)
			)
			OR json_extract(sys.info, '$.sv[0]') = 0
		)
		AND NOT EXISTS (
			SELECT 1 FROM systemd_services s
			WHERE s.system = a.system AND s.state = {:state}
			AND s.updated = (SELECT MAX(updated) FROM systemd_services WHERE system = a.system)
		)
	`).Bind(dbx.Params{
		"name":  alertNameSystemdFailed,
		"state": systemd.StatusFailed,
	}).Column(&alertIds)
	if err != nil {
		return err
	}
	for _, alertId := range alertIds {
		alert, err := app.FindRecordById("alerts", alertId)
		if err != nil {
			return err
		}
		alert.Set("triggered", false)
		if err := app.Save(alert); err != nil {
			return err
		}
	}
	return nil
}
