package alerts

import (
	"database/sql"
	"errors"
	"net/http"
	"slices"

	"github.com/henrygd/beszel/internal/hub/utils"
	"github.com/nicholas-fedor/shoutrrr"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// UpsertAlerts handles API request to create or update alerts across multiple systems.
// Only admins and superusers can manage alerts (POST /api/beszel/alerts).
func UpsertAlerts(e *core.RequestEvent) error {
	if !e.Auth.IsSuperuser() && e.Auth.GetString("role") != "admin" {
		return e.ForbiddenError("Only admins can manage alerts.", nil)
	}

	reqData := struct {
		Min       uint8    `json:"min"`
		Value     float64  `json:"value"`
		Name      string   `json:"name"`
		Systems   []string `json:"systems"`
		Overwrite bool     `json:"overwrite"`
	}{}
	err := e.BindBody(&reqData)
	if err != nil || reqData.Name == "" || len(reqData.Systems) == 0 {
		return e.BadRequestError("Bad data", err)
	}

	if reqData.Name == alertNameNetworkMonitorLoss {
		if reqData.Value < 0 || reqData.Value >= 100 {
			return e.BadRequestError("Monitor loss threshold must be at least 0 and below 100", nil)
		}
		reqData.Min = 0
	}

	alertsCollection, err := e.App.FindCachedCollectionByNameOrId("alerts")
	if err != nil {
		return err
	}

	err = e.App.RunInTransaction(func(txApp core.App) error {
		for _, systemId := range reqData.Systems {
			if !canManageSystemAlerts(txApp, e.Auth, systemId) {
				continue
			}
			alertRecord, err := txApp.FindFirstRecordByFilter(alertsCollection,
				"system={:system} && name={:name}",
				dbx.Params{"system": systemId, "name": reqData.Name})

			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}

			if !reqData.Overwrite && alertRecord != nil {
				continue
			}

			if alertRecord == nil {
				alertRecord = core.NewRecord(alertsCollection)
				alertRecord.Set("system", systemId)
				alertRecord.Set("name", reqData.Name)
			}

			alertRecord.Set("value", reqData.Value)
			alertRecord.Set("min", reqData.Min)

			if err := txApp.SaveNoValidate(alertRecord); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	return e.JSON(http.StatusOK, map[string]any{"success": true})
}

// DeleteAlerts handles API request to delete alerts across multiple systems.
// Only admins and superusers can manage alerts (DELETE /api/beszel/alerts).
func DeleteAlerts(e *core.RequestEvent) error {
	if !e.Auth.IsSuperuser() && e.Auth.GetString("role") != "admin" {
		return e.ForbiddenError("Only admins can manage alerts.", nil)
	}

	reqData := struct {
		AlertName string   `json:"name"`
		Systems   []string `json:"systems"`
	}{}
	err := e.BindBody(&reqData)
	if err != nil || reqData.AlertName == "" || len(reqData.Systems) == 0 {
		return e.BadRequestError("Bad data", err)
	}

	var numDeleted uint16

	err = e.App.RunInTransaction(func(txApp core.App) error {
		for _, systemId := range reqData.Systems {
			if !canManageSystemAlerts(txApp, e.Auth, systemId) {
				continue
			}
			alertRecord, err := txApp.FindFirstRecordByFilter("alerts",
				"system={:system} && name={:name}",
				dbx.Params{"system": systemId, "name": reqData.AlertName})

			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					continue
				}
				return err
			}

			if err := txApp.Delete(alertRecord); err != nil {
				return err
			}
			numDeleted++
		}
		return nil
	})

	if err != nil {
		return err
	}

	return e.JSON(http.StatusOK, map[string]any{"success": true, "count": numDeleted})
}

// canManageSystemAlerts reports whether auth may change alerts on a system.
// Superusers manage all systems; admins only systems they can access.
func canManageSystemAlerts(app core.App, auth *core.Record, systemID string) bool {
	return auth.IsSuperuser() || userHasSystem(app, auth.Id, systemID)
}

func userHasSystem(app core.App, userID, systemID string) bool {
	system, err := app.FindRecordById("systems", systemID)
	if err != nil {
		return false
	}
	shareAll, _ := utils.GetEnv("SHARE_ALL_SYSTEMS")
	return shareAll == "true" || slices.Contains(system.GetStringSlice("users"), userID)
}

// SendTestNotification handles API request to send a test notification to a specified Shoutrrr URL
func (am *AlertManager) SendTestNotification(e *core.RequestEvent) error {
	var data struct {
		URL string `json:"url"`
	}
	err := e.BindBody(&data)
	if err != nil || data.URL == "" {
		return e.BadRequestError("URL is required", err)
	}
	send := shoutrrr.Send
	if !e.Auth.IsSuperuser() && e.Auth.GetString("role") != "admin" {
		send = sendPublicNotification
	}
	err = am.sendShoutrrrAlert(data.URL, "Test Alert", "This is a notification from Beszel.", am.hub.Settings().Meta.AppURL, "View Beszel", send)
	if errors.Is(err, errInternalDestination) || errors.Is(err, errUnrestrictedService) {
		return e.ForbiddenError(err.Error(), nil)
	}
	if err != nil {
		return e.JSON(200, map[string]string{"err": err.Error()})
	}
	return e.JSON(200, map[string]bool{"err": false})
}
