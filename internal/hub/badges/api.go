package badges

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
)

// APIHandler handles badge generation requests
type APIHandler struct {
	app core.App
}

// NewAPIHandler creates a new badges API handler
func NewAPIHandler(app core.App) *APIHandler {
	return &APIHandler{app: app}
}

// RegisterRoutes registers badge API routes
func (h *APIHandler) RegisterRoutes(se *core.ServeEvent) {
	// Public badge endpoints (no auth required)
	se.Router.GET("/badge/:type/:id", h.generateBadge)
	se.Router.GET("/badge/:type/:id.svg", h.generateBadge)

	// Protected badge management
	api := se.Router.Group("/api/beszel/badges")
	api.GET("/", h.listBadges)
	api.POST("/", h.createBadge)
	api.DELETE("/{id}", h.deleteBadge)
}

// BadgeRequest for creating a badge
type BadgeRequest struct {
	Name         string `json:"name"`
	MonitorID    string `json:"monitor_id,omitempty"`
	DomainID     string `json:"domain_id,omitempty"`
	SystemID     string `json:"system_id,omitempty"`
	StatusPageID string `json:"status_page_id,omitempty"`
	Type         string `json:"type"` // uptime, status, response, cert
	Label        string `json:"label,omitempty"`
	Color        string `json:"color,omitempty"`
	Style        string `json:"style,omitempty"` // flat, flat-square, plastic, for-the-badge
}

// listBadges lists all badges for the authenticated user
func (h *APIHandler) listBadges(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	records, err := h.app.FindAllRecords("badges",
		dbx.NewExp("user = {:user}", dbx.Params{"user": authRecord.Id}),
	)
	if err != nil {
		return e.InternalServerError("failed to fetch badges", err)
	}

	badges := make([]map[string]interface{}, 0, len(records))
	for _, record := range records {
		badges = append(badges, map[string]interface{}{
			"id":             record.Id,
			"name":           record.GetString("name"),
			"type":           record.GetString("type"),
			"monitor_id":     record.GetString("monitor"),
			"domain_id":      record.GetString("domain"),
			"system_id":      record.GetString("system"),
			"status_page_id": record.GetString("status_page"),
			"label":          record.GetString("label"),
			"color":          record.GetString("color"),
			"style":          record.GetString("style"),
			"created":        record.GetDateTime("created").Time(),
		})
	}

	return e.JSON(http.StatusOK, badges)
}

// createBadge creates a new badge configuration
func (h *APIHandler) createBadge(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	var req BadgeRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("invalid request body", err)
	}

	if req.Name == "" || req.Type == "" {
		return e.BadRequestError("name and type are required", nil)
	}

	collection, err := h.app.FindCollectionByNameOrId("badges")
	if err != nil {
		return e.InternalServerError("failed to find collection", err)
	}

	record := core.NewRecord(collection)
	record.Set("name", req.Name)
	record.Set("type", req.Type)
	record.Set("monitor", req.MonitorID)
	record.Set("domain", req.DomainID)
	record.Set("system", req.SystemID)
	record.Set("status_page", req.StatusPageID)
	record.Set("label", req.Label)
	record.Set("color", req.Color)
	record.Set("style", req.Style)
	record.Set("user", authRecord.Id)

	if err := h.app.Save(record); err != nil {
		return e.InternalServerError("failed to create badge", err)
	}

	// Generate badge URL
	badgeURL := fmt.Sprintf("/badge/%s/%s.svg", req.Type, record.Id)

	return e.JSON(http.StatusCreated, map[string]interface{}{
		"id":         record.Id,
		"name":       req.Name,
		"type":       req.Type,
		"url":        badgeURL,
		"embed_code": fmt.Sprintf(`<img src="%s" alt="status">`, badgeURL),
		"markdown":   fmt.Sprintf(`![Status](%s)`, badgeURL),
	})
}

// deleteBadge deletes a badge
func (h *APIHandler) deleteBadge(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("badges", id)
	if err != nil {
		return e.NotFoundError("badge not found", err)
	}

	if record.GetString("user") != authRecord.Id {
		return e.ForbiddenError("not authorized", nil)
	}

	if err := h.app.Delete(record); err != nil {
		return e.InternalServerError("failed to delete badge", err)
	}

	return e.NoContent(http.StatusNoContent)
}

// generateBadge generates an SVG badge
func (h *APIHandler) generateBadge(e *core.RequestEvent) error {
	badgeType := e.Request.PathValue("type")
	id := e.Request.PathValue("id")

	// Get query parameters for customization
	label := e.Request.URL.Query().Get("label")
	color := e.Request.URL.Query().Get("color")
	style := e.Request.URL.Query().Get("style")
	if style == "" {
		style = "flat"
	}

	// Find the resource and get status/uptime
	var status, message, badgeColor string

	switch badgeType {
	case "monitor", "status":
		record, err := h.app.FindRecordById("monitors", id)
		if err != nil {
			return e.NotFoundError("monitor not found", err)
		}
		status = record.GetString("status")
		if label == "" {
			label = record.GetString("name")
		}
		message = status
		if status == "up" {
			badgeColor = "brightgreen"
		} else if status == "down" {
			badgeColor = "red"
		} else {
			badgeColor = "yellow"
		}

	case "uptime":
		record, err := h.app.FindRecordById("monitors", id)
		if err != nil {
			return e.NotFoundError("monitor not found", err)
		}
		if label == "" {
			label = "uptime"
		}
		// Get uptime from stats
		uptimeStats := record.GetString("uptime_stats")
		if uptimeStats != "" {
			// Parse simple uptime value if available
			message = uptimeStats + "%"
		} else {
			message = "unknown"
		}
		badgeColor = "blue"

	case "domain":
		record, err := h.app.FindRecordById("domains", id)
		if err != nil {
			return e.NotFoundError("domain not found", err)
		}
		status = record.GetString("status")
		if label == "" {
			label = record.GetString("domain_name")
		}
		message = status
		if status == "active" {
			badgeColor = "brightgreen"
		} else if status == "expiring" {
			badgeColor = "yellow"
		} else if status == "expired" {
			badgeColor = "red"
		} else {
			badgeColor = "lightgrey"
		}

	case "system":
		record, err := h.app.FindRecordById("systems", id)
		if err != nil {
			return e.NotFoundError("system not found", err)
		}
		status = record.GetString("status")
		if label == "" {
			label = record.GetString("name")
		}
		message = status
		if status == "up" {
			badgeColor = "brightgreen"
		} else {
			badgeColor = "red"
		}

	case "response":
		record, err := h.app.FindRecordById("monitors", id)
		if err != nil {
			return e.NotFoundError("monitor not found", err)
		}
		if label == "" {
			label = "response"
		}
		responseTime := record.GetInt("last_response_time")
		message = fmt.Sprintf("%dms", responseTime)
		if responseTime < 200 {
			badgeColor = "brightgreen"
		} else if responseTime < 500 {
			badgeColor = "yellow"
		} else {
			badgeColor = "red"
		}

	default:
		return e.BadRequestError("invalid badge type", nil)
	}

	// Override color if provided
	if color != "" {
		badgeColor = color
	}

	// Generate SVG badge
	svg := generateSVGBadge(label, message, badgeColor, style)

	e.Response.Header().Set("Content-Type", "image/svg+xml")
	e.Response.Header().Set("Cache-Control", "no-cache")
	return e.String(http.StatusOK, svg)
}

// generateSVGBadge creates an SVG badge
func generateSVGBadge(label, message, color, style string) string {
	labelWidth := len(label)*6 + 10
	messageWidth := len(message)*6 + 10
	totalWidth := labelWidth + messageWidth

	// Colors
	labelColor := "#555"
	if style == "flat-square" || style == "for-the-badge" {
		labelColor = "#555"
	}

	// SVG template
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="%d" height="20" role="img" aria-label="%s: %s">
	<title>%s: %s</title>
	<linearGradient id="s" x2="0" y2="100%%">
		<stop offset="0" stop-color="#bbb" stop-opacity=".1"/>
		<stop offset="1" stop-opacity=".1"/>
	</linearGradient>
	<clipPath id="r">
		<rect width="%d" height="20" rx="3" fill="#fff"/>
	</clipPath>
	<g clip-path="url(#r)">
		<rect width="%d" height="20" fill="%s"/>
		<rect x="%d" width="%d" height="20" fill="#%s"/>
		<rect width="%d" height="20" fill="url(#s)"/>
	</g>
	<g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" font-size="11">
		<text x="%d" y="14" fill="#010101" fill-opacity=".3">%s</text>
		<text x="%d" y="13">%s</text>
		<text x="%d" y="14" fill="#010101" fill-opacity=".3">%s</text>
		<text x="%d" y="13">%s</text>
	</g>
</svg>`,
		totalWidth, label, message,
		label, message,
		totalWidth,
		labelWidth, labelColor,
		labelWidth, messageWidth, color,
		totalWidth,
		labelWidth/2, label,
		labelWidth/2, label,
		labelWidth+messageWidth/2, message,
		labelWidth+messageWidth/2, message,
	)

	return svg
}

// getOverallStatusPageStatus calculates overall status for a status page
func (h *APIHandler) getOverallStatusPageStatus(statusPageID string) (string, float64) {
	// Get all monitors linked to this status page
	links, err := h.app.FindAllRecords("status_page_monitors",
		dbx.NewExp("status_page = {:statusPage}", dbx.Params{"statusPage": statusPageID}),
	)
	if err != nil {
		return "unknown", 0
	}

	upCount := 0
	downCount := 0
	totalCount := len(links)

	for _, link := range links {
		monitorID := link.GetString("monitor")
		monitor, err := h.app.FindRecordById("monitors", monitorID)
		if err != nil {
			continue
		}

		status := monitor.GetString("status")
		if status == "up" {
			upCount++
		} else if status == "down" {
			downCount++
		}
	}

	if totalCount == 0 {
		return "unknown", 0
	}

	uptime := float64(upCount) / float64(totalCount) * 100

	if downCount > 0 {
		return "down", uptime
	} else if upCount == totalCount {
		return "up", uptime
	}
	return "degraded", uptime
}

// GetEmbedCode generates embed code for a badge
func GetEmbedCode(badgeURL, format string) string {
	switch format {
	case "html":
		return fmt.Sprintf(`<img src="%s" alt="status badge">`, badgeURL)
	case "markdown":
		return fmt.Sprintf(`![Status](%s)`, badgeURL)
	case "rst":
		return fmt.Sprintf(`.. image:: %s
   :alt: status badge`, badgeURL)
	case "asciidoc":
		return fmt.Sprintf(`image:%s[Status]`, badgeURL)
	default:
		return fmt.Sprintf(`<img src="%s" alt="status badge">`, badgeURL)
	}
}

// FormatDuration formats a duration for display
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	} else {
		days := int(d.Hours()) / 24
		return fmt.Sprintf("%dd", days)
	}
}

// ParseDuration parses a duration string (e.g., "24h", "7d")
func ParseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}

	// Try to parse as number of hours
	if hours, err := strconv.Atoi(s); err == nil {
		return time.Duration(hours) * time.Hour, nil
	}

	// Parse with suffix
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(s[:len(s)-1])
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	if strings.HasSuffix(s, "h") {
		hours, err := strconv.Atoi(s[:len(s)-1])
		if err != nil {
			return 0, err
		}
		return time.Duration(hours) * time.Hour, nil
	}

	if strings.HasSuffix(s, "m") {
		minutes, err := strconv.Atoi(s[:len(s)-1])
		if err != nil {
			return 0, err
		}
		return time.Duration(minutes) * time.Minute, nil
	}

	return time.ParseDuration(s)
}
