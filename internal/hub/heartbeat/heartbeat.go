// Package heartbeat sends periodic outbound pings to an external monitoring
// endpoint (e.g. BetterStack, Uptime Kuma, Healthchecks.io) so operators can
// monitor Beszel without exposing it to the internet.
package heartbeat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/henrygd/beszel"
	"github.com/pocketbase/pocketbase/core"
)

// Default values for heartbeat configuration.
const (
	defaultInterval = 60 // seconds
	httpTimeout     = 10 * time.Second
)

// Payload is the JSON body sent with each heartbeat request.
type Payload struct {
	// Status is "ok" when all non-paused systems are up, "warn" when alerts
	// are triggered but no systems are down, and "error" when any system is down.
	Status    string          `json:"status"`
	Timestamp string          `json:"timestamp"`
	Msg       string          `json:"msg"`
	Systems   SystemsSummary  `json:"systems"`
	Down      []SystemInfo    `json:"down_systems,omitempty"`
	Alerts    []AlertInfo     `json:"triggered_alerts,omitempty"`
	Version   string          `json:"beszel_version"`
}

// SystemsSummary contains counts of systems by status.
type SystemsSummary struct {
	Total   int `json:"total"`
	Up      int `json:"up"`
	Down    int `json:"down"`
	Paused  int `json:"paused"`
	Pending int `json:"pending"`
}

// SystemInfo identifies a system that is currently down.
type SystemInfo struct {
	ID   string `json:"id" db:"id"`
	Name string `json:"name" db:"name"`
	Host string `json:"host" db:"host"`
}

// AlertInfo describes a currently triggered alert.
type AlertInfo struct {
	SystemID   string  `json:"system_id"`
	SystemName string  `json:"system_name"`
	AlertName  string  `json:"alert_name"`
	Threshold  float64 `json:"threshold"`
}

// Config holds heartbeat settings read from environment variables.
type Config struct {
	URL      string // endpoint to ping
	Interval int    // seconds between pings
	Method   string // HTTP method (GET or POST, default POST)
}

// Heartbeat manages the periodic outbound health check.
type Heartbeat struct {
	app    core.App
	config Config
	client *http.Client
}

// New creates a Heartbeat if configuration is present.
// Returns nil if HEARTBEAT_URL is not set (feature disabled).
func New(app core.App, getEnv func(string) (string, bool)) *Heartbeat {
	url, ok := getEnv("HEARTBEAT_URL")
	if !ok || url == "" {
		return nil
	}

	interval := defaultInterval
	if v, ok := getEnv("HEARTBEAT_INTERVAL"); ok {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			interval = parsed
		}
	}

	method := "POST"
	if v, ok := getEnv("HEARTBEAT_METHOD"); ok {
		v = strings.ToUpper(strings.TrimSpace(v))
		if v == "GET" || v == "HEAD" {
			method = v
		}
	}

	return &Heartbeat{
		app: app,
		config: Config{
			URL:      url,
			Interval: interval,
			Method:   method,
		},
		client: &http.Client{Timeout: httpTimeout},
	}
}

// Start begins the heartbeat loop. It blocks and should be called in a goroutine.
// The loop runs until the provided stop channel is closed.
func (hb *Heartbeat) Start(stop <-chan struct{}) {
	hb.app.Logger().Info("Heartbeat enabled",
		"url", hb.config.URL,
		"interval", fmt.Sprintf("%ds", hb.config.Interval),
		"method", hb.config.Method,
	)

	// Send an initial heartbeat immediately on startup.
	hb.send()

	ticker := time.NewTicker(time.Duration(hb.config.Interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			hb.send()
		}
	}
}

// Send performs a single heartbeat ping. Exposed for the test-heartbeat API endpoint.
func (hb *Heartbeat) Send() error {
	return hb.send()
}

// GetConfig returns the current heartbeat configuration.
func (hb *Heartbeat) GetConfig() Config {
	return hb.config
}

func (hb *Heartbeat) send() error {
	payload, err := hb.buildPayload()
	if err != nil {
		hb.app.Logger().Error("Heartbeat: failed to build payload", "err", err)
		return err
	}

	var req *http.Request

	if hb.config.Method == "GET" || hb.config.Method == "HEAD" {
		req, err = http.NewRequest(hb.config.Method, hb.config.URL, nil)
	} else {
		body, jsonErr := json.Marshal(payload)
		if jsonErr != nil {
			hb.app.Logger().Error("Heartbeat: failed to marshal payload", "err", jsonErr)
			return jsonErr
		}
		req, err = http.NewRequest("POST", hb.config.URL, bytes.NewReader(body))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
		}
	}

	if err != nil {
		hb.app.Logger().Error("Heartbeat: failed to create request", "err", err)
		return err
	}

	req.Header.Set("User-Agent", "Beszel-Heartbeat")

	resp, err := hb.client.Do(req)
	if err != nil {
		hb.app.Logger().Error("Heartbeat: request failed", "url", hb.config.URL, "err", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		hb.app.Logger().Warn("Heartbeat: non-success response",
			"url", hb.config.URL,
			"status", resp.StatusCode,
		)
		return fmt.Errorf("heartbeat endpoint returned status %d", resp.StatusCode)
	}

	return nil
}

func (hb *Heartbeat) buildPayload() (*Payload, error) {
	db := hb.app.DB()

	// Count systems by status.
	var systemCounts []struct {
		Status string `db:"status"`
		Count  int    `db:"cnt"`
	}
	err := db.NewQuery("SELECT status, COUNT(*) as cnt FROM systems GROUP BY status").All(&systemCounts)
	if err != nil {
		return nil, fmt.Errorf("query system counts: %w", err)
	}

	summary := SystemsSummary{}
	for _, sc := range systemCounts {
		switch sc.Status {
		case "up":
			summary.Up = sc.Count
		case "down":
			summary.Down = sc.Count
		case "paused":
			summary.Paused = sc.Count
		case "pending":
			summary.Pending = sc.Count
		}
		summary.Total += sc.Count
	}

	// Get names of down systems.
	var downSystems []SystemInfo
	err = db.NewQuery("SELECT id, name, host FROM systems WHERE status = 'down'").All(&downSystems)
	if err != nil {
		return nil, fmt.Errorf("query down systems: %w", err)
	}

	// Get triggered alerts with system names.
	var triggeredAlerts []struct {
		SystemID   string  `db:"system"`
		SystemName string  `db:"system_name"`
		AlertName  string  `db:"name"`
		Value      float64 `db:"value"`
	}
	err = db.NewQuery(`
		SELECT a.system, s.name as system_name, a.name, a.value
		FROM alerts a
		JOIN systems s ON a.system = s.id
		WHERE a.triggered = true
	`).All(&triggeredAlerts)
	if err != nil {
		// Non-fatal: alerts info is supplementary.
		triggeredAlerts = nil
	}

	alerts := make([]AlertInfo, 0, len(triggeredAlerts))
	for _, ta := range triggeredAlerts {
		alerts = append(alerts, AlertInfo{
			SystemID:   ta.SystemID,
			SystemName: ta.SystemName,
			AlertName:  ta.AlertName,
			Threshold:  ta.Value,
		})
	}

	// Determine overall status.
	status := "ok"
	msg := "All systems operational"
	if summary.Down > 0 {
		status = "error"
		names := make([]string, len(downSystems))
		for i, ds := range downSystems {
			names[i] = ds.Name
		}
		msg = fmt.Sprintf("%d system(s) down: %s", summary.Down, strings.Join(names, ", "))
	} else if len(alerts) > 0 {
		status = "warn"
		msg = fmt.Sprintf("%d alert(s) triggered", len(alerts))
	}

	return &Payload{
		Status:    status,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Msg:       msg,
		Systems:   summary,
		Down:      downSystems,
		Alerts:    alerts,
		Version:   beszel.Version,
	}, nil
}
