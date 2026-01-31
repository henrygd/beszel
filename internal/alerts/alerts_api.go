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
		Min       uint8    `json:"min"`
		Value     float64  `json:"value"`
		Name      string   `json:"name"`
		Systems   []string `json:"systems"`
		Overwrite bool     `json:"overwrite"`
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
			// find existing matching alert
			alertRecord, err := txApp.FindFirstRecordByFilter(alertsCollection,
				"system={:system} && name={:name} && user={:user}",
				dbx.Params{"system": systemId, "name": reqData.Name, "user": userID})

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
		AlertName string   `json:"name"`
		Systems   []string `json:"systems"`
	}{}
	err := e.BindBody(&reqData)
	if err != nil || userID == "" || reqData.AlertName == "" || len(reqData.Systems) == 0 {
		return e.BadRequestError("Bad data", err)
	}

	var numDeleted uint16

	err = e.App.RunInTransaction(func(txApp core.App) error {
		for _, systemId := range reqData.Systems {
			// Find existing alert to delete
			alertRecord, err := txApp.FindFirstRecordByFilter("alerts",
				"system={:system} && name={:name} && user={:user}",
				dbx.Params{"system": systemId, "name": reqData.AlertName, "user": userID})

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

// UpsertUserContainerAlerts handles API request to create or update container alerts for a user
// across multiple containers (POST /api/beszel/user-container-alerts)
func UpsertUserContainerAlerts(e *core.RequestEvent) error {
	userID := e.Auth.Id

	reqData := struct {
		Min        uint8    `json:"min"`
		Value      float64  `json:"value"`
		Name       string   `json:"name"`
		Systems    []string `json:"systems"`
		Containers []string `json:"containers"`
		Overwrite  bool     `json:"overwrite"`
	}{}
	err := e.BindBody(&reqData)
	if err != nil || userID == "" || reqData.Name == "" || len(reqData.Systems) == 0 || len(reqData.Containers) == 0 {
		return e.BadRequestError("Bad data", err)
	}

	containerAlertsCollection, err := e.App.FindCachedCollectionByNameOrId("container_alerts")
	if err != nil {
		return err
	}

	err = e.App.RunInTransaction(func(txApp core.App) error {
		for _, systemId := range reqData.Systems {
			for _, containerId := range reqData.Containers {
				// find existing matching alert
				alertRecord, err := txApp.FindFirstRecordByFilter(containerAlertsCollection,
					"system={:system} && container={:container} && name={:name} && user={:user}",
					dbx.Params{"system": systemId, "container": containerId, "name": reqData.Name, "user": userID})

				if err != nil && !errors.Is(err, sql.ErrNoRows) {
					return err
				}

				// skip if alert already exists and overwrite is not set
				if !reqData.Overwrite && alertRecord != nil {
					continue
				}

				// create new alert if it doesn't exist
				if alertRecord == nil {
					alertRecord = core.NewRecord(containerAlertsCollection)
					alertRecord.Set("user", userID)
					alertRecord.Set("system", systemId)
					alertRecord.Set("container", containerId)
					alertRecord.Set("name", reqData.Name)
				}

				alertRecord.Set("value", reqData.Value)
				alertRecord.Set("min", reqData.Min)

				if err := txApp.SaveNoValidate(alertRecord); err != nil {
					return err
				}
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	return e.JSON(http.StatusOK, map[string]any{"success": true})
}

// DeleteUserContainerAlerts handles API request to delete container alerts for a user
// across multiple containers (DELETE /api/beszel/user-container-alerts)
func DeleteUserContainerAlerts(e *core.RequestEvent) error {
	userID := e.Auth.Id

	reqData := struct {
		AlertName  string   `json:"name"`
		Systems    []string `json:"systems"`
		Containers []string `json:"containers"`
	}{}
	err := e.BindBody(&reqData)
	if err != nil || userID == "" || reqData.AlertName == "" || len(reqData.Systems) == 0 || len(reqData.Containers) == 0 {
		return e.BadRequestError("Bad data", err)
	}

	var numDeleted uint16

	err = e.App.RunInTransaction(func(txApp core.App) error {
		for _, systemId := range reqData.Systems {
			for _, containerId := range reqData.Containers {
				// Find existing alert to delete
				alertRecord, err := txApp.FindFirstRecordByFilter("container_alerts",
					"system={:system} && container={:container} && name={:name} && user={:user}",
					dbx.Params{"system": systemId, "container": containerId, "name": reqData.AlertName, "user": userID})

				if err != nil {
					if errors.Is(err, sql.ErrNoRows) {
						// alert doesn't exist, continue to next container
						continue
					}
					return err
				}

				if err := txApp.Delete(alertRecord); err != nil {
					return err
				}
				numDeleted++
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	return e.JSON(http.StatusOK, map[string]any{"success": true, "count": numDeleted})
}
