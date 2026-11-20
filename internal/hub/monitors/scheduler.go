package monitors

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/store"
	"github.com/henrygd/beszel/internal/entities/incident"
	"github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/henrygd/beszel/internal/hub/monitors/checks"
	"github.com/henrygd/beszel/internal/hub/notifications"
)

// AlertCallback is a function that sends alerts
type AlertCallback func(userID, title, message, link, linkText string)

// Scheduler manages the periodic execution of monitor checks
type Scheduler struct {
	app           core.App
	registry      *checks.CheckerRegistry
	monitors      *store.Store[string, *ScheduledMonitor]
	ticker        *time.Ticker
	stopChan      chan struct{}
	wg            sync.WaitGroup
	mu            sync.RWMutex
	running       bool
	alertCallback AlertCallback
	dispatcher    *notifications.Dispatcher
}

// ScheduledMonitor wraps a monitor with scheduling info
type ScheduledMonitor struct {
	Monitor   *monitor.Monitor
	NextCheck time.Time
	mu        sync.Mutex
}

// NewScheduler creates a new monitor scheduler
func NewScheduler(app core.App) *Scheduler {
	return &Scheduler{
		app:        app,
		registry:   checks.NewCheckerRegistry(),
		monitors:   store.New(map[string]*ScheduledMonitor{}),
		dispatcher: notifications.NewDispatcher(app),
		stopChan:   make(chan struct{}),
	}
}

// SetAlertCallback sets the callback function for sending alerts
func (s *Scheduler) SetAlertCallback(callback AlertCallback) {
	s.alertCallback = callback
}

// Start begins the scheduler loop
func (s *Scheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	// Load active monitors from database
	if err := s.loadMonitors(); err != nil {
		return fmt.Errorf("failed to load monitors: %w", err)
	}

	// Start the ticker (minimum 20 second resolution)
	s.ticker = time.NewTicker(20 * time.Second)
	s.running = true

	s.wg.Add(1)
	go s.run()

	log.Println("[monitor-scheduler] Started")
	return nil
}

// Stop halts the scheduler
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.ticker.Stop()
	close(s.stopChan)
	s.mu.Unlock()

	s.wg.Wait()
	log.Println("[monitor-scheduler] Stopped")
}

// run is the main scheduler loop
func (s *Scheduler) run() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ticker.C:
			s.checkMonitors()
		case <-s.stopChan:
			return
		}
	}
}

// checkMonitors checks all due monitors
func (s *Scheduler) checkMonitors() {
	now := time.Now()

	allMonitors := s.monitors.GetAll()
	for _, sm := range allMonitors {
		sm.mu.Lock()

		// Skip if monitor is paused or not active
		if !sm.Monitor.Active || sm.Monitor.Status == monitor.StatusPaused {
			sm.mu.Unlock()
			continue
		}

		// Check if it's time to run
		if now.Before(sm.NextCheck) {
			sm.mu.Unlock()
			continue
		}

		// Schedule the next check
		interval := time.Duration(sm.Monitor.Interval) * time.Second
		if interval < 20*time.Second {
			interval = 20 * time.Second
		}
		sm.NextCheck = now.Add(interval)
		sm.mu.Unlock()

		// Run check in background
		s.wg.Add(1)
		go func(m *monitor.Monitor) {
			defer s.wg.Done()
			s.runCheck(m)
		}(sm.Monitor)
	}
}

// runCheck executes a single monitor check
func (s *Scheduler) runCheck(m *monitor.Monitor) {
	// Get the appropriate checker
	checker, ok := s.registry.Get(m.Type)
	if !ok {
		log.Printf("[monitor-scheduler] No checker found for type: %s", m.Type)
		return
	}

	// Create context with timeout
	timeout := time.Duration(m.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Execute check
	result := checker.Check(ctx, m)

	// Handle retries
	if result.Status == monitor.StatusDown && m.Retries > 0 {
		retryInterval := time.Duration(m.RetryInterval) * time.Second
		if retryInterval == 0 {
			retryInterval = time.Second
		}

		for i := 0; i < m.Retries && result.Status == monitor.StatusDown; i++ {
			time.Sleep(retryInterval)

			ctx, cancel = context.WithTimeout(context.Background(), timeout)
			result = checker.Check(ctx, m)
			cancel()
		}
	}

	// Save heartbeat and update monitor status
	if err := s.saveResult(m, result); err != nil {
		log.Printf("[monitor-scheduler] Failed to save result: %v", err)
	}

	// Log result
	if result.Status == monitor.StatusUp {
		log.Printf("[monitor-scheduler] Check UP: %s (ping: %dms)", m.Name, result.Ping)
	} else {
		log.Printf("[monitor-scheduler] Check DOWN: %s - %s", m.Name, result.Msg)
	}
}

// saveResult saves the check result to the database and sends notifications on status change
func (s *Scheduler) saveResult(m *monitor.Monitor, result *monitor.CheckResult) error {
	record, err := s.app.FindRecordById("monitors", m.ID)
	if err != nil {
		return fmt.Errorf("failed to find monitor: %w", err)
	}

	prevStatus := monitor.Status(record.GetString("status"))
	newStatus := result.Status
	now := time.Now()

	hbCollection, err := s.app.FindCollectionByNameOrId("monitor_heartbeats")
	if err != nil {
		return fmt.Errorf("failed to find heartbeats collection: %w", err)
	}

	hbRecord := core.NewRecord(hbCollection)
	hbRecord.Set("monitor", m.ID)
	hbRecord.Set("status", string(result.Status))
	hbRecord.Set("ping", result.Ping)
	hbRecord.Set("msg", result.Msg)
	hbRecord.Set("cert_expiry", result.CertExpiry)
	hbRecord.Set("cert_valid", result.CertValid)
	hbRecord.Set("time", now)

	if err := s.app.Save(hbRecord); err != nil {
		return fmt.Errorf("failed to save heartbeat: %w", err)
	}

	stats, err := s.calculateUptimeStats(m.ID)
	if err != nil {
		return fmt.Errorf("failed to calculate uptime stats: %w", err)
	}
	stats["last_ping"] = float64(result.Ping)

	record.Set("status", string(newStatus))
	record.Set("last_check", now)
	record.Set("uptime_stats", stats)

	if err := s.app.Save(record); err != nil {
		return fmt.Errorf("failed to update monitor: %w", err)
	}

	m.Status = newStatus
	m.LastCheck = now
	m.UptimeStats = stats

	if prevStatus != newStatus {
		s.handleStatusChange(m, record, prevStatus, newStatus, result)
	}

	// Per-check performance thresholds (response time, uptime drop)
	if s.dispatcher != nil {
		hb := &monitor.Heartbeat{
			MonitorID:  m.ID,
			Status:     newStatus,
			Ping:       result.Ping,
			Msg:        result.Msg,
			CertExpiry: result.CertExpiry,
			CertValid:  result.CertValid,
			Time:       now,
		}
		s.dispatcher.EvaluateThresholds(m, hb, stats["uptime_24h"])
	}

	return nil
}

// handleStatusChange sends notifications when monitor status changes
func (s *Scheduler) handleStatusChange(m *monitor.Monitor, record *core.Record, prevStatus, newStatus monitor.Status, result *monitor.CheckResult) {
	userID := record.GetString("user")
	if userID == "" {
		return
	}

	var title, message string
	isRecovery := false

	switch {
	case newStatus == monitor.StatusDown && prevStatus != monitor.StatusDown:
		title = fmt.Sprintf("Monitor Down: %s", m.Name)
		message = fmt.Sprintf("The monitor %s (%s) is now DOWN.\n\nError: %s", m.Name, m.URL, result.Msg)
	case prevStatus == monitor.StatusDown && newStatus == monitor.StatusUp:
		title = fmt.Sprintf("Monitor Recovered: %s", m.Name)
		message = fmt.Sprintf("The monitor %s (%s) is now UP.\n\nResponse time: %dms", m.Name, m.URL, result.Ping)
		isRecovery = true
	default:
		return
	}

	s.createIncident(m, prevStatus, newStatus, result, isRecovery)

	// Notify per-monitor providers (Discord, Slack, Telegram, email, etc.)
	if s.dispatcher != nil {
		s.dispatcher.SendNotification(m, &monitor.Heartbeat{
			MonitorID:  m.ID,
			Status:     newStatus,
			Ping:       result.Ping,
			Msg:        result.Msg,
			CertExpiry: result.CertExpiry,
			CertValid:  result.CertValid,
			Time:       time.Now(),
		}, isRecovery)
	}

	// Send notification via AlertManager if available
	if s.alertCallback != nil {
		link := fmt.Sprintf("/monitor/%s", m.ID)
		linkText := "View Monitor"
		s.alertCallback(userID, title, message, link, linkText)
	}

	log.Printf("[monitor-scheduler] Status change: %s -> %s for %s", prevStatus, newStatus, m.Name)
}

// createIncident creates an incident record for the status change
func (s *Scheduler) createIncident(m *monitor.Monitor, prevStatus, newStatus monitor.Status, result *monitor.CheckResult, isRecovery bool) {
	if isRecovery {
		records, err := s.app.FindRecordsByFilter(
			"incidents",
			"monitor = {:monitor} && type = {:type} && (status = {:open} || status = {:acknowledged})",
			"-started_at",
			0,
			0,
			dbx.Params{
				"monitor":      m.ID,
				"type":         incident.TypeMonitorDown,
				"open":         incident.StatusOpen,
				"acknowledged": incident.StatusAcknowledged,
			},
		)
		if err != nil {
			log.Printf("[monitor-scheduler] Failed to find open incident: %v", err)
			return
		}

		now := time.Now()
		for _, record := range records {
			record.Set("status", incident.StatusResolved)
			record.Set("resolved_at", now)
			record.Set("resolution", fmt.Sprintf("Monitor recovered: %s", result.Msg))
			if err := s.app.Save(record); err != nil {
				log.Printf("[monitor-scheduler] Failed to resolve incident: %v", err)
			}
		}
		return
	}

	if newStatus != monitor.StatusDown || prevStatus == monitor.StatusDown {
		return
	}

	existing, err := s.app.FindFirstRecordByFilter(
		"incidents",
		"monitor = {:monitor} && type = {:type} && (status = {:open} || status = {:acknowledged})",
		dbx.Params{
			"monitor":      m.ID,
			"type":         incident.TypeMonitorDown,
			"open":         incident.StatusOpen,
			"acknowledged": incident.StatusAcknowledged,
		},
	)
	if err == nil && existing != nil {
		return
	}

	incidentCollection, err := s.app.FindCollectionByNameOrId("incidents")
	if err != nil {
		log.Printf("[monitor-scheduler] Could not create incident: %v", err)
		return
	}

	record := core.NewRecord(incidentCollection)
	record.Set("title", fmt.Sprintf("Monitor Down: %s", m.Name))
	record.Set("description", result.Msg)
	record.Set("type", incident.TypeMonitorDown)
	record.Set("severity", incident.SeverityHigh)
	record.Set("status", incident.StatusOpen)
	record.Set("monitor", m.ID)
	record.Set("started_at", time.Now())
	record.Set("user", m.UserID)

	if err := s.app.Save(record); err != nil {
		log.Printf("[monitor-scheduler] Failed to save incident: %v", err)
	}
}

// loadMonitors loads active monitors from the database
func (s *Scheduler) loadMonitors() error {
	records, err := s.app.FindRecordsByFilter("monitors", "active = true", "-created", 0, 0)
	if err != nil {
		return fmt.Errorf("failed to query monitors: %w", err)
	}

	for _, record := range records {
		m := recordToMonitor(record)
		s.monitors.Set(m.ID, &ScheduledMonitor{
			Monitor:   m,
			NextCheck: time.Now(),
		})
	}

	log.Printf("[monitor-scheduler] Loaded %d monitors", len(records))
	return nil
}

// AddMonitor adds a new monitor to the scheduler
func (s *Scheduler) AddMonitor(record *core.Record) {
	m := recordToMonitor(record)

	s.monitors.Set(m.ID, &ScheduledMonitor{
		Monitor:   m,
		NextCheck: time.Now(),
	})

	log.Printf("[monitor-scheduler] Added monitor: %s (%s)", m.Name, m.Type)
}

// UpdateMonitor updates a monitor in the scheduler
func (s *Scheduler) UpdateMonitor(record *core.Record) {
	m := recordToMonitor(record)

	// Get existing scheduled monitor
	if sm, ok := s.monitors.GetOk(m.ID); ok {
		sm.mu.Lock()
		wasPaused := sm.Monitor.Status == monitor.StatusPaused || !sm.Monitor.Active
		nowActive := m.Active && m.Status != monitor.StatusPaused
		// If monitor just became active (resumed), reset NextCheck so it's checked immediately
		if wasPaused && nowActive {
			sm.NextCheck = time.Now()
		}
		sm.Monitor = m
		sm.mu.Unlock()
	} else {
		s.monitors.Set(m.ID, &ScheduledMonitor{
			Monitor:   m,
			NextCheck: time.Now(),
		})
	}

	log.Printf("[monitor-scheduler] Updated monitor: %s", m.Name)
}

// RemoveMonitor removes a monitor from the scheduler
func (s *Scheduler) RemoveMonitor(monitorID string) {
	s.monitors.Remove(monitorID)
	log.Printf("[monitor-scheduler] Removed monitor: %s", monitorID)
}

// RunManualCheck runs a manual check for a monitor
func (s *Scheduler) RunManualCheck(monitorID string) (*monitor.CheckResult, error) {
	// Get monitor from database
	record, err := s.app.FindRecordById("monitors", monitorID)
	if err != nil {
		return nil, fmt.Errorf("monitor not found: %w", err)
	}

	m := recordToMonitor(record)

	// Get checker
	checker, ok := s.registry.Get(m.Type)
	if !ok {
		return nil, fmt.Errorf("no checker for type: %s", m.Type)
	}

	// Run check
	timeout := time.Duration(m.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result := checker.Check(ctx, m)
	if err := s.saveResult(m, result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetUptimeStats calculates uptime statistics for a monitor
func (s *Scheduler) GetUptimeStats(monitorID string, hours int) (*monitor.UptimeStats, error) {
	// Query heartbeats from the last N hours
	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	records, err := s.app.FindRecordsByFilter(
		"monitor_heartbeats",
		"monitor = {:monitorId} && time >= {:since}",
		"-time",
		0,
		0,
		map[string]any{
			"monitorId": monitorID,
			"since":     since.Format("2006-01-02 15:04:05"),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query heartbeats: %w", err)
	}

	stats := &monitor.UptimeStats{}

	for _, record := range records {
		stats.Total++
		status := record.GetString("status")
		if status == string(monitor.StatusUp) {
			stats.Up++
		} else if status == string(monitor.StatusDown) {
			stats.Down++
		}
	}

	if stats.Total > 0 {
		uptime := float64(stats.Up) / float64(stats.Total) * 100
		switch hours {
		case 24:
			stats.Uptime24h = uptime
		case 168: // 7 days
			stats.Uptime7d = uptime
		case 720: // 30 days
			stats.Uptime30d = uptime
		}
	}

	return stats, nil
}

func (s *Scheduler) calculateUptimeStats(monitorID string) (map[string]float64, error) {
	stats := make(map[string]float64)
	for _, window := range []struct {
		hours int
		key   string
	}{
		{24, "uptime_24h"},
		{168, "uptime_7d"},
		{720, "uptime_30d"},
	} {
		windowStats, err := s.GetUptimeStats(monitorID, window.hours)
		if err != nil {
			return nil, err
		}
		if windowStats.Total > 0 {
			stats[window.key] = float64(windowStats.Up) / float64(windowStats.Total) * 100
		}
		stats[fmt.Sprintf("checks_%s", window.key)] = float64(windowStats.Total)
	}

	avgPing, err := s.averagePing(monitorID, 24)
	if err != nil {
		return nil, err
	}
	stats["avg_ping_24h"] = avgPing
	return stats, nil
}

func (s *Scheduler) averagePing(monitorID string, hours int) (float64, error) {
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	records, err := s.app.FindRecordsByFilter(
		"monitor_heartbeats",
		"monitor = {:monitorId} && time >= {:since} && status = {:status}",
		"-time",
		0,
		0,
		dbx.Params{
			"monitorId": monitorID,
			"since":     since.Format("2006-01-02 15:04:05"),
			"status":    string(monitor.StatusUp),
		},
	)
	if err != nil {
		return 0, err
	}
	if len(records) == 0 {
		return 0, nil
	}
	total := 0
	for _, record := range records {
		total += record.GetInt("ping")
	}
	return float64(total) / float64(len(records)), nil
}

// recordToMonitor converts a PocketBase record to a Monitor struct
func recordToMonitor(record *core.Record) *monitor.Monitor {
	m := &monitor.Monitor{
		ID:                     record.Id,
		Name:                   record.GetString("name"),
		Type:                   record.GetString("type"),
		URL:                    record.GetString("url"),
		Hostname:               record.GetString("hostname"),
		Port:                   record.GetInt("port"),
		Method:                 record.GetString("method"),
		Headers:                record.GetString("headers"),
		Body:                   record.GetString("body"),
		Interval:               record.GetInt("interval"),
		Timeout:                record.GetInt("timeout"),
		Retries:                record.GetInt("retries"),
		RetryInterval:          record.GetInt("retry_interval"),
		MaxRedirects:           record.GetInt("max_redirects"),
		Keyword:                record.GetString("keyword"),
		JSONQuery:              record.GetString("json_query"),
		ExpectedValue:          record.GetString("expected_value"),
		InvertKeyword:          record.GetBool("invert_keyword"),
		DNSResolveServer:       record.GetString("dns_resolve_server"),
		DNSResolverMode:        record.GetString("dns_resolver_mode"),
		Status:                 monitor.Status(record.GetString("status")),
		Active:                 record.GetBool("active"),
		UserID:                 record.GetString("user"),
		Description:            record.GetString("description"),
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
	}

	// Parse JSON fields
	if tagsData := record.Get("tags"); tagsData != nil {
		if tags, ok := tagsData.([]string); ok {
			m.Tags = tags
		}
	}

	if statsData := record.Get("uptime_stats"); statsData != nil {
		stats := map[string]float64{}
		if raw, err := json.Marshal(statsData); err == nil {
			if err := json.Unmarshal(raw, &stats); err == nil {
				m.UptimeStats = stats
			}
		}
	}

	if lastCheck := record.Get("last_check"); lastCheck != nil {
		if t, ok := lastCheck.(time.Time); ok {
			m.LastCheck = t
		}
	}

	return m
}

// CleanupOldHeartbeats removes heartbeats older than retention period
func (s *Scheduler) CleanupOldHeartbeats(retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	records, err := s.app.FindRecordsByFilter(
		"monitor_heartbeats",
		"time < {:cutoff}",
		"",
		0,
		0,
		map[string]any{
			"cutoff": cutoff.Format("2006-01-02 15:04:05"),
		},
	)
	if err != nil {
		return fmt.Errorf("failed to find old heartbeats: %w", err)
	}

	deleted := 0
	for _, record := range records {
		if err := s.app.Delete(record); err == nil {
			deleted++
		}
	}

	log.Printf("[monitor-scheduler] Cleaned up %d old heartbeats", deleted)
	return nil
}
