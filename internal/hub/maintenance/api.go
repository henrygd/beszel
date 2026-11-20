package maintenance

import (
	"encoding/json"
	"net/http"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/apis"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
)

// APIHandler handles maintenance window API requests
type APIHandler struct {
	app core.App
}

// NewAPIHandler creates a new maintenance API handler
func NewAPIHandler(app core.App) *APIHandler {
	return &APIHandler{app: app}
}

// RegisterRoutes registers maintenance API routes
func (h *APIHandler) RegisterRoutes(se *core.ServeEvent) {
	api := se.Router.Group("/api/beszel/maintenance")
	api.Bind(apis.RequireAuth())

	api.GET("/", h.listMaintenanceWindows)
	api.POST("/", h.createMaintenanceWindow)
	api.GET("/{id}", h.getMaintenanceWindow)
	api.PATCH("/{id}", h.updateMaintenanceWindow)
	api.DELETE("/{id}", h.deleteMaintenanceWindow)
	api.POST("/{id}/cancel", h.cancelMaintenanceWindow)
}

// MaintenanceWindowRequest for create/update
type MaintenanceWindowRequest struct {
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	MonitorID         string    `json:"monitor_id"`
	DomainID          string    `json:"domain_id"`
	StartTime         time.Time `json:"start_time"`
	EndTime           time.Time `json:"end_time"`
	Recurring         bool      `json:"recurring"`
	RecurrencePattern string    `json:"recurrence_pattern"`
	SuppressAlerts    bool      `json:"suppress_alerts"`
}

// listMaintenanceWindows lists all maintenance windows for the authenticated user
func (h *APIHandler) listMaintenanceWindows(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	records, err := h.app.FindAllRecords("maintenance_windows",
		dbx.NewExp("user = {:user}", dbx.Params{"user": authRecord.Id}),
	)
	if err != nil {
		return e.InternalServerError("failed to fetch maintenance windows", err)
	}

	windows := make([]map[string]interface{}, 0, len(records))
	for _, record := range records {
		windows = append(windows, h.recordToResponse(record))
	}

	return e.JSON(http.StatusOK, windows)
}

// createMaintenanceWindow creates a new maintenance window
func (h *APIHandler) createMaintenanceWindow(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	var req MaintenanceWindowRequest
	if err := json.NewDecoder(e.Request.Body).Decode(&req); err != nil {
		return e.BadRequestError("invalid request body", err)
	}

	if req.Name == "" || req.StartTime.IsZero() || req.EndTime.IsZero() {
		return e.BadRequestError("name, start_time, and end_time are required", nil)
	}

	// Verify monitor/domain belongs to user if specified
	if req.MonitorID != "" {
		monitor, err := h.app.FindRecordById("monitors", req.MonitorID)
		if err != nil || monitor.GetString("user") != authRecord.Id {
			return e.BadRequestError("invalid monitor_id", nil)
		}
	}
	if req.DomainID != "" {
		domain, err := h.app.FindRecordById("domains", req.DomainID)
		if err != nil || domain.GetString("user") != authRecord.Id {
			return e.BadRequestError("invalid domain_id", nil)
		}
	}

	collection, err := h.app.FindCollectionByNameOrId("maintenance_windows")
	if err != nil {
		return e.InternalServerError("failed to find collection", err)
	}

	record := core.NewRecord(collection)
	record.Set("name", req.Name)
	record.Set("description", req.Description)
	record.Set("monitor", req.MonitorID)
	record.Set("domain", req.DomainID)
	record.Set("start_time", req.StartTime)
	record.Set("end_time", req.EndTime)
	record.Set("recurring", req.Recurring)
	record.Set("recurrence_pattern", req.RecurrencePattern)
	record.Set("suppress_alerts", req.SuppressAlerts)
	record.Set("status", "scheduled")
	record.Set("user", authRecord.Id)

	if err := h.app.Save(record); err != nil {
		return e.InternalServerError("failed to create maintenance window", err)
	}

	return e.JSON(http.StatusCreated, h.recordToResponse(record))
}

// getMaintenanceWindow gets a single maintenance window
func (h *APIHandler) getMaintenanceWindow(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("maintenance_windows", id)
	if err != nil {
		return e.NotFoundError("maintenance window not found", err)
	}

	if record.GetString("user") != authRecord.Id {
		return e.ForbiddenError("not authorized", nil)
	}

	return e.JSON(http.StatusOK, h.recordToResponse(record))
}

// updateMaintenanceWindow updates a maintenance window
func (h *APIHandler) updateMaintenanceWindow(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("maintenance_windows", id)
	if err != nil {
		return e.NotFoundError("maintenance window not found", err)
	}

	if record.GetString("user") != authRecord.Id {
		return e.ForbiddenError("not authorized", nil)
	}

	// Only allow updates if not already completed or cancelled
	status := record.GetString("status")
	if status == "completed" || status == "cancelled" {
		return e.BadRequestError("cannot update completed or cancelled maintenance window", nil)
	}

	var req MaintenanceWindowRequest
	if err := json.NewDecoder(e.Request.Body).Decode(&req); err != nil {
		return e.BadRequestError("invalid request body", err)
	}

	if req.Name != "" {
		record.Set("name", req.Name)
	}
	if req.Description != "" {
		record.Set("description", req.Description)
	}
	if !req.StartTime.IsZero() {
		record.Set("start_time", req.StartTime)
	}
	if !req.EndTime.IsZero() {
		record.Set("end_time", req.EndTime)
	}

	if err := h.app.Save(record); err != nil {
		return e.InternalServerError("failed to update maintenance window", err)
	}

	return e.JSON(http.StatusOK, h.recordToResponse(record))
}

// deleteMaintenanceWindow deletes a maintenance window
func (h *APIHandler) deleteMaintenanceWindow(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("maintenance_windows", id)
	if err != nil {
		return e.NotFoundError("maintenance window not found", err)
	}

	if record.GetString("user") != authRecord.Id {
		return e.ForbiddenError("not authorized", nil)
	}

	if err := h.app.Delete(record); err != nil {
		return e.InternalServerError("failed to delete maintenance window", err)
	}

	return e.NoContent(http.StatusNoContent)
}

// cancelMaintenanceWindow cancels a maintenance window
func (h *APIHandler) cancelMaintenanceWindow(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("maintenance_windows", id)
	if err != nil {
		return e.NotFoundError("maintenance window not found", err)
	}

	if record.GetString("user") != authRecord.Id {
		return e.ForbiddenError("not authorized", nil)
	}

	status := record.GetString("status")
	if status == "completed" || status == "cancelled" {
		return e.BadRequestError("cannot cancel completed or already cancelled maintenance window", nil)
	}

	record.Set("status", "cancelled")
	if err := h.app.Save(record); err != nil {
		return e.InternalServerError("failed to cancel maintenance window", err)
	}

	return e.JSON(http.StatusOK, h.recordToResponse(record))
}

// recordToResponse converts a record to a response map
func (h *APIHandler) recordToResponse(record *core.Record) map[string]interface{} {
	return map[string]interface{}{
		"id":                 record.Id,
		"name":               record.GetString("name"),
		"description":        record.GetString("description"),
		"monitor_id":         record.GetString("monitor"),
		"domain_id":          record.GetString("domain"),
		"start_time":         record.GetDateTime("start_time").Time(),
		"end_time":           record.GetDateTime("end_time").Time(),
		"status":             record.GetString("status"),
		"recurring":          record.GetBool("recurring"),
		"recurrence_pattern": record.GetString("recurrence_pattern"),
		"suppress_alerts":    record.GetBool("suppress_alerts"),
		"created":            record.GetDateTime("created").Time(),
		"updated":            record.GetDateTime("updated").Time(),
	}
}

// IsInMaintenanceWindow checks if a monitor or domain is currently in a maintenance window
func (h *APIHandler) IsInMaintenanceWindow(monitorID, domainID string) bool {
	now := time.Now()

	exp := dbx.NewExp("status = {:status} AND start_time <= {:now} AND end_time >= {:now}",
		dbx.Params{"status": "scheduled", "now": now})

	if monitorID != "" {
		exp = dbx.NewExp("status = {:status} AND start_time <= {:now} AND end_time >= {:now} AND (monitor = {:monitor} OR monitor = '')",
			dbx.Params{"status": "scheduled", "now": now, "monitor": monitorID})
	}
	if domainID != "" {
		exp = dbx.NewExp("status = {:status} AND start_time <= {:now} AND end_time >= {:now} AND (domain = {:domain} OR domain = '')",
			dbx.Params{"status": "scheduled", "now": now, "domain": domainID})
	}

	records, err := h.app.FindAllRecords("maintenance_windows", exp)
	if err != nil {
		return false
	}

	for _, record := range records {
		if record.GetBool("suppress_alerts") {
			return true
		}
	}

	return false
}
