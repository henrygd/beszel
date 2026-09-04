// Package monitors implements external uptime monitoring (PR1: hub-only).
// It provides HTTP/keyword, TLS certificate, DNS and ping checkers executed
// from the hub, with PocketBase persistence, REST API and alert transitions.
package monitors

import (
	"errors"
	"fmt"
)

// MonitorType identifies the kind of check a monitor performs.
type MonitorType string

// Supported monitor types.
const (
	TypeHTTP    MonitorType = "http"
	TypeKeyword MonitorType = "keyword"
	TypePing    MonitorType = "ping"
	TypeDNS     MonitorType = "dns"
	TypeTLS     MonitorType = "tls"
)

// Check result statuses.
const (
	StatusUp   = "up"
	StatusDown = "down"
	StatusWarn = "warn"
)

// Validation bounds shared by the API and config sync.
const (
	MinIntervalSeconds = 20
	MaxIntervalSeconds = 86400
	MaxRetries         = 10
)

// Monitor describes a single uptime check configuration.
type Monitor struct {
	Name            string
	Type            MonitorType
	Target          string
	IntervalSeconds int
	TimeoutSeconds  int
	MaxRetries      int
	UpsideDown      bool
	Config          map[string]any
}

// CheckResult is the outcome of a single check attempt.
type CheckResult struct {
	Status    string
	LatencyMs float64
	Code      *int
	Message   string
	Details   map[string]any
	CertDays  *float64
}

// Validate checks monitor configuration bounds.
func (m Monitor) Validate() error {
	switch m.Type {
	case TypeHTTP, TypeKeyword, TypePing, TypeDNS, TypeTLS:
	default:
		return fmt.Errorf("unknown monitor type %q", m.Type)
	}
	if m.Target == "" {
		return errors.New("target is required")
	}
	if m.IntervalSeconds < MinIntervalSeconds || m.IntervalSeconds > MaxIntervalSeconds {
		return fmt.Errorf("interval must be %d..%d seconds", MinIntervalSeconds, MaxIntervalSeconds)
	}
	if m.TimeoutSeconds <= 0 || m.TimeoutSeconds >= m.IntervalSeconds {
		return errors.New("timeout must be > 0 and < interval")
	}
	if m.MaxRetries < 0 || m.MaxRetries > MaxRetries {
		return fmt.Errorf("max_retries must be 0..%d", MaxRetries)
	}
	return nil
}
