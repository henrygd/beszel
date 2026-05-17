package alerts

import (
	"database/sql"
	"errors"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"

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

// SendTestNotification handles API request to send a test notification to a specified Shoutrrr URL
func (am *AlertManager) SendTestNotification(e *core.RequestEvent) error {
	var data struct {
		URL string `json:"url"`
	}
	err := e.BindBody(&data)
	if err != nil || data.URL == "" {
		return e.BadRequestError("URL is required", err)
	}
	// Only allow admins to send test notifications to internal URLs
	if !e.Auth.IsSuperuser() && e.Auth.GetString("role") != "admin" {
		internalURL, err := isInternalURL(data.URL)
		if err != nil {
			return e.BadRequestError(err.Error(), nil)
		}
		if internalURL {
			return e.ForbiddenError("Only admins can send to internal destinations", nil)
		}
	}
	err = am.SendShoutrrrAlert(data.URL, "Test Alert", "This is a notification from Beszel.", am.hub.Settings().Meta.AppURL, "View Beszel")
	if err != nil {
		return e.JSON(200, map[string]string{"err": err.Error()})
	}
	return e.JSON(200, map[string]bool{"err": false})
}

// isInternalURL checks if the given shoutrrr URL points to an internal destination (localhost or private IP)
func isInternalURL(rawURL string) (bool, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false, err
	}

	host := parsedURL.Hostname()
	if host == "" {
		return false, nil
	}

	if strings.EqualFold(host, "localhost") {
		return true, nil
	}

	if ip := net.ParseIP(host); ip != nil {
		return isInternalIP(ip), nil
	}

	// Some Shoutrrr URLs use the host position for service identifiers rather than a
	// network hostname (for example, discord://token@webhookid). Restrict DNS lookups
	// to names that look like actual hostnames so valid service URLs keep working.
	if !strings.Contains(host, ".") {
		return false, nil
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return false, nil
	}

	if slices.ContainsFunc(ips, isInternalIP) {
		return true, nil
	}

	return false, nil
}

func isInternalIP(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified()
}
