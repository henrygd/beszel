package alerts

import (
	"fmt"
	"maps"
	"net/http"
	"strings"

	"github.com/henrygd/beszel"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// PreviewNotificationTemplate handles POST /api/beszel/notification-templates/preview.
// It renders the submitted template on top of the user's saved templates
// against sample data (using one of the user's systems when available) and
// optionally emails the result.
//
// Test emails go to the account address only; admins may also target their
// configured notification addresses.
func (am *AlertManager) PreviewNotificationTemplate(e *core.RequestEvent) error {
	var req struct {
		Kind      string `json:"kind"`
		Title     string `json:"title"`
		Body      string `json:"body"`
		Timezone  string `json:"timezone"`
		SendEmail bool   `json:"sendEmail"`
	}
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Bad data", err)
	}
	if req.Kind == "" {
		req.Kind = NotificationKindStatus
	}
	if !isNotificationKind(req.Kind) {
		return e.BadRequestError("Unknown notification kind", nil)
	}

	saved, err := am.getUserNotificationSettings(e.Auth.Id)
	if err != nil {
		am.hub.Logger().Debug("No saved notification settings for preview", "user", e.Auth.Id, "err", err)
	}
	// preview the submitted template exactly as it would run
	settings := UserNotificationSettings{
		Templates:  maps.Clone(saved.Templates),
		Timezone:   req.Timezone,
		HourFormat: saved.HourFormat,
	}
	if settings.Templates == nil {
		settings.Templates = map[string]NotificationTemplate{}
	}
	settings.Templates[req.Kind] = NotificationTemplate{Title: req.Title, Body: req.Body}
	if err := validateNotificationSettings(settings); err != nil {
		return e.BadRequestError(err.Error(), nil)
	}

	rendered := am.applyUserTemplates(settings, am.sampleAlertData(req.Kind, e.Auth.Id))
	resp := map[string]any{"title": rendered.Title, "body": rendered.Message}

	if req.SendEmail {
		isAdmin := e.Auth.IsSuperuser() || e.Auth.GetString("role") == "admin"
		if e.Auth.GetString("role") == "readonly" {
			return e.ForbiddenError("Read-only users cannot send test emails", nil)
		}
		recipients := []string{e.Auth.Email()}
		if isAdmin && len(saved.Emails) > 0 {
			recipients = saved.Emails
		}
		if err := am.sendEmailAlert(recipients, rendered); err != nil {
			am.hub.Logger().Error("Failed to send test email", "user", e.Auth.Id, "err", err)
			resp["err"] = "Failed to send test email"
			if isAdmin {
				resp["err"] = err.Error()
			}
		}
	}
	return e.JSON(http.StatusOK, resp)
}

// sampleAlertData builds representative alert data for previews. Example
// values are seeded for every {system.*} placeholder; when the user has at
// least one system, the values that its records provide replace them.
func (am *AlertManager) sampleAlertData(kind, userID string) AlertMessageData {
	systemName := "example-server"
	data := AlertMessageData{UserID: userID, Kind: kind, Vars: map[string]string{}}
	if sys, err := am.hub.FindFirstRecordByFilter("systems", "users.id ?= {:user}", dbx.Params{"user": userID}); err == nil {
		systemName = sys.GetString("name")
		data.SystemID = sys.Id
		data.Link = am.hub.MakeLink("system", sys.Id)
	} else {
		data.Link = am.hub.MakeLink("system", "example")
	}
	for key, value := range map[string]string{
		"system.id": "example", "system.name": systemName, "system.host": "192.168.1.10",
		"system.port": "45876", "system.status": "up", "system.hostname": systemName,
		"system.os": "Ubuntu 24.04", "system.kernel": "6.8.0", "system.cpu": "Intel Core i5-8250U",
		"system.cores": "4", "system.arch": "amd64", "system.agent_version": beszel.Version,
		"system.uptime": "3d 4h 12m",
	} {
		data.Vars[key] = value
	}
	data.LinkText = "View " + systemName

	switch kind {
	case NotificationKindSystem:
		data.State = "above"
		data.Title = fmt.Sprintf("%s CPU above threshold", systemName)
		data.Message = "CPU averaged 92.50% for the previous 10 minutes."
		for key, value := range map[string]string{
			"metric": "CPU", "descriptor": "CPU", "value": "92.50", "unit": "%", "threshold": "80", "minutes": "10",
		} {
			data.Vars[key] = value
		}
	case NotificationKindContainer:
		data.State = "unhealthy"
		data.Title = fmt.Sprintf("2 unhealthy containers on %s \U0001F534", systemName)
		data.Message = "Unhealthy: web, db"
		for key, value := range map[string]string{"containers": "web, db", "count": "2", "logs": ""} {
			data.Vars[key] = value
		}
	case NotificationKindSmart:
		data.State = "FAILED"
		data.Title = fmt.Sprintf("SMART %s on %s: sda %s", smartStateLabel("FAILED"), systemName, smartStateEmoji("FAILED"))
		data.Message = "Disk sda (Samsung SSD 870 EVO) SMART status changed to FAILED"
		for key, value := range map[string]string{
			"device": "sda", "model": "Samsung SSD 870 EVO", "old_state": "PASSED", "new_state": "FAILED",
		} {
			data.Vars[key] = value
		}
	case NotificationKindSystemd:
		failed := []string{"nginx.service", "cron.service"}
		data.State = "failed"
		data.Title = fmt.Sprintf("Failed services on %s \U0001F534", systemName)
		data.Message = fmt.Sprintf("%s on %s: %s", pluralizeServices(len(failed)), systemName, formatServiceList(failed))
		data.Vars["services"] = strings.Join(failed, ", ")
		data.Vars["count"] = "2"
	case NotificationKindZfs:
		data.State = "DEGRADED"
		data.Title = fmt.Sprintf("Storage pool DEGRADED on %s: tank", systemName)
		data.Message = fmt.Sprintf("Storage pool tank (%s) health changed from ONLINE to DEGRADED", systemName)
		for key, value := range map[string]string{"pool": "tank", "old_health": "ONLINE", "new_health": "DEGRADED"} {
			data.Vars[key] = value
		}
	default: // status
		data.Kind = NotificationKindStatus
		data.State = "down"
		data.Title = fmt.Sprintf("Connection to %s is down \U0001F534", systemName)
		data.Message = strings.TrimSuffix(data.Title, "\U0001F534")
	}
	return data
}
