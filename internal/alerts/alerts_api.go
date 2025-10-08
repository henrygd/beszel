package alerts

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// UpsertUserAlerts handles API request to create or update alerts for a user
// across multiple systems (POST /api/beszel/user-alerts)
func UpsertUserAlerts(e *core.RequestEvent) error {
	userID := e.Auth.Id

	reqData := struct {
		Min             uint8    `json:"min"`
		Value           float64  `json:"value"`
		Name            string   `json:"name"`
		Systems         []string `json:"systems"`
		Overwrite       bool     `json:"overwrite"`
		RepeatInterval  *uint16  `json:"repeat_interval"`
		MaxRepeats      *uint16  `json:"max_repeats"`
		Filesystem      string   `json:"filesystem"`
	}{}
	err := e.BindBody(&reqData)
	if err != nil || userID == "" || reqData.Name == "" || len(reqData.Systems) == 0 {
		return e.BadRequestError("Bad data", err)
	}

	alertsCollection, err := e.App.FindCachedCollectionByNameOrId("alerts")
	if err != nil {
		return err
	}

	err = e.App.RunInTransaction(func(txApp core.App) error {
		for _, systemId := range reqData.Systems {
			// find existing matching alert (including filesystem if specified)
			var filter string
			var params dbx.Params
			if reqData.Filesystem != "" {
				filter = "system={:system} && name={:name} && user={:user} && filesystem={:filesystem}"
				params = dbx.Params{"system": systemId, "name": reqData.Name, "user": userID, "filesystem": reqData.Filesystem}
			} else {
				filter = "system={:system} && name={:name} && user={:user} && (filesystem='' || filesystem = null)"
				params = dbx.Params{"system": systemId, "name": reqData.Name, "user": userID}
			}
			alertRecord, err := txApp.FindFirstRecordByFilter(alertsCollection, filter, params)

			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}

			// skip if alert already exists and overwrite is not set
			if !reqData.Overwrite && alertRecord != nil {
				continue
			}

			// create new alert if it doesn't exist
			if alertRecord == nil {
				alertRecord = core.NewRecord(alertsCollection)
				alertRecord.Set("user", userID)
				alertRecord.Set("system", systemId)
				alertRecord.Set("name", reqData.Name)
			}

			alertRecord.Set("value", reqData.Value)
			alertRecord.Set("min", reqData.Min)
			
			// Set repeat fields if provided
			if reqData.RepeatInterval != nil {
				alertRecord.Set("repeat_interval", *reqData.RepeatInterval)
			}
			if reqData.MaxRepeats != nil {
				alertRecord.Set("max_repeats", *reqData.MaxRepeats)
			}
			// Set filesystem field if provided
			if reqData.Filesystem != "" {
				alertRecord.Set("filesystem", reqData.Filesystem)
			}

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

// DeleteUserAlerts handles API request to delete alerts for a user across multiple systems
// (DELETE /api/beszel/user-alerts)
func DeleteUserAlerts(e *core.RequestEvent) error {
	userID := e.Auth.Id

	reqData := struct {
		AlertName  string   `json:"name"`
		Systems    []string `json:"systems"`
		Filesystem string   `json:"filesystem"`
	}{}
	err := e.BindBody(&reqData)
	if err != nil || userID == "" || reqData.AlertName == "" || len(reqData.Systems) == 0 {
		return e.BadRequestError("Bad data", err)
	}

	var numDeleted uint16

	err = e.App.RunInTransaction(func(txApp core.App) error {
		for _, systemId := range reqData.Systems {
			// Find existing alert to delete (including filesystem if specified)
			var filter string
			var params dbx.Params
			if reqData.Filesystem != "" {
				filter = "system={:system} && name={:name} && user={:user} && filesystem={:filesystem}"
				params = dbx.Params{"system": systemId, "name": reqData.AlertName, "user": userID, "filesystem": reqData.Filesystem}
			} else {
				filter = "system={:system} && name={:name} && user={:user} && (filesystem='' || filesystem = null)"
				params = dbx.Params{"system": systemId, "name": reqData.AlertName, "user": userID}
			}
			alertRecord, err := txApp.FindFirstRecordByFilter("alerts", filter, params)

			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					// alert doesn't exist, continue to next system
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
