//go:build linux

package agent

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/henrygd/beszel/internal/entities/systemd"
)

var (
	errNoActiveTime = errors.New("no active time")
)

// systemdManager manages the collection of systemd service statistics.
type systemdManager struct {
	sync.Mutex
	serviceStatsMap map[string]*systemd.Service
	isRunning       bool
	hasFreshStats   bool
	patterns        []string
}

// newSystemdManager creates a new systemdManager.
func newSystemdManager() (*systemdManager, error) {
	conn, err := dbus.NewSystemConnectionContext(context.Background())
	if err != nil {
		slog.Warn("Error connecting to systemd", "err", err, "ref", "https://beszel.dev/guide/systemd")
		return nil, err
	}

	manager := &systemdManager{
		serviceStatsMap: make(map[string]*systemd.Service),
		patterns:        getServicePatterns(),
	}

	manager.startWorker(conn)

	return manager, nil
}

func (sm *systemdManager) startWorker(conn *dbus.Conn) {
	if sm.isRunning {
		return
	}
	sm.isRunning = true
	// prime the service stats map with the current services
	_ = sm.getServiceStats(conn, true)
	// update the services every 10 minutes
	go func() {
		for {
			time.Sleep(time.Minute * 10)
			_ = sm.getServiceStats(nil, true)
		}
	}()
}

// getServiceStatsCount returns the number of systemd services.
func (sm *systemdManager) getServiceStatsCount() int {
	return len(sm.serviceStatsMap)
}

// getFailedServiceCount returns the number of systemd services in a failed state.
func (sm *systemdManager) getFailedServiceCount() uint16 {
	sm.Lock()
	defer sm.Unlock()
	count := uint16(0)
	for _, service := range sm.serviceStatsMap {
		if service.State == systemd.StatusFailed {
			count++
		}
	}
	return count
}

// getServiceStats collects statistics for all running systemd services.
func (sm *systemdManager) getServiceStats(conn *dbus.Conn, refresh bool) []*systemd.Service {
	// start := time.Now()
	// defer func() {
	// 	slog.Info("systemdManager.getServiceStats", "duration", time.Since(start))
	// }()

	var services []*systemd.Service
	var err error

	if !refresh {
		// return nil
		sm.Lock()
		defer sm.Unlock()
		for _, service := range sm.serviceStatsMap {
			services = append(services, service)
		}
		sm.hasFreshStats = false
		return services
	}

	if conn == nil || !conn.Connected() {
		conn, err = dbus.NewSystemConnectionContext(context.Background())
		if err != nil {
			return nil
		}
		defer conn.Close()
	}

	units, err := conn.ListUnitsByPatternsContext(context.Background(), []string{"loaded"}, sm.patterns)
	if err != nil {
		slog.Error("Error listing systemd service units", "err", err)
		return nil
	}

	for _, unit := range units {
		service, err := sm.updateServiceStats(conn, unit)
		if err != nil {
			continue
		}
		services = append(services, service)
	}
	sm.hasFreshStats = true
	return services
}

// updateServiceStats updates the statistics for a single systemd service.
func (sm *systemdManager) updateServiceStats(conn *dbus.Conn, unit dbus.UnitStatus) (*systemd.Service, error) {
	sm.Lock()
	defer sm.Unlock()

	ctx := context.Background()

	// if service has never been active (no active since time), skip it
	if activeEnterTsProp, err := conn.GetUnitTypePropertyContext(ctx, unit.Name, "Unit", "ActiveEnterTimestamp"); err == nil {
		if ts, ok := activeEnterTsProp.Value.Value().(uint64); !ok || ts == 0 || ts == math.MaxUint64 {
			return nil, errNoActiveTime
		}
	} else {
		return nil, err
	}

	service, serviceExists := sm.serviceStatsMap[unit.Name]
	if !serviceExists {
		service = &systemd.Service{Name: unescapeServiceName(strings.TrimSuffix(unit.Name, ".service"))}
		sm.serviceStatsMap[unit.Name] = service
	}

	memPeak := service.MemPeak
	if memPeakProp, err := conn.GetUnitTypePropertyContext(ctx, unit.Name, "Service", "MemoryPeak"); err == nil {
		// If memPeak is MaxUint64 the api is saying it's not available
		if v, ok := memPeakProp.Value.Value().(uint64); ok && v != math.MaxUint64 {
			memPeak = v
		}
	}

	var memUsage uint64
	if memProp, err := conn.GetUnitTypePropertyContext(ctx, unit.Name, "Service", "MemoryCurrent"); err == nil {
		// If memUsage is MaxUint64 the api is saying it's not available
		if v, ok := memProp.Value.Value().(uint64); ok && v != math.MaxUint64 {
			memUsage = v
		}
	}

	service.State = systemd.ParseServiceStatus(unit.ActiveState)
	service.Sub = systemd.ParseServiceSubState(unit.SubState)

	// some systems always return 0 for mem peak, so we should update the peak if the current usage is greater
	if memUsage > memPeak {
		memPeak = memUsage
	}

	var cpuUsage uint64
	if cpuProp, err := conn.GetUnitTypePropertyContext(ctx, unit.Name, "Service", "CPUUsageNSec"); err == nil {
		if v, ok := cpuProp.Value.Value().(uint64); ok {
			cpuUsage = v
		}
	}

	service.Mem = memUsage
	if memPeak > service.MemPeak {
		service.MemPeak = memPeak
	}
	service.UpdateCPUPercent(cpuUsage)

	return service, nil
}

// getServiceDetails collects extended information for a specific systemd service.
func (sm *systemdManager) getServiceDetails(serviceName string) (systemd.ServiceDetails, error) {
	conn, err := dbus.NewSystemConnectionContext(context.Background())
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	unitName := serviceName
	if !strings.HasSuffix(unitName, ".service") {
		unitName += ".service"
	}

	ctx := context.Background()
	props, err := conn.GetUnitPropertiesContext(ctx, unitName)
	if err != nil {
		return nil, err
	}

	// Start with all unit properties
	details := make(systemd.ServiceDetails)
	maps.Copy(details, props)

	// // Add service-specific properties
	servicePropNames := []string{
		"MainPID", "ExecMainPID", "TasksCurrent", "TasksMax",
		"MemoryCurrent", "MemoryPeak", "MemoryLimit", "CPUUsageNSec",
		"NRestarts", "ExecMainStartTimestampRealtime", "Result",
	}

	for _, propName := range servicePropNames {
		if variant, err := conn.GetUnitTypePropertyContext(ctx, unitName, "Service", propName); err == nil {
			value := variant.Value.Value()
			// Check if the value is MaxUint64, which indicates unlimited/infinite
			if uint64Value, ok := value.(uint64); ok && uint64Value == math.MaxUint64 {
				// Set to nil to indicate unlimited - frontend will handle this appropriately
				details[propName] = nil
			} else {
				details[propName] = value
			}
		}
	}

	return details, nil
}

// unescapeServiceName unescapes systemd service names that contain C-style escape sequences like \x2d
func unescapeServiceName(name string) string {
	if !strings.Contains(name, "\\x") {
		return name
	}
	unescaped, err := strconv.Unquote("\"" + name + "\"")
	if err != nil {
		return name
	}
	return unescaped
}

// getServicePatterns returns the list of service patterns to match.
// It reads from the SERVICE_PATTERNS environment variable if set,
// otherwise defaults to "*service".
func getServicePatterns() []string {
	patterns := []string{}
	if envPatterns, _ := GetEnv("SERVICE_PATTERNS"); envPatterns != "" {
		for pattern := range strings.SplitSeq(envPatterns, ",") {
			pattern = strings.TrimSpace(pattern)
			if pattern == "" {
				continue
			}
			if !strings.HasSuffix(pattern, ".service") {
				pattern += ".service"
			}
			patterns = append(patterns, pattern)
		}
	}
	if len(patterns) == 0 {
		patterns = []string{"*.service"}
	}
	return patterns
}
