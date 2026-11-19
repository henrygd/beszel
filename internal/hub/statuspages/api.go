package statuspages

import (
	"encoding/json"
	"net/http"
	"strings"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/apis"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/henrygd/beszel/internal/entities/statuspage"
)

// APIHandler handles status page API requests
type APIHandler struct {
	app core.App
}

// NewAPIHandler creates a new status page API handler
func NewAPIHandler(app core.App) *APIHandler {
	return &APIHandler{app: app}
}

// RegisterRoutes registers status page API routes
func (h *APIHandler) RegisterRoutes(se *core.ServeEvent) {
	// Public status page (no auth required)
	se.Router.GET("/status/:slug", h.getPublicStatusPage)

	// Protected routes
	api := se.Router.Group("/api/beszel/status-pages")
	api.Bind(apis.RequireAuth())

	api.GET("/", h.listStatusPages)
	api.POST("/", h.createStatusPage)
	api.GET("/{id}", h.getStatusPage)
	api.PATCH("/{id}", h.updateStatusPage)
	api.DELETE("/{id}", h.deleteStatusPage)
	api.POST("/{id}/monitors", h.addMonitor)
	api.DELETE("/{id}/monitors/{monitorId}", h.removeMonitor)
	api.GET("/{id}/monitors", h.listMonitors)
}

// listStatusPages lists all status pages for the authenticated user
func (h *APIHandler) listStatusPages(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	records, err := h.app.FindAllRecords("status_pages",
		dbx.NewExp("user = {:user}", dbx.Params{"user": authRecord.Id}),
	)
	if err != nil {
		return e.InternalServerError("failed to fetch status pages", err)
	}

	pages := make([]statuspage.StatusPageResponse, 0, len(records))
	for _, record := range records {
		pages = append(pages, h.recordToResponse(record))
	}

	return e.JSON(http.StatusOK, pages)
}

// createStatusPage creates a new status page
func (h *APIHandler) createStatusPage(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	var req statuspage.CreateStatusPageRequest
	if err := json.NewDecoder(e.Request.Body).Decode(&req); err != nil {
		return e.BadRequestError("invalid request body", err)
	}

	if req.Name == "" || req.Slug == "" {
		return e.BadRequestError("name and slug are required", nil)
	}

	// Check if slug is unique
	existing, _ := h.app.FindFirstRecordByFilter("status_pages", "slug = {:slug}",
		dbx.Params{"slug": req.Slug})
	if existing != nil {
		return e.BadRequestError("slug already exists", nil)
	}

	collection, err := h.app.FindCollectionByNameOrId("status_pages")
	if err != nil {
		return e.InternalServerError("failed to find collection", err)
	}

	record := core.NewRecord(collection)
	record.Set("name", req.Name)
	record.Set("slug", strings.ToLower(req.Slug))
	record.Set("title", req.Title)
	record.Set("description", req.Description)
	record.Set("logo", req.Logo)
	record.Set("favicon", req.Favicon)
	record.Set("theme", statuspage.ValidateTheme(req.Theme))
	record.Set("custom_css", req.CustomCSS)
	record.Set("public", req.Public)
	record.Set("show_uptime", req.ShowUptime)
	record.Set("user", authRecord.Id)

	if err := h.app.Save(record); err != nil {
		return e.InternalServerError("failed to create status page", err)
	}

	return e.JSON(http.StatusCreated, h.recordToResponse(record))
}

// getStatusPage gets a single status page
func (h *APIHandler) getStatusPage(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("status_pages", id)
	if err != nil {
		return e.NotFoundError("status page not found", err)
	}

	if record.GetString("user") != authRecord.Id {
		return e.ForbiddenError("not authorized", nil)
	}

	return e.JSON(http.StatusOK, h.recordToResponse(record))
}

// updateStatusPage updates a status page
func (h *APIHandler) updateStatusPage(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("status_pages", id)
	if err != nil {
		return e.NotFoundError("status page not found", err)
	}

	if record.GetString("user") != authRecord.Id {
		return e.ForbiddenError("not authorized", nil)
	}

	var req statuspage.UpdateStatusPageRequest
	if err := json.NewDecoder(e.Request.Body).Decode(&req); err != nil {
		return e.BadRequestError("invalid request body", err)
	}

	if req.Name != nil {
		record.Set("name", *req.Name)
	}
	if req.Title != nil {
		record.Set("title", *req.Title)
	}
	if req.Description != nil {
		record.Set("description", *req.Description)
	}
	if req.Logo != nil {
		record.Set("logo", *req.Logo)
	}
	if req.Favicon != nil {
		record.Set("favicon", *req.Favicon)
	}
	if req.Theme != nil {
		record.Set("theme", statuspage.ValidateTheme(*req.Theme))
	}
	if req.CustomCSS != nil {
		record.Set("custom_css", *req.CustomCSS)
	}
	if req.Public != nil {
		record.Set("public", *req.Public)
	}
	if req.ShowUptime != nil {
		record.Set("show_uptime", *req.ShowUptime)
	}

	if err := h.app.Save(record); err != nil {
		return e.InternalServerError("failed to update status page", err)
	}

	return e.JSON(http.StatusOK, h.recordToResponse(record))
}

// deleteStatusPage deletes a status page
func (h *APIHandler) deleteStatusPage(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("status_pages", id)
	if err != nil {
		return e.NotFoundError("status page not found", err)
	}

	if record.GetString("user") != authRecord.Id {
		return e.ForbiddenError("not authorized", nil)
	}

	if err := h.app.Delete(record); err != nil {
		return e.InternalServerError("failed to delete status page", err)
	}

	return e.NoContent(http.StatusNoContent)
}

// addMonitor adds a monitor to a status page
func (h *APIHandler) addMonitor(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	statusPageID := e.Request.PathValue("id")
	statusPage, err := h.app.FindRecordById("status_pages", statusPageID)
	if err != nil {
		return e.NotFoundError("status page not found", err)
	}

	if statusPage.GetString("user") != authRecord.Id {
		return e.ForbiddenError("not authorized", nil)
	}

	var req statuspage.StatusPageMonitorRequest
	if err := json.NewDecoder(e.Request.Body).Decode(&req); err != nil {
		return e.BadRequestError("invalid request body", err)
	}

	// Verify monitor exists and belongs to user
	monitorRecord, err := h.app.FindRecordById("monitors", req.MonitorID)
	if err != nil {
		return e.NotFoundError("monitor not found", err)
	}

	if monitorRecord.GetString("user") != authRecord.Id {
		return e.ForbiddenError("not authorized for this monitor", nil)
	}

	collection, err := h.app.FindCollectionByNameOrId("status_page_monitors")
	if err != nil {
		return e.InternalServerError("failed to find collection", err)
	}

	record := core.NewRecord(collection)
	record.Set("status_page", statusPageID)
	record.Set("monitor", req.MonitorID)
	record.Set("display_name", req.DisplayName)
	record.Set("group", req.Group)
	record.Set("sort_order", req.SortOrder)
	record.Set("user", authRecord.Id)

	if err := h.app.Save(record); err != nil {
		return e.InternalServerError("failed to add monitor", err)
	}

	return e.JSON(http.StatusCreated, map[string]string{"status": "added"})
}

// removeMonitor removes a monitor from a status page
func (h *APIHandler) removeMonitor(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	statusPageID := e.Request.PathValue("id")
	monitorID := e.Request.PathValue("monitorId")

	// Find the link record
	records, err := h.app.FindAllRecords("status_page_monitors",
		dbx.HashExp{
			"status_page": statusPageID,
			"monitor":     monitorID,
			"user":        authRecord.Id,
		},
	)
	if err != nil || len(records) == 0 {
		return e.NotFoundError("monitor link not found", err)
	}

	if err := h.app.Delete(records[0]); err != nil {
		return e.InternalServerError("failed to remove monitor", err)
	}

	return e.NoContent(http.StatusNoContent)
}

// listMonitors lists monitors on a status page
func (h *APIHandler) listMonitors(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	statusPageID := e.Request.PathValue("id")
	statusPage, err := h.app.FindRecordById("status_pages", statusPageID)
	if err != nil {
		return e.NotFoundError("status page not found", err)
	}

	if statusPage.GetString("user") != authRecord.Id {
		return e.ForbiddenError("not authorized", nil)
	}

	records, err := h.app.FindAllRecords("status_page_monitors",
		dbx.NewExp("status_page = {:statusPage}", dbx.Params{"statusPage": statusPageID}),
	)
	if err != nil {
		return e.InternalServerError("failed to fetch monitors", err)
	}

	monitors := make([]map[string]interface{}, 0, len(records))
	for _, record := range records {
		monitors = append(monitors, map[string]interface{}{
			"id":           record.Id,
			"monitor_id":   record.GetString("monitor"),
			"display_name": record.GetString("display_name"),
			"group":        record.GetString("group"),
			"sort_order":   record.GetInt("sort_order"),
		})
	}

	return e.JSON(http.StatusOK, monitors)
}

// getPublicStatusPage gets a public status page by slug
func (h *APIHandler) getPublicStatusPage(e *core.RequestEvent) error {
	slug := e.Request.PathValue("slug")

	record, err := h.app.FindFirstRecordByFilter("status_pages", "slug = {:slug} && public = true",
		dbx.Params{"slug": slug})
	if err != nil {
		return e.NotFoundError("status page not found", err)
	}

	// Build public status page
	publicPage := h.buildPublicStatusPage(record)

	return e.JSON(http.StatusOK, publicPage)
}

// buildPublicStatusPage builds a public status page from a record
func (h *APIHandler) buildPublicStatusPage(record *core.Record) *statuspage.PublicStatusPage {
	statusPageID := record.Id

	// Get linked monitors
	links, err := h.app.FindAllRecords("status_page_monitors",
		dbx.NewExp("status_page = {:statusPage}", dbx.Params{"statusPage": statusPageID}),
	)
	if err != nil {
		links = []*core.Record{}
	}

	publicMonitors := make([]statuspage.PublicMonitorStatus, 0, len(links))
	overallStatus := statuspage.StatusOperational

	for _, link := range links {
		monitorID := link.GetString("monitor")
		monitorRecord, err := h.app.FindRecordById("monitors", monitorID)
		if err != nil {
			continue
		}

		status := monitorRecord.GetString("status")
		if status == string(monitor.StatusDown) && overallStatus == statuspage.StatusOperational {
			overallStatus = statuspage.StatusMajor
		}

		uptimeStats := make(map[string]float64)
		if statsJSON := monitorRecord.GetString("uptime_stats"); statsJSON != "" {
			json.Unmarshal([]byte(statsJSON), &uptimeStats)
		}

		publicMonitors = append(publicMonitors, statuspage.PublicMonitorStatus{
			ID:          monitorID,
			Name:        monitorRecord.GetString("name"),
			DisplayName: link.GetString("display_name"),
			Group:       link.GetString("group"),
			Status:      status,
			Uptime24h:   uptimeStats["24h"],
			Uptime7d:    uptimeStats["7d"],
			Uptime30d:   uptimeStats["30d"],
			LastCheck:   monitorRecord.GetDateTime("last_check").Time(),
		})
	}

	// Get active incidents for this user (not closed)
	var publicIncidents []statuspage.PublicIncident
	incidentRecords, _ := h.app.FindAllRecords("incidents",
		dbx.NewExp("user = {:user} && status != {:status}",
			dbx.Params{"user": record.GetString("user"), "status": "closed"}),
	)
	for _, incident := range incidentRecords {
		publicIncidents = append(publicIncidents, statuspage.PublicIncident{
			ID:          incident.Id,
			Title:       incident.GetString("title"),
			Description: incident.GetString("description"),
			Status:      incident.GetString("status"),
			Severity:    incident.GetString("severity"),
			StartedAt:   incident.GetDateTime("started_at").Time(),
		})
	}

	return &statuspage.PublicStatusPage{
		ID:            record.Id,
		Name:          record.GetString("name"),
		Title:         record.GetString("title"),
		Description:   record.GetString("description"),
		Logo:          record.GetString("logo"),
		Favicon:       record.GetString("favicon"),
		Theme:         record.GetString("theme"),
		CustomCSS:     record.GetString("custom_css"),
		Monitors:      publicMonitors,
		Incidents:     publicIncidents,
		OverallStatus: overallStatus,
		UpdatedAt:     record.GetDateTime("updated").Time(),
	}
}

// recordToResponse converts a record to a response
func (h *APIHandler) recordToResponse(record *core.Record) statuspage.StatusPageResponse {
	// Count monitors
	count := 0
	links, _ := h.app.FindAllRecords("status_page_monitors",
		dbx.NewExp("status_page = {:statusPage}", dbx.Params{"statusPage": record.Id}),
	)
	count = len(links)

	return statuspage.StatusPageResponse{
		ID:           record.Id,
		Name:         record.GetString("name"),
		Slug:         record.GetString("slug"),
		Title:        record.GetString("title"),
		Description:  record.GetString("description"),
		Logo:         record.GetString("logo"),
		Favicon:      record.GetString("favicon"),
		Theme:        record.GetString("theme"),
		Public:       record.GetBool("public"),
		ShowUptime:   record.GetBool("show_uptime"),
		MonitorCount: count,
		Created:      record.GetDateTime("created").String(),
		Updated:      record.GetDateTime("updated").String(),
	}
}
