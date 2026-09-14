package agent

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/henrygd/beszel/internal/entities/monitor"
)

// MonitorManager manages network monitor configurations and task lifetimes.
type MonitorManager struct {
	mu       sync.RWMutex
	monitors map[string]*monitorTask // keyed by monitor ID
	probe    monitorProbe
}

func newMonitorManager() *MonitorManager {
	return newMonitorManagerWithProbe(networkMonitorProbe(&http.Client{Timeout: monitor.MaxProbeTimeout}))
}

func newMonitorManagerWithProbe(probe monitorProbe) *MonitorManager {
	return &MonitorManager{monitors: make(map[string]*monitorTask), probe: probe}
}

// SyncMonitors replaces all monitor tasks with the given configs.
func (pm *MonitorManager) SyncMonitors(configs []monitor.Config) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Build set of new keys
	newKeys := make(map[string]monitor.Config, len(configs))
	for _, cfg := range configs {
		if cfg.ID == "" {
			continue
		}
		newKeys[cfg.ID] = cfg
	}

	// Stop removed monitors
	for key, task := range pm.monitors {
		if _, exists := newKeys[key]; !exists {
			task.cancel()
			delete(pm.monitors, key)
		}
	}

	// Start new monitors and restart tasks whose config changed.
	for key, cfg := range newKeys {
		task, exists := pm.monitors[key]
		if exists && task.config == cfg {
			continue
		}
		if exists {
			task.cancel()
		}
		task = newMonitorTaskFromExisting(cfg, task)
		pm.monitors[key] = task
		pm.startMonitor(task)
	}
}

// HandleSyncRequest applies a full or incremental monitor sync request.
func (pm *MonitorManager) HandleSyncRequest(req monitor.SyncRequest) (monitor.SyncResponse, error) {
	switch req.Action {
	case monitor.SyncActionReplace:
		pm.SyncMonitors(req.Configs)
		return monitor.SyncResponse{}, nil
	case monitor.SyncActionUpsert:
		result, err := pm.UpsertMonitor(req.Config, req.RunNow)
		if err != nil {
			return monitor.SyncResponse{}, err
		}
		if result == nil {
			return monitor.SyncResponse{}, nil
		}
		return monitor.SyncResponse{Result: *result}, nil
	case monitor.SyncActionDelete:
		if req.Config.ID == "" {
			return monitor.SyncResponse{}, errors.New("missing monitor ID for delete")
		}
		pm.DeleteMonitor(req.Config.ID)
		return monitor.SyncResponse{}, nil
	default:
		return monitor.SyncResponse{}, fmt.Errorf("unknown monitor sync action: %d", req.Action)
	}
}

// UpsertMonitor creates or replaces a single monitor task.
func (pm *MonitorManager) UpsertMonitor(config monitor.Config, runNow bool) (*monitor.Result, error) {
	if config.ID == "" {
		return nil, errors.New("missing monitor ID")
	}

	pm.mu.Lock()
	task, exists := pm.monitors[config.ID]
	if exists && task.config == config {
		pm.mu.Unlock()
		if !runNow {
			return nil, nil
		}
		return task.runProbe(pm.probe), nil
	}
	if exists {
		task.cancel()
	}
	task = newMonitorTaskFromExisting(config, task)
	pm.monitors[config.ID] = task
	pm.mu.Unlock()

	if runNow {
		result := task.runProbe(pm.probe)
		pm.startMonitor(task)
		return result, nil
	}
	pm.startMonitor(task)
	return nil, nil
}

// DeleteMonitor stops and removes a single monitor task.
func (pm *MonitorManager) DeleteMonitor(id string) {
	if id == "" {
		return
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if task, exists := pm.monitors[id]; exists {
		task.cancel()
		delete(pm.monitors, id)
	}
}

// GetResults returns aggregated results for all monitors over the last supplied duration in ms.
func (pm *MonitorManager) GetResults(durationMs uint16) map[string]monitor.Result {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	results := make(map[string]monitor.Result, len(pm.monitors))
	now := time.Now()
	duration := time.Duration(durationMs) * time.Millisecond

	for _, task := range pm.monitors {
		result, ok := task.history.result(duration, now)

		if !ok {
			continue
		}
		results[task.config.ID] = result
	}

	return results
}

// Stop stops all monitor tasks.
func (pm *MonitorManager) Stop() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for key, task := range pm.monitors {
		task.cancel()
		delete(pm.monitors, key)
	}
}
