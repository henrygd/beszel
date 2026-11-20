package statuspage

import "time"

// StatusPage represents a public status page configuration
type StatusPage struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Slug        string    `json:"slug" db:"slug"`
	Title       string    `json:"title" db:"title"`
	Description string    `json:"description" db:"description"`
	Logo        string    `json:"logo" db:"logo"`
	Favicon     string    `json:"favicon" db:"favicon"`
	Theme       string    `json:"theme" db:"theme"` // light, dark, auto
	CustomCSS   string    `json:"custom_css" db:"custom_css"`
	Public      bool      `json:"public" db:"public"`
	ShowUptime  bool      `json:"show_uptime" db:"show_uptime"`
	UserID      string    `json:"user" db:"user"`
	Created     time.Time `json:"created" db:"created"`
	Updated     time.Time `json:"updated" db:"updated"`
}

// StatusPageMonitor links monitors to a status page
type StatusPageMonitor struct {
	ID           string `json:"id" db:"id"`
	StatusPageID string `json:"status_page" db:"status_page"`
	MonitorID    string `json:"monitor" db:"monitor"`
	DisplayName  string `json:"display_name" db:"display_name"`
	Group        string `json:"group" db:"group"`
	SortOrder    int    `json:"sort_order" db:"sort_order"`
	UserID       string `json:"user" db:"user"`
}

// PublicIncident represents an incident for public display
type PublicIncident struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Severity    string    `json:"severity"`
	StartedAt   time.Time `json:"started_at"`
	ResolvedAt  time.Time `json:"resolved_at,omitempty"`
}

// PublicStatusPage represents a status page for public viewing
type PublicStatusPage struct {
	ID            string                `json:"id"`
	Name          string                `json:"name"`
	Title         string                `json:"title"`
	Description   string                `json:"description"`
	Logo          string                `json:"logo"`
	Favicon       string                `json:"favicon"`
	Theme         string                `json:"theme"`
	CustomCSS     string                `json:"custom_css,omitempty"`
	Monitors      []PublicMonitorStatus `json:"monitors"`
	Incidents     []PublicIncident      `json:"incidents"`
	OverallStatus string                `json:"overall_status"`
	UpdatedAt     time.Time             `json:"updated_at"`
}

// PublicMonitorStatus represents a monitor's status for public display
type PublicMonitorStatus struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	DisplayName string            `json:"display_name"`
	Group       string            `json:"group"`
	Status      string            `json:"status"`
	LastError   string            `json:"last_error,omitempty"`
	Uptime24h   float64           `json:"uptime_24h"`
	Uptime7d    float64           `json:"uptime_7d"`
	Uptime30d   float64           `json:"uptime_30d"`
	LastCheck   time.Time         `json:"last_check"`
	Heartbeats  []PublicHeartbeat `json:"heartbeats,omitempty"`
}

// PublicHeartbeat is a recent check result for status-page bars
type PublicHeartbeat struct {
	Status string    `json:"status"`
	Ping   int       `json:"ping"`
	Msg    string    `json:"msg,omitempty"`
	Time   time.Time `json:"time"`
}

// CreateStatusPageRequest represents a status page creation request
type CreateStatusPageRequest struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Logo        string `json:"logo,omitempty"`
	Favicon     string `json:"favicon,omitempty"`
	Theme       string `json:"theme,omitempty"`
	CustomCSS   string `json:"custom_css,omitempty"`
	Public      bool   `json:"public"`
	ShowUptime  bool   `json:"show_uptime,omitempty"`
}

// UpdateStatusPageRequest represents a status page update request
type UpdateStatusPageRequest struct {
	Name        *string `json:"name,omitempty"`
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	Logo        *string `json:"logo,omitempty"`
	Favicon     *string `json:"favicon,omitempty"`
	Theme       *string `json:"theme,omitempty"`
	CustomCSS   *string `json:"custom_css,omitempty"`
	Public      *bool   `json:"public,omitempty"`
	ShowUptime  *bool   `json:"show_uptime,omitempty"`
}

// StatusPageMonitorRequest represents adding a monitor to a status page
type StatusPageMonitorRequest struct {
	MonitorID   string `json:"monitor"`
	DisplayName string `json:"display_name,omitempty"`
	Group       string `json:"group,omitempty"`
	SortOrder   int    `json:"sort_order,omitempty"`
}

// StatusPageResponse represents a status page response
type StatusPageResponse struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Logo         string `json:"logo"`
	Favicon      string `json:"favicon"`
	Theme        string `json:"theme"`
	Public       bool   `json:"public"`
	ShowUptime   bool   `json:"show_uptime"`
	MonitorCount int    `json:"monitor_count"`
	Created      string `json:"created"`
	Updated      string `json:"updated"`
}

// Overall status constants
const (
	StatusOperational = "operational"
	StatusDegraded    = "degraded"
	StatusPartial     = "partial_outage"
	StatusMajor       = "major_outage"
)

// Theme constants
const (
	ThemeLight = "light"
	ThemeDark  = "dark"
	ThemeAuto  = "auto"
)

// ValidateTheme validates and returns a theme
func ValidateTheme(theme string) string {
	switch theme {
	case ThemeLight, ThemeDark, ThemeAuto:
		return theme
	default:
		return ThemeAuto
	}
}
