package notifications

import (
	"encoding/json"
	"net/http"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/apis"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/henrygd/beszel/internal/entities/notification"
)

// RegisterRoutes registers notification API routes
func RegisterRoutes(app core.App, se *core.ServeEvent) {
	api := &NotificationAPI{app: app}

	group := se.Router.Group("/api/beszel/notifications")
	group.Bind(apis.RequireAuth())

	group.GET("/", api.listNotifications)
	group.POST("/", api.createNotification)
	group.GET("/{id}", api.getNotification)
	group.PATCH("/{id}", api.updateNotification)
	group.DELETE("/{id}", api.deleteNotification)
	group.POST("/{id}/test", api.testNotification)
}

// NotificationAPI handles notification API requests
type NotificationAPI struct {
	app core.App
}

// CreateNotificationRequest represents a notification creation request
type CreateNotificationRequest struct {
	Name      string                 `json:"name"`
	Type      string                 `json:"type"`
	Settings  map[string]interface{} `json:"settings"`
	IsDefault bool                   `json:"is_default"`
}

// UpdateNotificationRequest represents a notification update request
type UpdateNotificationRequest struct {
	Name      string                 `json:"name,omitempty"`
	Settings  map[string]interface{} `json:"settings,omitempty"`
	IsDefault *bool                  `json:"is_default,omitempty"`
	Active    *bool                  `json:"active,omitempty"`
}

// NotificationResponse represents a notification response
type NotificationResponse struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Type      string                 `json:"type"`
	IsDefault bool                   `json:"is_default"`
	Active    bool                   `json:"active"`
	Settings  map[string]interface{} `json:"settings"`
	Created   string                 `json:"created"`
	Updated   string                 `json:"updated"`
}

// listNotifications lists all notifications for the authenticated user
func (api *NotificationAPI) listNotifications(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	records, err := api.app.FindAllRecords("notifications",
		dbx.NewExp("user = {:user}", dbx.Params{"user": authRecord.Id}),
	)
	if err != nil {
		return e.InternalServerError("failed to fetch notifications", err)
	}

	notifications := make([]NotificationResponse, 0, len(records))
	for _, record := range records {
		notifications = append(notifications, api.recordToResponse(record))
	}

	return e.JSON(http.StatusOK, notifications)
}

// createNotification creates a new notification
func (api *NotificationAPI) createNotification(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	var req CreateNotificationRequest
	if err := json.NewDecoder(e.Request.Body).Decode(&req); err != nil {
		return e.BadRequestError("invalid request body", err)
	}

	if req.Name == "" || req.Type == "" {
		return e.BadRequestError("name and type are required", nil)
	}

	collection, err := api.app.FindCollectionByNameOrId("notifications")
	if err != nil {
		return e.InternalServerError("failed to find collection", err)
	}

	settingsJSON, _ := json.Marshal(req.Settings)

	record := core.NewRecord(collection)
	record.Set("name", req.Name)
	record.Set("type", req.Type)
	record.Set("settings", string(settingsJSON))
	record.Set("is_default", req.IsDefault)
	record.Set("active", true)
	record.Set("user", authRecord.Id)

	if err := api.app.Save(record); err != nil {
		return e.InternalServerError("failed to create notification", err)
	}

	return e.JSON(http.StatusCreated, api.recordToResponse(record))
}

// getNotification gets a single notification
func (api *NotificationAPI) getNotification(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	id := e.Request.PathValue("id")
	record, err := api.app.FindRecordById("notifications", id)
	if err != nil {
		return e.NotFoundError("notification not found", err)
	}

	if record.GetString("user") != authRecord.Id {
		return e.ForbiddenError("not authorized", nil)
	}

	return e.JSON(http.StatusOK, api.recordToResponse(record))
}

// updateNotification updates a notification
func (api *NotificationAPI) updateNotification(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	id := e.Request.PathValue("id")
	record, err := api.app.FindRecordById("notifications", id)
	if err != nil {
		return e.NotFoundError("notification not found", err)
	}

	if record.GetString("user") != authRecord.Id {
		return e.ForbiddenError("not authorized", nil)
	}

	var req UpdateNotificationRequest
	if err := json.NewDecoder(e.Request.Body).Decode(&req); err != nil {
		return e.BadRequestError("invalid request body", err)
	}

	if req.Name != "" {
		record.Set("name", req.Name)
	}
	if req.Settings != nil {
		settingsJSON, _ := json.Marshal(req.Settings)
		record.Set("settings", string(settingsJSON))
	}
	if req.IsDefault != nil {
		record.Set("is_default", *req.IsDefault)
	}
	if req.Active != nil {
		record.Set("active", *req.Active)
	}

	if err := api.app.Save(record); err != nil {
		return e.InternalServerError("failed to update notification", err)
	}

	return e.JSON(http.StatusOK, api.recordToResponse(record))
}

// deleteNotification deletes a notification
func (api *NotificationAPI) deleteNotification(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	id := e.Request.PathValue("id")
	record, err := api.app.FindRecordById("notifications", id)
	if err != nil {
		return e.NotFoundError("notification not found", err)
	}

	if record.GetString("user") != authRecord.Id {
		return e.ForbiddenError("not authorized", nil)
	}

	if err := api.app.Delete(record); err != nil {
		return e.InternalServerError("failed to delete notification", err)
	}

	return e.NoContent(http.StatusNoContent)
}

// testNotification sends a test notification
func (api *NotificationAPI) testNotification(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	id := e.Request.PathValue("id")
	record, err := api.app.FindRecordById("notifications", id)
	if err != nil {
		return e.NotFoundError("notification not found", err)
	}

	if record.GetString("user") != authRecord.Id {
		return e.ForbiddenError("not authorized", nil)
	}

	notif := &notification.Notification{
		ID:     record.Id,
		Name:   record.GetString("name"),
		Type:   record.GetString("type"),
		Active: record.GetBool("active"),
	}

	if settingsJSON := record.GetString("settings"); settingsJSON != "" {
		var settings map[string]interface{}
		if err := json.Unmarshal([]byte(settingsJSON), &settings); err == nil {
			notif.Settings = settings
		}
	}

	dispatcher := NewDispatcher(api.app)
	msg := &notification.NotificationMessage{
		Title:       "Test Notification",
		Body:        "This is a test notification from Beszel.",
		MonitorName: "Test Monitor",
		Status:      "UP",
		Message:     "Test message",
	}

	provider, err := dispatcher.getProvider(notif)
	if err != nil {
		return e.InternalServerError("failed to create provider", err)
	}

	if err := provider.Send(msg); err != nil {
		return e.InternalServerError("failed to send test notification", err)
	}

	return e.JSON(http.StatusOK, map[string]string{"status": "sent"})
}

// recordToResponse converts a record to a response
func (api *NotificationAPI) recordToResponse(record *core.Record) NotificationResponse {
	var settings map[string]interface{}
	if settingsJSON := record.GetString("settings"); settingsJSON != "" {
		json.Unmarshal([]byte(settingsJSON), &settings)
	}

	return NotificationResponse{
		ID:        record.Id,
		Name:      record.GetString("name"),
		Type:      record.GetString("type"),
		IsDefault: record.GetBool("is_default"),
		Active:    record.GetBool("active"),
		Settings:  settings,
		Created:   record.GetDateTime("created").String(),
		Updated:   record.GetDateTime("updated").String(),
	}
}
