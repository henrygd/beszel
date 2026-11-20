package incidents

import (
	"encoding/json"
	"net/http"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/apis"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/henrygd/beszel/internal/entities/incident"
)

// APIHandler handles incident API requests
type APIHandler struct {
	app core.App
}

// NewAPIHandler creates a new incidents API handler
func NewAPIHandler(app core.App) *APIHandler {
	return &APIHandler{app: app}
}

// RegisterRoutes registers incident API routes
func (h *APIHandler) RegisterRoutes(se *core.ServeEvent) {
	api := se.Router.Group("/api/beszel/incidents")
	api.Bind(apis.RequireAuth())

	api.GET("/", h.listIncidents)
	api.POST("/", h.createIncident)
	api.GET("/stats", h.getIncidentStats)
	api.GET("/calendar", h.getCalendarEvents)
	api.GET("/{id}", h.getIncident)
	api.PATCH("/{id}", h.updateIncident)
	api.POST("/{id}/acknowledge", h.acknowledgeIncident)
	api.POST("/{id}/resolve", h.resolveIncident)
	api.POST("/{id}/close", h.closeIncident)
	api.POST("/{id}/updates", h.addIncidentUpdate)
	api.GET("/{id}/updates", h.getIncidentUpdates)
}

// listIncidents lists all incidents for the authenticated user
func (h *APIHandler) listIncidents(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	// Get query params for filtering
	status := e.Request.URL.Query().Get("status")
	severity := e.Request.URL.Query().Get("severity")

	query := dbx.NewExp("user = {:user}", dbx.Params{"user": authRecord.Id})

	if status != "" {
		query = dbx.And(query, dbx.NewExp("status = {:status}", dbx.Params{"status": status}))
	}
	if severity != "" {
		query = dbx.And(query, dbx.NewExp("severity = {:severity}", dbx.Params{"severity": severity}))
	}

	records, err := h.app.FindAllRecords("incidents", query)
	if err != nil {
		return e.InternalServerError("failed to fetch incidents", err)
	}

	incidents := make([]map[string]interface{}, 0, len(records))
	for _, record := range records {
		incidents = append(incidents, h.recordToResponse(record))
	}

	return e.JSON(http.StatusOK, incidents)
}

// createIncident creates a new incident
func (h *APIHandler) createIncident(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	var req struct {
		Title       string  `json:"title"`
		Description string  `json:"description"`
		Type        string  `json:"type"`
		Severity    string  `json:"severity"`
		MonitorID   *string `json:"monitor,omitempty"`
		DomainID    *string `json:"domain,omitempty"`
		SystemID    *string `json:"system,omitempty"`
	}
	if err := json.NewDecoder(e.Request.Body).Decode(&req); err != nil {
		return e.BadRequestError("invalid request body", err)
	}

	if req.Title == "" || req.Type == "" {
		return e.BadRequestError("title and type are required", nil)
	}

	collection, err := h.app.FindCollectionByNameOrId("incidents")
	if err != nil {
		return e.InternalServerError("failed to find collection", err)
	}

	record := core.NewRecord(collection)
	record.Set("title", req.Title)
	record.Set("description", req.Description)
	record.Set("type", req.Type)
	record.Set("severity", req.Severity)
	record.Set("status", incident.StatusOpen)
	record.Set("started_at", time.Now())
	if req.MonitorID != nil {
		record.Set("monitor", *req.MonitorID)
	}
	if req.DomainID != nil {
		record.Set("domain", *req.DomainID)
	}
	if req.SystemID != nil {
		record.Set("system", *req.SystemID)
	}
	record.Set("user", authRecord.Id)

	if err := h.app.Save(record); err != nil {
		return e.InternalServerError("failed to create incident", err)
	}

	return e.JSON(http.StatusCreated, h.recordToResponse(record))
}

// getIncident gets a single incident
func (h *APIHandler) getIncident(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("incidents", id)
	if err != nil {
		return e.NotFoundError("incident not found", err)
	}

	if record.GetString("user") != authRecord.Id {
		return e.ForbiddenError("not authorized", nil)
	}

	return e.JSON(http.StatusOK, h.recordToResponse(record))
}

// updateIncident updates an incident
func (h *APIHandler) updateIncident(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("incidents", id)
	if err != nil {
		return e.NotFoundError("incident not found", err)
	}

	if record.GetString("user") != authRecord.Id {
		return e.ForbiddenError("not authorized", nil)
	}

	var req struct {
		Title       *string `json:"title,omitempty"`
		Description *string `json:"description,omitempty"`
		AssignedTo  *string `json:"assigned_to,omitempty"`
	}
	if err := json.NewDecoder(e.Request.Body).Decode(&req); err != nil {
		return e.BadRequestError("invalid request body", err)
	}

	if req.Title != nil {
		record.Set("title", *req.Title)
	}
	if req.Description != nil {
		record.Set("description", *req.Description)
	}
	if req.AssignedTo != nil {
		record.Set("assigned_to", *req.AssignedTo)
	}

	if err := h.app.Save(record); err != nil {
		return e.InternalServerError("failed to update incident", err)
	}

	return e.JSON(http.StatusOK, h.recordToResponse(record))
}

// acknowledgeIncident acknowledges an incident
func (h *APIHandler) acknowledgeIncident(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("incidents", id)
	if err != nil {
		return e.NotFoundError("incident not found", err)
	}

	if record.GetString("user") != authRecord.Id {
		return e.ForbiddenError("not authorized", nil)
	}

	oldStatus := record.GetString("status")
	now := time.Now()
	record.Set("status", incident.StatusAcknowledged)
	record.Set("acknowledged_at", now)

	if err := h.app.Save(record); err != nil {
		return e.InternalServerError("failed to acknowledge incident", err)
	}

	// Add update record
	h.addUpdate(id, "Incident acknowledged", "status_change", &oldStatus, strPtr(incident.StatusAcknowledged), authRecord.Id)

	return e.JSON(http.StatusOK, h.recordToResponse(record))
}

// resolveIncident resolves an incident
func (h *APIHandler) resolveIncident(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("incidents", id)
	if err != nil {
		return e.NotFoundError("incident not found", err)
	}

	if record.GetString("user") != authRecord.Id {
		return e.ForbiddenError("not authorized", nil)
	}

	var req struct {
		Resolution string `json:"resolution,omitempty"`
		RootCause  string `json:"root_cause,omitempty"`
	}
	if err := json.NewDecoder(e.Request.Body).Decode(&req); err != nil {
		return e.BadRequestError("invalid request body", err)
	}

	oldStatus := record.GetString("status")
	now := time.Now()
	record.Set("status", incident.StatusResolved)
	record.Set("resolved_at", now)
	if req.Resolution != "" {
		record.Set("resolution", req.Resolution)
	}
	if req.RootCause != "" {
		record.Set("root_cause", req.RootCause)
	}

	if err := h.app.Save(record); err != nil {
		return e.InternalServerError("failed to resolve incident", err)
	}

	// Add update record
	h.addUpdate(id, "Incident resolved: "+req.Resolution, "status_change", &oldStatus, strPtr(incident.StatusResolved), authRecord.Id)

	return e.JSON(http.StatusOK, h.recordToResponse(record))
}

// closeIncident closes an incident
func (h *APIHandler) closeIncident(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("incidents", id)
	if err != nil {
		return e.NotFoundError("incident not found", err)
	}

	if record.GetString("user") != authRecord.Id {
		return e.ForbiddenError("not authorized", nil)
	}

	oldStatus := record.GetString("status")
	now := time.Now()
	record.Set("status", incident.StatusClosed)
	record.Set("closed_at", now)

	if err := h.app.Save(record); err != nil {
		return e.InternalServerError("failed to close incident", err)
	}

	// Add update record
	h.addUpdate(id, "Incident closed", "status_change", &oldStatus, strPtr(incident.StatusClosed), authRecord.Id)

	return e.JSON(http.StatusOK, h.recordToResponse(record))
}

// addIncidentUpdate adds an update to an incident
func (h *APIHandler) addIncidentUpdate(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	id := e.Request.PathValue("id")

	// Verify incident exists and belongs to user
	incident, err := h.app.FindRecordById("incidents", id)
	if err != nil {
		return e.NotFoundError("incident not found", err)
	}
	if incident.GetString("user") != authRecord.Id {
		return e.ForbiddenError("not authorized", nil)
	}

	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(e.Request.Body).Decode(&req); err != nil {
		return e.BadRequestError("invalid request body", err)
	}

	if req.Message == "" {
		return e.BadRequestError("message is required", nil)
	}

	h.addUpdate(id, req.Message, "note", nil, nil, authRecord.Id)

	return e.JSON(http.StatusCreated, map[string]string{"status": "added"})
}

// getIncidentUpdates gets all updates for an incident
func (h *APIHandler) getIncidentUpdates(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	id := e.Request.PathValue("id")

	// Verify incident exists and belongs to user
	incident, err := h.app.FindRecordById("incidents", id)
	if err != nil {
		return e.NotFoundError("incident not found", err)
	}
	if incident.GetString("user") != authRecord.Id {
		return e.ForbiddenError("not authorized", nil)
	}

	records, err := h.app.FindAllRecords("incident_updates",
		dbx.NewExp("incident = {:incident}", dbx.Params{"incident": id}),
	)
	if err != nil {
		return e.InternalServerError("failed to fetch updates", err)
	}

	updates := make([]map[string]interface{}, 0, len(records))
	for _, record := range records {
		updates = append(updates, map[string]interface{}{
			"id":          record.Id,
			"message":     record.GetString("message"),
			"update_type": record.GetString("update_type"),
			"old_status":  record.GetString("old_status"),
			"new_status":  record.GetString("new_status"),
			"created_by":  record.GetString("created_by"),
			"created_at":  record.GetDateTime("created_at").String(),
		})
	}

	return e.JSON(http.StatusOK, updates)
}

// getIncidentStats returns incident statistics
func (h *APIHandler) getIncidentStats(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	// Count by status
	total, _ := h.app.CountRecords("incidents", dbx.NewExp("user = {:user}", dbx.Params{"user": authRecord.Id}))
	open, _ := h.app.CountRecords("incidents", dbx.HashExp{"user": authRecord.Id, "status": incident.StatusOpen})
	acknowledged, _ := h.app.CountRecords("incidents", dbx.HashExp{"user": authRecord.Id, "status": incident.StatusAcknowledged})
	resolved, _ := h.app.CountRecords("incidents", dbx.HashExp{"user": authRecord.Id, "status": incident.StatusResolved})

	// Calculate MTTR
	resolvedRecords, _ := h.app.FindAllRecords("incidents",
		dbx.And(
			dbx.NewExp("user = {:user}", dbx.Params{"user": authRecord.Id}),
			dbx.NewExp("status = {:status}", dbx.Params{"status": incident.StatusResolved}),
		),
	)

	var totalResolutionTime time.Duration
	for _, r := range resolvedRecords {
		started := r.GetDateTime("started_at").Time()
		resolved := r.GetDateTime("resolved_at").Time()
		if !started.IsZero() && !resolved.IsZero() {
			totalResolutionTime += resolved.Sub(started)
		}
	}

	mttr := 0.0
	if len(resolvedRecords) > 0 {
		mttr = totalResolutionTime.Hours() / float64(len(resolvedRecords))
	}

	return e.JSON(http.StatusOK, map[string]interface{}{
		"total_incidents":        total,
		"open_incidents":         open,
		"acknowledged_incidents": acknowledged,
		"resolved_incidents":     resolved,
		"mttr_hours":             mttr,
	})
}

// getCalendarEvents returns events for calendar view
func (h *APIHandler) getCalendarEvents(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	events := []map[string]interface{}{}
	from, to := calendarRange(e)

	// Domain expirations
	domains, _ := h.app.FindAllRecords("domains",
		dbx.NewExp("user = {:user}", dbx.Params{"user": authRecord.Id}),
	)
	for _, d := range domains {
		expiryDate := d.GetDateTime("expiry_date").Time()
		if expiryDate.IsZero() || !dateInRange(expiryDate, from, to) {
			continue
		}
		domainName := d.GetString("domain_name")
		daysUntil := int(expiryDate.Sub(time.Now()).Hours() / 24)

		var color string
		if daysUntil <= 7 {
			color = "#ef4444" // red
		} else if daysUntil <= 30 {
			color = "#f59e0b" // orange
		} else {
			color = "#3b82f6" // blue
		}

		events = append(events, map[string]interface{}{
			"id":          "domain-" + d.Id,
			"title":       domainName + " expires",
			"date":        expiryDate.Format("2006-01-02"),
			"type":        "domain_expiry",
			"color":       color,
			"domain_id":   d.Id,
			"entity_id":   d.Id,
			"entity_name": domainName,
			"link":        "/domain/" + d.Id,
			"days_until":  daysUntil,
		})
	}

	// SSL expirations
	for _, d := range domains {
		sslExpiry := d.GetDateTime("ssl_valid_to").Time()
		if sslExpiry.IsZero() || !dateInRange(sslExpiry, from, to) {
			continue
		}
		domainName := d.GetString("domain_name")
		daysUntil := int(sslExpiry.Sub(time.Now()).Hours() / 24)

		var color string
		if daysUntil <= 7 {
			color = "#ef4444"
		} else if daysUntil <= 14 {
			color = "#f59e0b"
		} else {
			color = "#8b5cf6"
		}

		events = append(events, map[string]interface{}{
			"id":          "ssl-" + d.Id,
			"title":       domainName + " SSL expires",
			"date":        sslExpiry.Format("2006-01-02"),
			"type":        "ssl_expiry",
			"color":       color,
			"domain_id":   d.Id,
			"entity_id":   d.Id,
			"entity_name": domainName,
			"link":        "/domain/" + d.Id,
			"days_until":  daysUntil,
		})
	}

	// Incidents
	incidents, _ := h.app.FindAllRecords("incidents",
		dbx.NewExp("user = {:user}", dbx.Params{"user": authRecord.Id}),
	)
	for _, i := range incidents {
		startedAt := i.GetDateTime("started_at").Time()
		if startedAt.IsZero() || !dateInRange(startedAt, from, to) {
			continue
		}
		title := i.GetString("title")
		severity := i.GetString("severity")

		var color string
		switch severity {
		case incident.SeverityCritical:
			color = "#dc2626"
		case incident.SeverityHigh:
			color = "#ea580c"
		default:
			color = "#6b7280"
		}

		events = append(events, map[string]interface{}{
			"id":          "incident-" + i.Id,
			"title":       title,
			"date":        startedAt.Format("2006-01-02"),
			"type":        "incident",
			"color":       color,
			"incident_id": i.Id,
			"entity_id":   i.Id,
			"entity_name": title,
			"link":        "/incidents",
		})
	}

	return e.JSON(http.StatusOK, events)
}

func calendarRange(e *core.RequestEvent) (*time.Time, *time.Time) {
	var from, to *time.Time
	if value := e.Request.URL.Query().Get("from"); value != "" {
		if parsed, err := time.Parse("2006-01-02", value); err == nil {
			from = &parsed
		}
	}
	if value := e.Request.URL.Query().Get("to"); value != "" {
		if parsed, err := time.Parse("2006-01-02", value); err == nil {
			end := parsed.Add(24*time.Hour - time.Nanosecond)
			to = &end
		}
	}
	return from, to
}

func dateInRange(value time.Time, from, to *time.Time) bool {
	if from != nil && value.Before(*from) {
		return false
	}
	if to != nil && value.After(*to) {
		return false
	}
	return true
}

// addUpdate adds an update record
func (h *APIHandler) addUpdate(incidentID, message, updateType string, oldStatus, newStatus *string, createdBy string) {
	collection, err := h.app.FindCollectionByNameOrId("incident_updates")
	if err != nil {
		return
	}

	record := core.NewRecord(collection)
	record.Set("incident", incidentID)
	record.Set("message", message)
	record.Set("update_type", updateType)
	if oldStatus != nil {
		record.Set("old_status", *oldStatus)
	}
	if newStatus != nil {
		record.Set("new_status", *newStatus)
	}
	record.Set("created_by", createdBy)
	record.Set("created_at", time.Now())

	h.app.Save(record)
}

// recordToResponse converts a record to API response
func (h *APIHandler) recordToResponse(record *core.Record) map[string]interface{} {
	return map[string]interface{}{
		"id":              record.Id,
		"title":           record.GetString("title"),
		"description":     record.GetString("description"),
		"type":            record.GetString("type"),
		"severity":        record.GetString("severity"),
		"status":          record.GetString("status"),
		"monitor":         record.GetString("monitor"),
		"domain":          record.GetString("domain"),
		"system":          record.GetString("system"),
		"assigned_to":     record.GetString("assigned_to"),
		"started_at":      record.GetDateTime("started_at").String(),
		"acknowledged_at": record.GetDateTime("acknowledged_at").String(),
		"resolved_at":     record.GetDateTime("resolved_at").String(),
		"closed_at":       record.GetDateTime("closed_at").String(),
		"resolution":      record.GetString("resolution"),
		"root_cause":      record.GetString("root_cause"),
		"created":         record.GetDateTime("created").String(),
		"updated":         record.GetDateTime("updated").String(),
	}
}

func strPtr(s string) *string {
	return &s
}
