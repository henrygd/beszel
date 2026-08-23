package uptime

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/henrygd/beszel/internal/alerts"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/subscriptions"
)

// Monitor status values (kept in sync with the frontend SystemStatus enum).
const (
	statusUp      = "up"
	statusDown    = "down"
	statusPaused  = "paused"
	statusPending = "pending"
)

// defaultIntervalSec is used when a monitor record has no interval set.
const defaultIntervalSec = 60

// checkTimeoutSec caps a single check when the record has no timeout.
const checkTimeoutSec = 10

// hubLike is the subset of hub.Hub required by the monitor manager.
type hubLike interface {
	core.App
	MakeLink(parts ...string) string
	SendAlert(data alerts.AlertMessageData) error
}

// MonitorManager periodically checks monitors and records the results.
type MonitorManager struct {
	hub      hubLike
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	monitors map[string]*monitorRuntime // id -> runtime
}

// monitorRuntime holds per-monitor runtime state.
type monitorRuntime struct {
	lastStatus string
	cancel     context.CancelFunc
}

// NewMonitorManager creates a new MonitorManager.
func NewMonitorManager(hub hubLike) *MonitorManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &MonitorManager{
		hub:      hub,
		ctx:      ctx,
		cancel:   cancel,
		monitors: make(map[string]*monitorRuntime),
	}
}

// Initialize binds event hooks and starts checking existing monitors.
func (m *MonitorManager) Initialize() error {
	m.bindEventHooks()

	var ids []struct {
		Id string
	}
	if err := m.hub.DB().NewQuery("SELECT id FROM monitors").All(&ids); err != nil {
		return err
	}

	// Start monitors with a small stagger so they don't all fire at once.
	go func() {
		for i, monitor := range ids {
			if !m.waitForContext(time.Duration(i)*time.Second) {
				return
			}
			if err := m.startMonitor(monitor.Id); err != nil {
				m.hub.Logger().Error("Failed to start monitor", "id", monitor.Id, "err", err)
			}
		}
	}()
	return nil
}

func (m *MonitorManager) bindEventHooks() {
	m.hub.OnRecordAfterCreateSuccess("monitors").BindFunc(func(e *core.RecordEvent) error {
		go m.startMonitor(e.Record.Id)
		return e.Next()
	})
	m.hub.OnRecordAfterUpdateSuccess("monitors").BindFunc(func(e *core.RecordEvent) error {
		// Restart monitor to pick up config/status changes.
		m.stopMonitor(e.Record.Id)
		if e.Record.GetString("status") != statusPaused {
			go m.startMonitor(e.Record.Id)
		}
		return e.Next()
	})
	m.hub.OnRecordAfterDeleteSuccess("monitors").BindFunc(func(e *core.RecordEvent) error {
		m.stopMonitor(e.Record.Id)
		return e.Next()
	})
	m.hub.OnTerminate().BindFunc(func(e *core.TerminateEvent) error {
		m.cancel()
		return e.Next()
	})
}

// startMonitor (re)starts the check loop for a monitor.
func (m *MonitorManager) startMonitor(monitorID string) error {
	m.mu.Lock()
	if rt, ok := m.monitors[monitorID]; ok {
		m.mu.Unlock()
		rt.cancel()
	}
	record, err := m.hub.FindRecordById("monitors", monitorID)
	if err != nil {
		m.mu.Unlock()
		return err
	}
	if record.GetString("status") == statusPaused {
		m.mu.Unlock()
		return nil
	}

	// remember previous status for state-change detection
	prev := statusPending
	if rt, ok := m.monitors[monitorID]; ok {
		prev = rt.lastStatus
	}

	ctx, cancel := context.WithCancel(m.ctx)
	m.monitors[monitorID] = &monitorRuntime{lastStatus: prev, cancel: cancel}
	m.mu.Unlock()

	// mark as pending if it has never been checked
	if prev == "" {
		record.Set("status", statusPending)
		if err := m.hub.Save(record); err != nil {
			m.hub.Logger().Error("Failed to set monitor pending", "id", monitorID, "err", err)
		}
		m.broadcast(record)
	}

	interval := time.Duration(record.GetInt("interval")) * time.Second
	if interval <= 0 {
		interval = defaultIntervalSec * time.Second
	}

	go m.checkLoop(ctx, record.Id, interval)
	return nil
}

// RunCheckNow performs an immediate check of the given monitor (API trigger).
func (m *MonitorManager) RunCheckNow(monitorID string) {
	go m.runSingleCheck(context.Background(), monitorID)
}

// stopMonitor cancels the check loop for a monitor.
func (m *MonitorManager) stopMonitor(monitorID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if rt, ok := m.monitors[monitorID]; ok {
		rt.cancel()
		delete(m.monitors, monitorID)
	}
}

// checkLoop runs checks for a monitor until the context is cancelled.
func (m *MonitorManager) checkLoop(ctx context.Context, monitorID string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// run an initial check immediately
	m.runSingleCheck(ctx, monitorID)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.runSingleCheck(ctx, monitorID)
		}
	}
}

// runSingleCheck performs one check, records the result, updates status,
// sends alerts and broadcasts realtime data.
func (m *MonitorManager) runSingleCheck(ctx context.Context, monitorID string) {
	record, err := m.hub.FindRecordById("monitors", monitorID)
	if err != nil {
		return
	}
	if record.GetString("status") == statusPaused {
		return
	}

	timeout := time.Duration(record.GetInt("timeout")) * time.Second
	if timeout <= 0 {
		timeout = checkTimeoutSec * time.Second
	}
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	up, _, errMsg := runCheck(checkCtx, record)
	elapsed := time.Since(start).Milliseconds()
	if checkCtx.Err() != nil {
		up = false
		if errMsg == "" {
			errMsg = "Timeout"
		}
	}

	newStatus := statusDown
	if up {
		newStatus = statusUp
	}

	// record the check
	collection, err := m.hub.FindCollectionByNameOrId("monitor_checks")
	if err != nil {
		m.hub.Logger().Error("Failed to find monitor_checks collection", "err", err)
		return
	}
	check := core.NewRecord(collection)
	check.Set("monitor", record.Id)
	check.Set("up", up)
	check.Set("ms", elapsed)
	check.Set("msg", errMsg)
	if err := m.hub.Save(check); err != nil {
		m.hub.Logger().Error("Failed to save monitor check", "monitor", monitorID, "err", err)
	}

	// update monitor status
	prevStatus := ""
	m.mu.Lock()
	if rt, ok := m.monitors[monitorID]; ok {
		prevStatus = rt.lastStatus
		rt.lastStatus = newStatus
	}
	m.mu.Unlock()

	record.Set("status", newStatus)
	if err := m.hub.Save(record); err != nil {
		m.hub.Logger().Error("Failed to save monitor status", "id", monitorID, "err", err)
	}

	m.broadcast(record)

	// alert on state changes
	if (prevStatus == statusUp && newStatus == statusDown) ||
		(prevStatus == statusDown && newStatus == statusUp) ||
		(prevStatus == "" && newStatus == statusUp) {
		m.sendStatusAlert(record, newStatus, errMsg)
	}
}

// sendStatusAlert notifies the monitor owner about an up/down transition.
func (m *MonitorManager) sendStatusAlert(record *core.Record, status string, errMsg string) {
	name := record.GetString("name")
	target := strings.TrimSpace(record.GetString("url"))
	if target == "" {
		target = strings.TrimSpace(record.GetString("host"))
	}
	title := "Monitor " + name + " is " + status
	message := "Your monitor " + name + " (" + target + ") is now " + status + "."
	if errMsg != "" {
		message += " " + errMsg
	}
	if err := m.hub.SendAlert(alerts.AlertMessageData{
		UserID:   record.GetString("user"),
		Title:    title,
		Message:  message,
		Link:     m.hub.MakeLink("monitors", record.Id),
		LinkText: "View monitor",
	}); err != nil {
		m.hub.Logger().Error("Failed to send monitor alert", "monitor", record.Id, "err", err)
	}
}

// broadcast sends the latest monitor state to subscribed realtime clients.
func (m *MonitorManager) broadcast(record *core.Record) {
	data, err := json.Marshal(map[string]any{
		"monitor": record.Id,
		"name":    record.GetString("name"),
		"type":    record.GetString("type"),
		"status":  record.GetString("status"),
		"ts":      time.Now().Unix(),
	})
	if err != nil {
		return
	}
	for _, client := range m.hub.SubscriptionsBroker().Clients() {
		for subscription := range client.Subscriptions() {
			if !strings.HasPrefix(subscription, "rt_uptime") {
				continue
			}
			// filter by monitor id query param, if provided
			if monitorId := client.Subscriptions()[subscription].Query["monitor"]; monitorId != "" && monitorId != record.Id {
				continue
			}
			client.Send(subscriptions.Message{Name: subscription, Data: data})
		}
	}
}

func (m *MonitorManager) waitForContext(d time.Duration) bool {
	if d <= 0 {
		return m.ctx.Err() == nil
	}
	select {
	case <-m.ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
