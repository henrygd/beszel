package alerts

import (
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type AlertTemplateRecord struct {
	ID              string `db:"id" json:"id"`
	User            string `db:"user" json:"user"`
	Name            string `db:"name" json:"name"`
	AlertType       string `db:"alert_type" json:"alert_type"`
	TitleTemplate   string `db:"title_template" json:"title_template"`
	MessageTemplate string `db:"message_template" json:"message_template"`
}

type TemplateData struct {
	SystemName      string
	AlertName       string
	AlertType       string
	ThresholdStatus string
	Status          string
	Emoji           string
	Value           string
	Unit            string
	Threshold       string
	Minutes         string
	MinutesLabel    string
	Descriptor      string
	Filesystem      string
}

type templateCacheEntry struct {
	template  *AlertTemplateRecord
	timestamp time.Time
}

type TemplateService struct {
	hub          core.App
	templateCache sync.Map // map[string]templateCacheEntry
	cacheTTL     time.Duration
}

func NewTemplateService(hub core.App) *TemplateService {
	return &TemplateService{
		hub:      hub,
		cacheTTL: 5 * time.Minute, // Cache templates for 5 minutes
	}
}

// GetTemplate retrieves the template for an alert type, or returns system default
func (ts *TemplateService) GetTemplate(userID, alertType string) (*AlertTemplateRecord, error) {
	cacheKey := userID + ":" + alertType

	// Check cache first
	if cached, ok := ts.templateCache.Load(cacheKey); ok {
		entry := cached.(templateCacheEntry)
		if time.Since(entry.timestamp) < ts.cacheTTL {
			return entry.template, nil
		}
		// Cache expired, remove it
		ts.templateCache.Delete(cacheKey)
	}

	// Try to find user's custom template for this alert type
	template := &AlertTemplateRecord{}
	err := ts.hub.DB().
		Select("*").
		From("alert_templates").
		Where(dbx.NewExp("user={:user} AND alert_type={:type}",
			dbx.Params{"user": userID, "type": alertType})).
		One(template)

	if err == nil {
		// Cache the template
		ts.templateCache.Store(cacheKey, templateCacheEntry{
			template:  template,
			timestamp: time.Now(),
		})
		return template, nil
	}

	// If no custom template found, return system default templates
	defaultTemplate := ts.getSystemDefaultTemplate(alertType)
	// Also cache the default to avoid repeated DB lookups
	ts.templateCache.Store(cacheKey, templateCacheEntry{
		template:  defaultTemplate,
		timestamp: time.Now(),
	})
	return defaultTemplate, nil
}

// InvalidateCache clears the template cache (call this when templates are updated)
func (ts *TemplateService) InvalidateCache() {
	ts.templateCache.Range(func(key, value interface{}) bool {
		ts.templateCache.Delete(key)
		return true
	})
}

// getSystemDefaultTemplate returns built-in templates
func (ts *TemplateService) getSystemDefaultTemplate(alertType string) *AlertTemplateRecord {
	switch alertType {
	case "Status":
		return &AlertTemplateRecord{
			AlertType:       "Status",
			TitleTemplate:   "Connection to {{systemName}} is {{status}} {{emoji}}",
			MessageTemplate: "Connection to {{systemName}} is {{status}}",
		}
	case "CPU":
		return &AlertTemplateRecord{
			AlertType:       "CPU",
			TitleTemplate:   "{{systemName}} {{alertName}} {{thresholdStatus}} threshold",
			MessageTemplate: "{{descriptor}} averaged {{value}}{{unit}} for the previous {{minutes}} {{minutesLabel}}.",
		}
	case "Memory":
		return &AlertTemplateRecord{
			AlertType:       "Memory",
			TitleTemplate:   "{{systemName}} {{alertName}} {{thresholdStatus}} threshold",
			MessageTemplate: "{{descriptor}} averaged {{value}}{{unit}} for the previous {{minutes}} {{minutesLabel}}.",
		}
	case "Disk":
		return &AlertTemplateRecord{
			AlertType:       "Disk",
			TitleTemplate:   "{{systemName}} {{alertName}} {{thresholdStatus}} threshold",
			MessageTemplate: "{{descriptor}} averaged {{value}}{{unit}} for the previous {{minutes}} {{minutesLabel}}.",
		}
	case "Temperature":
		return &AlertTemplateRecord{
			AlertType:       "Temperature",
			TitleTemplate:   "{{systemName}} {{alertName}} {{thresholdStatus}} threshold",
			MessageTemplate: "{{descriptor}} averaged {{value}}{{unit}} for the previous {{minutes}} {{minutesLabel}}.",
		}
	case "Bandwidth", "BandwidthUp", "BandwidthDown":
		return &AlertTemplateRecord{
			AlertType:       alertType,
			TitleTemplate:   "{{systemName}} {{alertName}} {{thresholdStatus}} threshold",
			MessageTemplate: "{{descriptor}} averaged {{value}}{{unit}} for the previous {{minutes}} {{minutesLabel}}.",
		}
	case "LoadAvg1", "LoadAvg5", "LoadAvg15":
		return &AlertTemplateRecord{
			AlertType:       alertType,
			TitleTemplate:   "{{systemName}} {{alertName}} {{thresholdStatus}} threshold",
			MessageTemplate: "{{descriptor}} averaged {{value}}{{unit}} for the previous {{minutes}} {{minutesLabel}}.",
		}
	case "Swap":
		return &AlertTemplateRecord{
			AlertType:       "Swap",
			TitleTemplate:   "{{systemName}} {{alertName}} {{thresholdStatus}} threshold",
			MessageTemplate: "{{descriptor}} averaged {{value}}{{unit}} for the previous {{minutes}} {{minutesLabel}}.",
		}
	default:
		// Fallback generic template
		return &AlertTemplateRecord{
			AlertType:       alertType,
			TitleTemplate:   "{{systemName}} {{alertName}} {{thresholdStatus}} threshold",
			MessageTemplate: "{{descriptor}} averaged {{value}}{{unit}} for the previous {{minutes}} {{minutesLabel}}.",
		}
	}
}

// RenderTemplate processes template with variables
func (ts *TemplateService) RenderTemplate(template *AlertTemplateRecord, data TemplateData) (title, message string) {
	title = ts.replaceTemplateVariables(template.TitleTemplate, data)
	message = ts.replaceTemplateVariables(template.MessageTemplate, data)
	return title, message
}

// replaceTemplateVariables replaces {{variable}} placeholders with actual values
func (ts *TemplateService) replaceTemplateVariables(template string, data TemplateData) string {
	// Use strings.Builder for efficient string building
	var result strings.Builder
	result.Grow(len(template) + 100) // Pre-allocate with some extra space for replacements

	// Create replacer with all variables
	replacer := strings.NewReplacer(
		"{{systemName}}", data.SystemName,
		"{{alertName}}", data.AlertName,
		"{{alertType}}", data.AlertType,
		"{{thresholdStatus}}", data.ThresholdStatus,
		"{{status}}", data.Status,
		"{{emoji}}", data.Emoji,
		"{{value}}", data.Value,
		"{{unit}}", data.Unit,
		"{{threshold}}", data.Threshold,
		"{{minutes}}", data.Minutes,
		"{{minutesLabel}}", data.MinutesLabel,
		"{{descriptor}}", data.Descriptor,
		"{{filesystem}}", data.Filesystem,
	)

	// Write the replaced string to builder
	replacer.WriteString(&result, template)
	return result.String()
}

// FormatAlertName formats the alert name for display
func FormatAlertName(alertType, filesystem string) string {
	name := alertType
	
	// Apply formatting rules
	switch alertType {
	case "Disk":
		name = "disk usage"
		if filesystem != "" {
			name += " (" + filesystem + ")"
		}
	case "LoadAvg1":
		name = "1m Load"
	case "LoadAvg5": 
		name = "5m Load"
	case "LoadAvg15":
		name = "15m Load"
	case "BandwidthUp":
		name = "Upload bandwidth"
	case "BandwidthDown":
		name = "Download bandwidth"
	default:
		// Keep original name, but make lowercase if not CPU
		if name != "CPU" {
			name = strings.ToLower(name)
		}
	}
	
	return name
}