package incident

import "time"

// Incident represents a monitoring incident
type Incident struct {
	ID          string `json:"id" db:"id"`
	Title       string `json:"title" db:"title"`
	Description string `json:"description" db:"description"`
	Type        string `json:"type" db:"type"`         // monitor_down, domain_expiring, ssl_expiring, etc.
	Severity    string `json:"severity" db:"severity"` // critical, high, medium, low
	Status      string `json:"status" db:"status"`     // open, acknowledged, resolved, closed

	// Related entities
	MonitorID *string `json:"monitor,omitempty" db:"monitor"`
	DomainID  *string `json:"domain,omitempty" db:"domain"`
	SystemID  *string `json:"system,omitempty" db:"system"`

	// Assignment
	AssignedTo *string `json:"assigned_to,omitempty" db:"assigned_to"`

	// Timestamps
	StartedAt      time.Time  `json:"started_at" db:"started_at"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty" db:"acknowledged_at"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty" db:"resolved_at"`
	ClosedAt       *time.Time `json:"closed_at,omitempty" db:"closed_at"`

	// Resolution
	Resolution string `json:"resolution,omitempty" db:"resolution"`
	RootCause  string `json:"root_cause,omitempty" db:"root_cause"`

	// Metadata
	UserID  string    `json:"user" db:"user"`
	Created time.Time `json:"created" db:"created"`
	Updated time.Time `json:"updated" db:"updated"`
}

// IncidentUpdate represents an update/note added to an incident
type IncidentUpdate struct {
	ID         string    `json:"id" db:"id"`
	IncidentID string    `json:"incident" db:"incident"`
	Message    string    `json:"message" db:"message"`
	UpdateType string    `json:"update_type" db:"update_type"` // note, status_change, assignment
	OldStatus  *string   `json:"old_status,omitempty" db:"old_status"`
	NewStatus  *string   `json:"new_status,omitempty" db:"new_status"`
	CreatedBy  string    `json:"created_by" db:"created_by"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
}

// IncidentMetrics represents incident statistics
type IncidentMetrics struct {
	TotalIncidents        int     `json:"total_incidents"`
	OpenIncidents         int     `json:"open_incidents"`
	AcknowledgedIncidents int     `json:"acknowledged_incidents"`
	ResolvedIncidents     int     `json:"resolved_incidents"`
	AvgResolutionTime     string  `json:"avg_resolution_time"`
	MTTR                  float64 `json:"mttr_hours"` // Mean Time To Resolve
}

// Constants
const (
	// Incident Types
	TypeMonitorDown    = "monitor_down"
	TypeMonitorUp      = "monitor_up"
	TypeDomainExpiring = "domain_expiring"
	TypeDomainExpired  = "domain_expired"
	TypeSSLExpiring    = "ssl_expiring"
	TypeSystemOffline  = "system_offline"
	TypeSystemOnline   = "system_online"

	// Severity Levels
	SeverityCritical = "critical"
	SeverityHigh     = "high"
	SeverityMedium   = "medium"
	SeverityLow      = "low"

	// Status
	StatusOpen         = "open"
	StatusAcknowledged = "acknowledged"
	StatusResolved     = "resolved"
	StatusClosed       = "closed"
)

// IsOpen returns true if incident is not resolved/closed
func (i *Incident) IsOpen() bool {
	return i.Status == StatusOpen || i.Status == StatusAcknowledged
}

// Duration returns how long the incident has been open
func (i *Incident) Duration() time.Duration {
	if i.ResolvedAt != nil {
		return i.ResolvedAt.Sub(i.StartedAt)
	}
	return time.Since(i.StartedAt)
}

// GetSeverityColor returns CSS color class for severity
func GetSeverityColor(severity string) string {
	switch severity {
	case SeverityCritical:
		return "bg-red-600"
	case SeverityHigh:
		return "bg-orange-500"
	case SeverityMedium:
		return "bg-yellow-500"
	case SeverityLow:
		return "bg-blue-500"
	default:
		return "bg-gray-500"
	}
}

// GetStatusColor returns CSS color class for status
func GetStatusColor(status string) string {
	switch status {
	case StatusOpen:
		return "bg-red-500"
	case StatusAcknowledged:
		return "bg-yellow-500"
	case StatusResolved:
		return "bg-green-500"
	case StatusClosed:
		return "bg-gray-500"
	default:
		return "bg-gray-500"
	}
}
