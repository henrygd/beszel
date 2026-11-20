package monitors

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/henrygd/beszel/internal/hub/pagespeed"
	"github.com/henrygd/beszel/internal/hub/utils"
)

// APIHandler handles monitor API endpoints
type APIHandler struct {
	app       core.App
	scheduler *Scheduler
}

// NewAPIHandler creates a new monitor API handler
func NewAPIHandler(app core.App, scheduler *Scheduler) *APIHandler {
	return &APIHandler{
		app:       app,
		scheduler: scheduler,
	}
}

// RegisterRoutes registers monitor API routes
func (h *APIHandler) RegisterRoutes(se *core.ServeEvent) {
	api := se.Router.Group("/api/beszel/monitors")

	// Require auth for all routes
	api.BindFunc(func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return e.UnauthorizedError("Authentication required", nil)
		}
		return e.Next()
	})

	// CRUD endpoints
	api.GET("", h.listMonitors)
	api.POST("", h.createMonitor)
	api.GET("/{id}", h.getMonitor)
	api.PATCH("/{id}", h.updateMonitor)
	api.DELETE("/{id}", h.deleteMonitor)

	// Action endpoints
	api.POST("/{id}/check", h.manualCheck)
	api.POST("/{id}/pause", h.pauseMonitor)
	api.POST("/{id}/resume", h.resumeMonitor)
	api.GET("/{id}/stats", h.getStats)
	api.GET("/{id}/heartbeats", h.getHeartbeats)
	api.POST("/{id}/pagespeed", h.runPageSpeedCheck)
	api.GET("/{id}/notifications", h.listMonitorNotifications)
	api.PUT("/{id}/notifications", h.setMonitorNotifications)
}

// HeartbeatSummary represents a minimal heartbeat for the monitor list
type HeartbeatSummary struct {
	Status string    `json:"status"`
	Ping   int       `json:"ping"`
	Time   time.Time `json:"time"`
}

// MonitorResponse represents a monitor in API responses
type MonitorResponse struct {
	ID                     string             `json:"id"`
	Name                   string             `json:"name"`
	Type                   string             `json:"type"`
	URL                    string             `json:"url,omitempty"`
	Hostname               string             `json:"hostname,omitempty"`
	Port                   int                `json:"port,omitempty"`
	Method                 string             `json:"method,omitempty"`
	Interval               int                `json:"interval"`
	Timeout                int                `json:"timeout"`
	Retries                int                `json:"retries"`
	Status                 string             `json:"status"`
	Active                 bool               `json:"active"`
	Description            string             `json:"description,omitempty"`
	LastCheck              *time.Time         `json:"last_check,omitempty"`
	UptimeStats            map[string]float64 `json:"uptime_stats,omitempty"`
	RecentHeartbeats       []HeartbeatSummary `json:"recent_heartbeats,omitempty"`
	Tags                   []string           `json:"tags,omitempty"`
	Keyword                string             `json:"keyword,omitempty"`
	JSONQuery              string             `json:"json_query,omitempty"`
	ExpectedValue          string             `json:"expected_value,omitempty"`
	InvertKeyword          bool               `json:"invert_keyword"`
	DNSResolveServer       string             `json:"dns_resolve_server,omitempty"`
	DNSResolverMode        string             `json:"dns_resolver_mode,omitempty"`
	CertExpiryNotification bool               `json:"cert_expiry_notification"`
	CertExpiryDays         int                `json:"cert_expiry_days,omitempty"`
	IgnoreTLSError         bool               `json:"ignore_tls_error"`
	NotifyOnDown           bool               `json:"notify_on_down"`
	NotifyOnRecover        bool               `json:"notify_on_recover"`
	NotifyOnResponseTime   bool               `json:"notify_on_response_time"`
	ResponseTimeThreshold  int                `json:"response_time_threshold"`
	NotifyOnUptimeDrop     bool               `json:"notify_on_uptime_drop"`
	UptimeThreshold        float64            `json:"uptime_threshold"`
	NotifyRepeatedFailures bool               `json:"notify_repeated_failures"`
	ConsecutiveFailures    int                `json:"consecutive_failures"`
	QuietHoursEnabled      bool               `json:"quiet_hours_enabled"`
	QuietHoursStart        string             `json:"quiet_hours_start"`
	QuietHoursEnd          string             `json:"quiet_hours_end"`
	Created                time.Time          `json:"created"`
	Updated                time.Time          `json:"updated"`
}

// CreateMonitorRequest represents a request to create a monitor
// MonitorNotificationFields holds optional notification/extra fields shared
// by create and update requests; nil pointers leave existing values alone.
type MonitorNotificationFields struct {
	NotifyOnDown           *bool    `json:"notify_on_down,omitempty"`
	NotifyOnRecover        *bool    `json:"notify_on_recover,omitempty"`
	NotifyOnResponseTime   *bool    `json:"notify_on_response_time,omitempty"`
	ResponseTimeThreshold  *int     `json:"response_time_threshold,omitempty"`
	NotifyOnUptimeDrop     *bool    `json:"notify_on_uptime_drop,omitempty"`
	UptimeThreshold        *float64 `json:"uptime_threshold,omitempty"`
	NotifyRepeatedFailures *bool    `json:"notify_repeated_failures,omitempty"`
	ConsecutiveFailures    *int     `json:"consecutive_failures,omitempty"`
	QuietHoursEnabled      *bool    `json:"quiet_hours_enabled,omitempty"`
	QuietHoursStart        *string  `json:"quiet_hours_start,omitempty"`
	QuietHoursEnd          *string  `json:"quiet_hours_end,omitempty"`
	DbUsername             *string  `json:"db_username,omitempty"`
	DbPassword             *string  `json:"db_password,omitempty"`
	DbName                 *string  `json:"db_name,omitempty"`
	MqttTopic              *string  `json:"mqtt_topic,omitempty"`
	GrpcKeyword            *string  `json:"grpc_keyword,omitempty"`
}

// applyToRecord writes only the non-nil fields onto the record
func (n *MonitorNotificationFields) applyToRecord(record *core.Record) {
	fields := []struct {
		name string
		val  interface{}
	}{
		{"notify_on_down", n.NotifyOnDown},
		{"notify_on_recover", n.NotifyOnRecover},
		{"notify_on_response_time", n.NotifyOnResponseTime},
		{"response_time_threshold", n.ResponseTimeThreshold},
		{"notify_on_uptime_drop", n.NotifyOnUptimeDrop},
		{"uptime_threshold", n.UptimeThreshold},
		{"notify_repeated_failures", n.NotifyRepeatedFailures},
		{"consecutive_failures", n.ConsecutiveFailures},
		{"quiet_hours_enabled", n.QuietHoursEnabled},
		{"quiet_hours_start", n.QuietHoursStart},
		{"quiet_hours_end", n.QuietHoursEnd},
		{"db_username", n.DbUsername},
		{"db_password", n.DbPassword},
		{"db_name", n.DbName},
		{"mqtt_topic", n.MqttTopic},
		{"grpc_keyword", n.GrpcKeyword},
	}
	for _, f := range fields {
		switch v := f.val.(type) {
		case *bool:
			if v != nil {
				record.Set(f.name, *v)
			}
		case *int:
			if v != nil {
				record.Set(f.name, *v)
			}
		case *float64:
			if v != nil {
				record.Set(f.name, *v)
			}
		case *string:
			if v != nil {
				record.Set(f.name, *v)
			}
		}
	}
}

type CreateMonitorRequest struct {
	Name                   string   `json:"name"`
	Type                   string   `json:"type"`
	URL                    string   `json:"url,omitempty"`
	Hostname               string   `json:"hostname,omitempty"`
	Port                   int      `json:"port,omitempty"`
	Method                 string   `json:"method,omitempty"`
	Headers                string   `json:"headers,omitempty"`
	Body                   string   `json:"body,omitempty"`
	Interval               int      `json:"interval"`
	Timeout                int      `json:"timeout"`
	Retries                int      `json:"retries,omitempty"`
	RetryInterval          int      `json:"retry_interval,omitempty"`
	MaxRedirects           int      `json:"max_redirects,omitempty"`
	Keyword                string   `json:"keyword,omitempty"`
	JSONQuery              string   `json:"json_query,omitempty"`
	ExpectedValue          string   `json:"expected_value,omitempty"`
	InvertKeyword          bool     `json:"invert_keyword,omitempty"`
	DNSResolveServer       string   `json:"dns_resolve_server,omitempty"`
	DNSResolverMode        string   `json:"dns_resolver_mode,omitempty"`
	Description            string   `json:"description,omitempty"`
	Tags                   []string `json:"tags,omitempty"`
	CertExpiryNotification bool     `json:"cert_expiry_notification,omitempty"`
	CertExpiryDays         int      `json:"cert_expiry_days,omitempty"`
	IgnoreTLSError         bool     `json:"ignore_tls_error,omitempty"`
	MonitorNotificationFields
}

// UpdateMonitorRequest represents a request to update a monitor
type UpdateMonitorRequest struct {
	Name                   *string  `json:"name,omitempty"`
	URL                    *string  `json:"url,omitempty"`
	Hostname               *string  `json:"hostname,omitempty"`
	Port                   *int     `json:"port,omitempty"`
	Method                 *string  `json:"method,omitempty"`
	Headers                *string  `json:"headers,omitempty"`
	Body                   *string  `json:"body,omitempty"`
	Interval               *int     `json:"interval,omitempty"`
	Timeout                *int     `json:"timeout,omitempty"`
	Retries                *int     `json:"retries,omitempty"`
	RetryInterval          *int     `json:"retry_interval,omitempty"`
	MaxRedirects           *int     `json:"max_redirects,omitempty"`
	Keyword                *string  `json:"keyword,omitempty"`
	JSONQuery              *string  `json:"json_query,omitempty"`
	ExpectedValue          *string  `json:"expected_value,omitempty"`
	InvertKeyword          *bool    `json:"invert_keyword,omitempty"`
	DNSResolveServer       *string  `json:"dns_resolve_server,omitempty"`
	DNSResolverMode        *string  `json:"dns_resolver_mode,omitempty"`
	Active                 *bool    `json:"active,omitempty"`
	Description            *string  `json:"description,omitempty"`
	Tags                   []string `json:"tags,omitempty"`
	CertExpiryNotification *bool    `json:"cert_expiry_notification,omitempty"`
	CertExpiryDays         *int     `json:"cert_expiry_days,omitempty"`
	IgnoreTLSError         *bool    `json:"ignore_tls_error,omitempty"`
	MonitorNotificationFields
}

// listMonitors returns all monitors for the authenticated user
func (h *APIHandler) listMonitors(e *core.RequestEvent) error {
	userID := e.Auth.Id

	records, err := h.app.FindRecordsByFilter(
		"monitors",
		"user = {:userId}",
		"-created",
		0,
		0,
		map[string]any{"userId": userID},
	)
	if err != nil {
		return e.InternalServerError("Failed to fetch monitors", err)
	}

	monitors := make([]MonitorResponse, 0, len(records))
	for _, record := range records {
		monitors = append(monitors, recordToResponse(record))
	}

	return e.JSON(http.StatusOK, map[string]interface{}{
		"monitors": monitors,
	})
}

// getRecentHeartbeats fetches the last N heartbeats for a monitor
func (h *APIHandler) getRecentHeartbeats(monitorID string, limit int) []HeartbeatSummary {
	records, err := h.app.FindRecordsByFilter(
		"monitor_heartbeats",
		"monitor = {:monitorId}",
		"-time",
		limit,
		0,
		map[string]any{"monitorId": monitorID},
	)
	if err != nil {
		return nil
	}

	heartbeats := make([]HeartbeatSummary, 0, len(records))
	for _, hb := range records {
		var t time.Time
		if ts := hb.GetDateTime("time"); !ts.IsZero() {
			t = ts.Time()
		}
		heartbeats = append(heartbeats, HeartbeatSummary{
			Status: hb.GetString("status"),
			Ping:   hb.GetInt("ping"),
			Time:   t,
		})
	}
	return heartbeats
}

// getMonitor returns a single monitor by ID
func (h *APIHandler) getMonitor(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	if id == "" {
		return e.BadRequestError("Monitor ID is required", nil)
	}

	record, err := h.app.FindRecordById("monitors", id)
	if err != nil {
		return e.NotFoundError("Monitor not found", err)
	}

	// Verify ownership
	if record.GetString("user") != e.Auth.Id {
		return e.ForbiddenError("Access denied", nil)
	}

	resp := recordToResponse(record)
	resp.RecentHeartbeats = h.getRecentHeartbeats(id, 12)

	return e.JSON(http.StatusOK, resp)
}

// createMonitor creates a new monitor
func (h *APIHandler) createMonitor(e *core.RequestEvent) error {
	var req CreateMonitorRequest
	if err := json.NewDecoder(e.Request.Body).Decode(&req); err != nil {
		return e.BadRequestError("Invalid request body", err)
	}

	// Validate required fields
	if req.Name == "" || req.Type == "" {
		return e.BadRequestError("Name and type are required", nil)
	}

	// Set defaults
	if req.Interval == 0 {
		req.Interval = 60
	}
	if req.Timeout == 0 {
		req.Timeout = 30
	}
	if req.Retries == 0 {
		req.Retries = 1
	}

	// Get collection
	collection, err := h.app.FindCollectionByNameOrId("monitors")
	if err != nil {
		return e.InternalServerError("Failed to get collection", err)
	}

	// Create record
	record := core.NewRecord(collection)
	record.Set("name", req.Name)
	record.Set("type", req.Type)
	record.Set("url", req.URL)
	record.Set("hostname", req.Hostname)
	record.Set("port", req.Port)
	record.Set("method", req.Method)
	record.Set("headers", req.Headers)
	record.Set("body", req.Body)
	record.Set("interval", req.Interval)
	record.Set("timeout", req.Timeout)
	record.Set("retries", req.Retries)
	record.Set("retry_interval", req.RetryInterval)
	record.Set("max_redirects", req.MaxRedirects)
	record.Set("keyword", req.Keyword)
	record.Set("json_query", req.JSONQuery)
	record.Set("expected_value", req.ExpectedValue)
	record.Set("invert_keyword", req.InvertKeyword)
	record.Set("dns_resolve_server", req.DNSResolveServer)
	record.Set("dns_resolver_mode", req.DNSResolverMode)
	record.Set("status", string(monitor.StatusPending))
	record.Set("active", true)
	record.Set("user", e.Auth.Id)
	record.Set("description", req.Description)
	record.Set("tags", req.Tags)
	record.Set("cert_expiry_notification", req.CertExpiryNotification)
	record.Set("cert_expiry_days", req.CertExpiryDays)
	record.Set("ignore_tls_error", req.IgnoreTLSError)
	if req.NotifyOnDown == nil {
		record.Set("notify_on_down", true)
	}
	if req.NotifyOnRecover == nil {
		record.Set("notify_on_recover", true)
	}
	req.MonitorNotificationFields.applyToRecord(record)
	record.Set("uptime_stats", map[string]float64{})

	if err := h.app.Save(record); err != nil {
		return e.InternalServerError("Failed to create monitor", err)
	}

	// Add to scheduler
	h.scheduler.AddMonitor(record)

	// Run initial check synchronously so the monitor shows real status immediately
	result, err := h.scheduler.RunManualCheck(record.Id)
	if err != nil {
		log.Printf("[monitor-api] Initial check failed for %s: %v", record.Id, err)
		// Note: The monitor will remain in pending status and the scheduler will retry
	} else {
		log.Printf("[monitor-api] Initial check completed for %s: status=%s, ping=%v", record.Id, result.Status, result.Ping)
	}

	// Re-fetch the updated record to get the new status
	updatedRecord, err := h.app.FindRecordById("monitors", record.Id)
	if err == nil {
		record = updatedRecord
	}

	return e.JSON(http.StatusCreated, recordToResponse(record))
}

// updateMonitor updates an existing monitor
func (h *APIHandler) updateMonitor(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	if id == "" {
		return e.BadRequestError("Monitor ID is required", nil)
	}

	record, err := h.app.FindRecordById("monitors", id)
	if err != nil {
		return e.NotFoundError("Monitor not found", err)
	}

	// Verify ownership
	if record.GetString("user") != e.Auth.Id {
		return e.ForbiddenError("Access denied", nil)
	}

	var req UpdateMonitorRequest
	if err := json.NewDecoder(e.Request.Body).Decode(&req); err != nil {
		return e.BadRequestError("Invalid request body", err)
	}

	// Update fields
	if req.Name != nil {
		record.Set("name", *req.Name)
	}
	if req.URL != nil {
		record.Set("url", *req.URL)
	}
	if req.Hostname != nil {
		record.Set("hostname", *req.Hostname)
	}
	if req.Port != nil {
		record.Set("port", *req.Port)
	}
	if req.Method != nil {
		record.Set("method", *req.Method)
	}
	if req.Headers != nil {
		record.Set("headers", *req.Headers)
	}
	if req.Body != nil {
		record.Set("body", *req.Body)
	}
	if req.Interval != nil {
		record.Set("interval", *req.Interval)
	}
	if req.Timeout != nil {
		record.Set("timeout", *req.Timeout)
	}
	if req.Retries != nil {
		record.Set("retries", *req.Retries)
	}
	if req.RetryInterval != nil {
		record.Set("retry_interval", *req.RetryInterval)
	}
	if req.MaxRedirects != nil {
		record.Set("max_redirects", *req.MaxRedirects)
	}
	if req.Keyword != nil {
		record.Set("keyword", *req.Keyword)
	}
	if req.JSONQuery != nil {
		record.Set("json_query", *req.JSONQuery)
	}
	if req.ExpectedValue != nil {
		record.Set("expected_value", *req.ExpectedValue)
	}
	if req.InvertKeyword != nil {
		record.Set("invert_keyword", *req.InvertKeyword)
	}
	if req.DNSResolveServer != nil {
		record.Set("dns_resolve_server", *req.DNSResolveServer)
	}
	if req.DNSResolverMode != nil {
		record.Set("dns_resolver_mode", *req.DNSResolverMode)
	}
	if req.Active != nil {
		record.Set("active", *req.Active)
	}
	if req.Description != nil {
		record.Set("description", *req.Description)
	}
	if req.Tags != nil {
		record.Set("tags", req.Tags)
	}
	if req.CertExpiryNotification != nil {
		record.Set("cert_expiry_notification", *req.CertExpiryNotification)
	}
	if req.CertExpiryDays != nil {
		record.Set("cert_expiry_days", *req.CertExpiryDays)
	}
	if req.IgnoreTLSError != nil {
		record.Set("ignore_tls_error", *req.IgnoreTLSError)
	}
	req.MonitorNotificationFields.applyToRecord(record)

	if err := h.app.Save(record); err != nil {
		return e.InternalServerError("Failed to update monitor", err)
	}

	// Update scheduler
	h.scheduler.UpdateMonitor(record)

	return e.JSON(http.StatusOK, recordToResponse(record))
}

// deleteMonitor deletes a monitor
func (h *APIHandler) deleteMonitor(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	if id == "" {
		return e.BadRequestError("Monitor ID is required", nil)
	}

	record, err := h.app.FindRecordById("monitors", id)
	if err != nil {
		return e.NotFoundError("Monitor not found", err)
	}

	// Verify ownership
	if record.GetString("user") != e.Auth.Id {
		return e.ForbiddenError("Access denied", nil)
	}

	// Remove from scheduler first
	h.scheduler.RemoveMonitor(id)

	if err := h.app.Delete(record); err != nil {
		return e.InternalServerError("Failed to delete monitor", err)
	}

	return e.JSON(http.StatusOK, map[string]string{"message": "Monitor deleted"})
}

// manualCheck runs a manual check for a monitor
func (h *APIHandler) manualCheck(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	if id == "" {
		return e.BadRequestError("Monitor ID is required", nil)
	}

	record, err := h.app.FindRecordById("monitors", id)
	if err != nil {
		return e.NotFoundError("Monitor not found", err)
	}

	// Verify ownership
	if record.GetString("user") != e.Auth.Id {
		return e.ForbiddenError("Access denied", nil)
	}

	result, err := h.scheduler.RunManualCheck(id)
	if err != nil {
		return e.InternalServerError("Check failed", err)
	}

	response := map[string]interface{}{
		"status": result.Status,
		"ping":   result.Ping,
		"msg":    result.Msg,
	}

	if hbs, err := h.app.FindRecordsByFilter(
		"monitor_heartbeats",
		"monitor = {:monitorId}",
		"-time",
		1,
		0,
		dbx.Params{"monitorId": id},
	); err == nil && len(hbs) > 0 {
		response["heartbeat_id"] = hbs[0].Id
		response["time"] = hbs[0].GetDateTime("time").String()
	}

	return e.JSON(http.StatusOK, response)
}

// pauseMonitor pauses a monitor
func (h *APIHandler) pauseMonitor(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	if id == "" {
		return e.BadRequestError("Monitor ID is required", nil)
	}

	record, err := h.app.FindRecordById("monitors", id)
	if err != nil {
		return e.NotFoundError("Monitor not found", err)
	}

	// Verify ownership
	if record.GetString("user") != e.Auth.Id {
		return e.ForbiddenError("Access denied", nil)
	}

	record.Set("active", false)
	record.Set("status", string(monitor.StatusPaused))

	if err := h.app.Save(record); err != nil {
		return e.InternalServerError("Failed to pause monitor", err)
	}

	h.scheduler.UpdateMonitor(record)

	return e.JSON(http.StatusOK, recordToResponse(record))
}

// resumeMonitor resumes a paused monitor
func (h *APIHandler) resumeMonitor(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	if id == "" {
		return e.BadRequestError("Monitor ID is required", nil)
	}

	record, err := h.app.FindRecordById("monitors", id)
	if err != nil {
		return e.NotFoundError("Monitor not found", err)
	}

	// Verify ownership
	if record.GetString("user") != e.Auth.Id {
		return e.ForbiddenError("Access denied", nil)
	}

	record.Set("active", true)
	record.Set("status", string(monitor.StatusPending))

	if err := h.app.Save(record); err != nil {
		return e.InternalServerError("Failed to resume monitor", err)
	}

	h.scheduler.UpdateMonitor(record)

	// Run an immediate check so the monitor shows real status instead of pending
	if _, err := h.scheduler.RunManualCheck(record.Id); err != nil {
		log.Printf("[monitor-api] Resume check failed for %s: %v", record.Id, err)
	}

	// Re-fetch the updated record to get the new status
	updatedRecord, err := h.app.FindRecordById("monitors", record.Id)
	if err == nil {
		record = updatedRecord
	}

	return e.JSON(http.StatusOK, recordToResponse(record))
}

// getStats returns uptime statistics for a monitor
func (h *APIHandler) getStats(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	if id == "" {
		return e.BadRequestError("Monitor ID is required", nil)
	}

	record, err := h.app.FindRecordById("monitors", id)
	if err != nil {
		return e.NotFoundError("Monitor not found", err)
	}

	// Verify ownership
	if record.GetString("user") != e.Auth.Id {
		return e.ForbiddenError("Access denied", nil)
	}

	stats24h, _ := h.scheduler.GetUptimeStats(id, 24)
	stats7d, _ := h.scheduler.GetUptimeStats(id, 168)
	stats30d, _ := h.scheduler.GetUptimeStats(id, 720)
	avg24h := 0.0
	if stats24h.Total > 0 {
		records, _ := h.app.FindRecordsByFilter(
			"monitor_heartbeats",
			"monitor = {:monitorId} && time >= {:since} && status = {:status}",
			"-time",
			0,
			0,
			dbx.Params{
				"monitorId": id,
				"since":     time.Now().Add(-24 * time.Hour).Format("2006-01-02 15:04:05"),
				"status":    string(monitor.StatusUp),
			},
		)
		if len(records) > 0 {
			totalPing := 0
			for _, record := range records {
				totalPing += record.GetInt("ping")
			}
			avg24h = float64(totalPing) / float64(len(records))
		}
	}

	return e.JSON(http.StatusOK, map[string]interface{}{
		"uptime_24h":         stats24h,
		"uptime_7d":          stats7d,
		"uptime_30d":         stats30d,
		"uptime_percent_24h": percent(stats24h),
		"uptime_percent_7d":  percent(stats7d),
		"uptime_percent_30d": percent(stats30d),
		"avg_ping_24h":       avg24h,
	})
}

// runPageSpeedCheck runs a PageSpeed Insights check for a monitor
func (h *APIHandler) runPageSpeedCheck(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	if id == "" {
		return e.BadRequestError("Monitor ID is required", nil)
	}

	record, err := h.app.FindRecordById("monitors", id)
	if err != nil {
		return e.NotFoundError("Monitor not found", err)
	}

	if record.GetString("user") != e.Auth.Id {
		return e.ForbiddenError("Access denied", nil)
	}

	url := record.GetString("url")
	if url == "" {
		return e.BadRequestError("Monitor does not have a URL", nil)
	}

	// Get strategy from query param, default to mobile
	strategy := e.Request.URL.Query().Get("strategy")
	if strategy == "" {
		strategy = "mobile"
	}
	if strategy != "mobile" && strategy != "desktop" {
		return e.BadRequestError("strategy must be 'mobile' or 'desktop'", nil)
	}

	apiKey, _ := utils.GetEnv("PAGESPEED_API_KEY")
	checker := pagespeed.NewChecker(apiKey)
	metrics, err := checker.CheckURL(url, strategy)
	if err != nil {
		return e.BadRequestError("PageSpeed check failed: "+err.Error(), err)
	}

	vitals := pagespeed.GetCoreWebVitalsStatus(metrics)

	return e.JSON(http.StatusOK, map[string]interface{}{
		"performance":   metrics.Performance,
		"accessibility": metrics.Accessibility,
		"bestPractices": metrics.BestPractices,
		"seo":           metrics.SEO,
		"pwa":           metrics.PWA,
		"fcp":           metrics.FCP,
		"lcp":           metrics.LCP,
		"ttfb":          metrics.TTFB,
		"cls":           metrics.CLS,
		"tbt":           metrics.TBT,
		"speedIndex":    metrics.SpeedIndex,
		"tti":           metrics.TTI,
		"strategy":      metrics.Strategy,
		"checkedAt":     metrics.CheckedAt,
		"url":           metrics.URL,
		"vitals":        vitals,
	})
}

// getHeartbeats returns recent heartbeats for a monitor
func (h *APIHandler) getHeartbeats(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	if id == "" {
		return e.BadRequestError("Monitor ID is required", nil)
	}

	record, err := h.app.FindRecordById("monitors", id)
	if err != nil {
		return e.NotFoundError("Monitor not found", err)
	}

	// Verify ownership
	if record.GetString("user") != e.Auth.Id {
		return e.ForbiddenError("Access denied", nil)
	}

	// Get limit from query, default 100
	limit := 100

	records, err := h.app.FindRecordsByFilter(
		"monitor_heartbeats",
		"monitor = {:monitorId}",
		"-time",
		limit,
		0,
		map[string]any{
			"monitorId": id,
		},
	)
	if err != nil {
		return e.InternalServerError("Failed to fetch heartbeats", err)
	}

	heartbeats := make([]map[string]interface{}, 0, len(records))
	for _, hb := range records {
		heartbeats = append(heartbeats, map[string]interface{}{
			"id":          hb.Id,
			"status":      hb.GetString("status"),
			"ping":        hb.GetInt("ping"),
			"msg":         hb.GetString("msg"),
			"cert_expiry": hb.GetInt("cert_expiry"),
			"cert_valid":  hb.GetBool("cert_valid"),
			"time":        hb.Get("time"),
		})
	}

	return e.JSON(http.StatusOK, map[string]interface{}{
		"heartbeats": heartbeats,
	})
}

// recordToResponse converts a PocketBase record to MonitorResponse
func recordToResponse(record *core.Record) MonitorResponse {
	resp := MonitorResponse{
		ID:                     record.Id,
		Name:                   record.GetString("name"),
		Type:                   record.GetString("type"),
		URL:                    record.GetString("url"),
		Hostname:               record.GetString("hostname"),
		Port:                   record.GetInt("port"),
		Method:                 record.GetString("method"),
		Interval:               record.GetInt("interval"),
		Timeout:                record.GetInt("timeout"),
		Retries:                record.GetInt("retries"),
		Status:                 record.GetString("status"),
		Active:                 record.GetBool("active"),
		Description:            record.GetString("description"),
		Keyword:                record.GetString("keyword"),
		JSONQuery:              record.GetString("json_query"),
		ExpectedValue:          record.GetString("expected_value"),
		InvertKeyword:          record.GetBool("invert_keyword"),
		DNSResolveServer:       record.GetString("dns_resolve_server"),
		DNSResolverMode:        record.GetString("dns_resolver_mode"),
		CertExpiryNotification: record.GetBool("cert_expiry_notification"),
		CertExpiryDays:         record.GetInt("cert_expiry_days"),
		NotifyOnDown:           record.GetBool("notify_on_down"),
		NotifyOnRecover:        record.GetBool("notify_on_recover"),
		NotifyOnResponseTime:   record.GetBool("notify_on_response_time"),
		ResponseTimeThreshold:  record.GetInt("response_time_threshold"),
		NotifyOnUptimeDrop:     record.GetBool("notify_on_uptime_drop"),
		UptimeThreshold:        record.GetFloat("uptime_threshold"),
		NotifyRepeatedFailures: record.GetBool("notify_repeated_failures"),
		ConsecutiveFailures:    record.GetInt("consecutive_failures"),
		QuietHoursEnabled:      record.GetBool("quiet_hours_enabled"),
		QuietHoursStart:        record.GetString("quiet_hours_start"),
		QuietHoursEnd:          record.GetString("quiet_hours_end"),

		IgnoreTLSError: record.GetBool("ignore_tls_error"),
		Created:        record.GetDateTime("created").Time(),
		Updated:        record.GetDateTime("updated").Time(),
	}

	if lc := record.GetDateTime("last_check"); !lc.IsZero() {
		t := lc.Time()
		resp.LastCheck = &t
	}

	if stats := record.Get("uptime_stats"); stats != nil {
		var s map[string]float64
		if raw, err := json.Marshal(stats); err == nil && json.Unmarshal(raw, &s) == nil {
			resp.UptimeStats = s
		}
	}

	// Handle tags
	if tags := record.Get("tags"); tags != nil {
		if t, ok := tags.([]string); ok {
			resp.Tags = t
		}
	}

	return resp
}

func percent(stats *monitor.UptimeStats) float64 {
	if stats == nil || stats.Total == 0 {
		return 0
	}
	return float64(stats.Up) / float64(stats.Total) * 100
}

// listMonitorNotifications returns the notification ids linked to a monitor
func (h *APIHandler) listMonitorNotifications(e *core.RequestEvent) error {
	record, err := h.ownedMonitor(e)
	if err != nil {
		return err
	}
	links, err := h.app.FindAllRecords("monitor_notifications",
		dbx.HashExp{"monitor": record.Id})
	if err != nil {
		return e.InternalServerError("Failed to fetch notification links", err)
	}
	ids := make([]string, 0, len(links))
	for _, l := range links {
		ids = append(ids, l.GetString("notification"))
	}
	return e.JSON(http.StatusOK, map[string]any{"notification_ids": ids})
}

// setMonitorNotifications replaces the monitor's linked notifications
func (h *APIHandler) setMonitorNotifications(e *core.RequestEvent) error {
	record, err := h.ownedMonitor(e)
	if err != nil {
		return err
	}
	var req struct {
		NotificationIDs []string `json:"notification_ids"`
	}
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Invalid request body", err)
	}

	collection, err := h.app.FindCollectionByNameOrId("monitor_notifications")
	if err != nil {
		return e.InternalServerError("monitor_notifications collection missing", err)
	}
	existing, err := h.app.FindAllRecords("monitor_notifications",
		dbx.HashExp{"monitor": record.Id})
	if err != nil {
		return e.InternalServerError("Failed to fetch notification links", err)
	}

	wanted := map[string]bool{}
	for _, id := range req.NotificationIDs {
		wanted[id] = true
	}
	for _, link := range existing {
		if !wanted[link.GetString("notification")] {
			if err := h.app.Delete(link); err != nil {
				return e.InternalServerError("Failed to remove notification link", err)
			}
		} else {
			delete(wanted, link.GetString("notification"))
		}
	}
	for id := range wanted {
		link := core.NewRecord(collection)
		link.Set("monitor", record.Id)
		link.Set("notification", id)
		link.Set("user", record.GetString("user"))
		if err := h.app.Save(link); err != nil {
			return e.InternalServerError("Failed to link notification", err)
		}
	}
	return e.JSON(http.StatusOK, map[string]any{"notification_ids": req.NotificationIDs})
}

// ownedMonitor fetches a monitor by path id and verifies ownership
func (h *APIHandler) ownedMonitor(e *core.RequestEvent) (*core.Record, error) {
	id := e.Request.PathValue("id")
	if id == "" {
		return nil, e.BadRequestError("Monitor ID is required", nil)
	}
	record, err := h.app.FindRecordById("monitors", id)
	if err != nil {
		return nil, e.NotFoundError("Monitor not found", err)
	}
	if record.GetString("user") != e.Auth.Id {
		return nil, e.ForbiddenError("Access denied", nil)
	}
	return record, nil
}
