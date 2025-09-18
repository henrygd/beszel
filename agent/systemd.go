//go:build linux

package agent

import (
	"context"
	"log/slog"
	"math"
	"strings"
	"sync"

	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/henrygd/beszel/internal/entities/systemd"
)

// systemdManager manages the collection of systemd service statistics.
type systemdManager struct {
	conn            *dbus.Conn
	serviceStatsMap map[string]*systemd.Service
	mu              sync.Mutex
}

// newSystemdManager creates a new systemdManager.
func newSystemdManager() (*systemdManager, error) {
	conn, err := dbus.New()
	if err != nil {
		if strings.Contains(err.Error(), "permission denied") {
			slog.Error("Permission denied when connecting to systemd. Run as root or with appropriate user permissions.", "err", err)
			return nil, err
		}
		slog.Error("Error connecting to systemd", "err", err)
		return nil, err
	}

	return &systemdManager{
		conn:            conn,
		serviceStatsMap: make(map[string]*systemd.Service),
	}, nil
}

// getServiceStats collects statistics for all running systemd services.
func (sm *systemdManager) getServiceStats() []*systemd.Service {
	units, err := sm.conn.ListUnitsContext(context.Background())
	if err != nil {
		slog.Error("Error listing systemd units", "err", err)
		return nil
	}

	var services []*systemd.Service
	for _, unit := range units {
		if strings.HasSuffix(unit.Name, ".service") {
			service := sm.updateServiceStats(unit)
			services = append(services, service)
		}
	}
	return services
}

// updateServiceStats updates the statistics for a single systemd service.
func (sm *systemdManager) updateServiceStats(unit dbus.UnitStatus) *systemd.Service {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	props, err := sm.conn.GetUnitTypeProperties(unit.Name, "Service")
	if err != nil {
		slog.Debug("could not get unit type properties", "unit", unit.Name, "err", err)
		return &systemd.Service{
			Name:   unit.Name,
			Status: unit.ActiveState,
		}
	}

	var cpuUsage uint64
	if val, ok := props["CPUUsageNSec"]; ok {
		if v, ok := val.(uint64); ok {
			cpuUsage = v
		}
	}

	var memUsage uint64
	if val, ok := props["MemoryCurrent"]; ok {
		if v, ok := val.(uint64); ok {
			memUsage = v
		}
	}

	service, exists := sm.serviceStatsMap[unit.Name]
	if !exists {
		service = &systemd.Service{
			Name:   unit.Name,
			Status: unit.ActiveState,
		}
		sm.serviceStatsMap[unit.Name] = service
	}

	service.Status = unit.ActiveState

	// If memUsage is MaxUint64 the api is saying it's not available, return 0
	if memUsage == math.MaxUint64 {
		memUsage = 0
	}

	service.Mem = float64(memUsage) / (1024 * 1024) // Convert to MB
	service.CalculateCPUPercent(cpuUsage)

	return service
}
