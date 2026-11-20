package bulk

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/apis"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/henrygd/beszel/internal/entities/domain"
	"github.com/henrygd/beszel/internal/entities/monitor"
)

// APIHandler handles bulk import/export requests
type APIHandler struct {
	app core.App
}

// NewAPIHandler creates a new bulk API handler
func NewAPIHandler(app core.App) *APIHandler {
	return &APIHandler{app: app}
}

// RegisterRoutes registers bulk API routes
func (h *APIHandler) RegisterRoutes(se *core.ServeEvent) {
	api := se.Router.Group("/api/beszel/bulk")
	api.Bind(apis.RequireAuth())

	// Import endpoints
	api.POST("/import/domains", h.importDomains)
	api.POST("/import/monitors", h.importMonitors)

	// Export endpoints
	api.GET("/export/domains", h.exportDomains)
	api.GET("/export/monitors", h.exportMonitors)
}

// ImportResult represents the result of an import operation
type ImportResult struct {
	Success int      `json:"success"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors,omitempty"`
}

// importDomains handles bulk domain import from CSV
func (h *APIHandler) importDomains(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	// Parse multipart form
	if err := e.Request.ParseMultipartForm(10 << 20); err != nil { // 10 MB max
		return e.BadRequestError("failed to parse form", err)
	}

	file, _, err := e.Request.FormFile("file")
	if err != nil {
		return e.BadRequestError("missing file", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	// Skip header
	_, _ = reader.Read()

	collection, err := h.app.FindCollectionByNameOrId("domains")
	if err != nil {
		return e.InternalServerError("failed to find collection", err)
	}

	result := ImportResult{}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("Read error: %v", err))
			continue
		}

		if len(record) < 1 || record[0] == "" {
			result.Failed++
			result.Errors = append(result.Errors, "Empty domain name")
			continue
		}

		domainName := strings.TrimSpace(record[0])

		// Check if domain already exists
		existing, _ := h.app.FindFirstRecordByFilter("domains", "domain_name = {:domain} && user = {:user}",
			dbx.Params{"domain": domainName, "user": authRecord.Id})
		if existing != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("Domain already exists: %s", domainName))
			continue
		}

		// Create domain record
		newRecord := core.NewRecord(collection)
		newRecord.Set("domain_name", domainName)
		newRecord.Set("user", authRecord.Id)
		newRecord.Set("status", domain.DomainStatusUnknown)

		// Optional fields
		if len(record) > 1 && record[1] != "" {
			newRecord.Set("tags", strings.Split(record[1], ","))
		}
		if len(record) > 2 && record[2] != "" {
			newRecord.Set("notes", record[2])
		}
		if len(record) > 3 && record[3] != "" {
			newRecord.Set("auto_renew", record[3] == "true" || record[3] == "yes")
		}

		if err := h.app.Save(newRecord); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("Failed to save %s: %v", domainName, err))
		} else {
			result.Success++
		}
	}

	return e.JSON(http.StatusOK, result)
}

// importMonitors handles bulk monitor import from CSV
func (h *APIHandler) importMonitors(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	if err := e.Request.ParseMultipartForm(10 << 20); err != nil {
		return e.BadRequestError("failed to parse form", err)
	}

	file, _, err := e.Request.FormFile("file")
	if err != nil {
		return e.BadRequestError("missing file", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	// Skip header
	_, _ = reader.Read()

	collection, err := h.app.FindCollectionByNameOrId("monitors")
	if err != nil {
		return e.InternalServerError("failed to find collection", err)
	}

	result := ImportResult{}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("Read error: %v", err))
			continue
		}

		if len(record) < 2 || record[0] == "" || record[1] == "" {
			result.Failed++
			result.Errors = append(result.Errors, "Missing name or URL")
			continue
		}

		name := strings.TrimSpace(record[0])
		url := strings.TrimSpace(record[1])

		// Check if monitor already exists
		existing, _ := h.app.FindFirstRecordByFilter("monitors", "name = {:name} && user = {:user}",
			dbx.Params{"name": name, "user": authRecord.Id})
		if existing != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("Monitor already exists: %s", name))
			continue
		}

		// Create monitor record
		newRecord := core.NewRecord(collection)
		newRecord.Set("name", name)
		newRecord.Set("url", url)
		newRecord.Set("user", authRecord.Id)
		newRecord.Set("status", "unknown")
		newRecord.Set("type", monitor.TypeHTTP)
		newRecord.Set("interval", 60)
		newRecord.Set("retries", 3)

		// Optional fields
		if len(record) > 2 && record[2] != "" {
			newRecord.Set("type", record[2])
		}
		if len(record) > 3 && record[3] != "" {
			if interval, err := parseInt(record[3]); err == nil {
				newRecord.Set("interval", interval)
			}
		}
		if len(record) > 4 && record[4] != "" {
			if retries, err := parseInt(record[4]); err == nil {
				newRecord.Set("retries", retries)
			}
		}

		if err := h.app.Save(newRecord); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("Failed to save %s: %v", name, err))
		} else {
			result.Success++
		}
	}

	return e.JSON(http.StatusOK, result)
}

// exportDomains exports domains as JSON
func (h *APIHandler) exportDomains(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	records, err := h.app.FindAllRecords("domains",
		dbx.NewExp("user = {:user}", dbx.Params{"user": authRecord.Id}),
	)
	if err != nil {
		return e.InternalServerError("failed to fetch domains", err)
	}

	domains := make([]map[string]interface{}, 0, len(records))
	for _, record := range records {
		domains = append(domains, map[string]interface{}{
			"domain_name":    record.GetString("domain_name"),
			"status":         record.GetString("status"),
			"expiry_date":    record.GetDateTime("expiry_date").Time(),
			"registrar_name": record.GetString("registrar_name"),
			"ssl_issuer":     record.GetString("ssl_issuer"),
			"ssl_valid_to":   record.GetDateTime("ssl_valid_to").Time(),
			"ipv4_addresses": record.Get("ipv4_addresses"),
			"ipv6_addresses": record.Get("ipv6_addresses"),
			"name_servers":   record.Get("name_servers"),
			"mx_records":     record.Get("mx_records"),
			"tags":           record.Get("tags"),
			"auto_renew":     record.GetBool("auto_renew"),
			"notes":          record.GetString("notes"),
			"created":        record.GetDateTime("created").Time(),
			"updated":        record.GetDateTime("updated").Time(),
		})
	}

	e.Response.Header().Set("Content-Type", "application/json")
	e.Response.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=domains_export_%s.json", time.Now().Format("2006-01-02")))
	return json.NewEncoder(e.Response).Encode(domains)
}

// exportMonitors exports monitors as JSON
func (h *APIHandler) exportMonitors(e *core.RequestEvent) error {
	authRecord := e.Auth
	if authRecord == nil {
		return e.UnauthorizedError("unauthorized", nil)
	}

	records, err := h.app.FindAllRecords("monitors",
		dbx.NewExp("user = {:user}", dbx.Params{"user": authRecord.Id}),
	)
	if err != nil {
		return e.InternalServerError("failed to fetch monitors", err)
	}

	monitors := make([]map[string]interface{}, 0, len(records))
	for _, record := range records {
		monitors = append(monitors, map[string]interface{}{
			"name":               record.GetString("name"),
			"url":                record.GetString("url"),
			"type":               record.GetString("type"),
			"status":             record.GetString("status"),
			"interval":           record.GetInt("interval"),
			"retries":            record.GetInt("retries"),
			"last_response_time": record.GetInt("last_response_time"),
			"uptime_stats":       record.GetString("uptime_stats"),
			"tags":               record.Get("tags"),
			"created":            record.GetDateTime("created").Time(),
			"updated":            record.GetDateTime("updated").Time(),
		})
	}

	e.Response.Header().Set("Content-Type", "application/json")
	e.Response.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=monitors_export_%s.json", time.Now().Format("2006-01-02")))
	return json.NewEncoder(e.Response).Encode(monitors)
}

func parseInt(s string) (int, error) {
	var result int
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}
