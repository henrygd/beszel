package alerts

import (
	"strings"

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

type TemplateService struct {
	hub core.App
}

func NewTemplateService(hub core.App) *TemplateService {
	return &TemplateService{hub: hub}
}

// GetTemplate retrieves the template for an alert type, or returns system default
func (ts *TemplateService) GetTemplate(userID, alertType string) (*AlertTemplateRecord, error) {
	// Try to find user's custom template for this alert type
	template := &AlertTemplateRecord{}
	err := ts.hub.DB().
		Select("*").
		From("alert_templates").
		Where(dbx.NewExp("user={:user} AND alert_type={:type}", 
			dbx.Params{"user": userID, "type": alertType})).
		One(template)
	
	if err == nil {
		return template, nil
	}
	
	// If no custom template found, return system default templates
	return ts.getSystemDefaultTemplate(alertType), nil
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
	result := template
	
	// Replace all variables
	result = strings.ReplaceAll(result, "{{systemName}}", data.SystemName)
	result = strings.ReplaceAll(result, "{{alertName}}", data.AlertName)
	result = strings.ReplaceAll(result, "{{alertType}}", data.AlertType)
	result = strings.ReplaceAll(result, "{{thresholdStatus}}", data.ThresholdStatus)
	result = strings.ReplaceAll(result, "{{status}}", data.Status)
	result = strings.ReplaceAll(result, "{{emoji}}", data.Emoji)
	result = strings.ReplaceAll(result, "{{value}}", data.Value)
	result = strings.ReplaceAll(result, "{{unit}}", data.Unit)
	result = strings.ReplaceAll(result, "{{threshold}}", data.Threshold)
	result = strings.ReplaceAll(result, "{{minutes}}", data.Minutes)
	result = strings.ReplaceAll(result, "{{minutesLabel}}", data.MinutesLabel)
	result = strings.ReplaceAll(result, "{{descriptor}}", data.Descriptor)
	result = strings.ReplaceAll(result, "{{filesystem}}", data.Filesystem)
	
	return result
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