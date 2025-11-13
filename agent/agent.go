// Package agent implements the Beszel monitoring agent that collects and serves system metrics.
//
// The agent runs on monitored systems and communicates collected data
// to the Beszel hub for centralized monitoring and alerting.
package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gliderlabs/ssh"
	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/agent/deltatracker"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/shirou/gopsutil/v4/host"
	gossh "golang.org/x/crypto/ssh"
)

type Agent struct {
	sync.Mutex                                                                      // Used to lock agent while collecting data
	debug                     bool                                                  // true if LOG_LEVEL is set to debug
	zfs                       bool                                                  // true if system has arcstats
	memCalc                   string                                                // Memory calculation formula
	fsNames                   []string                                              // List of filesystem device names being monitored
	fsStats                   map[string]*system.FsStats                            // Keeps track of disk stats for each filesystem
	diskPrev                  map[uint16]map[string]prevDisk                        // Previous disk I/O counters per cache interval
	netInterfaces             map[string]struct{}                                   // Stores all valid network interfaces
	netIoStats                map[uint16]system.NetIoStats                          // Keeps track of bandwidth usage per cache interval
	netInterfaceDeltaTrackers map[uint16]*deltatracker.DeltaTracker[string, uint64] // Per-cache-time NIC delta trackers
	dockerManager             *dockerManager                                        // Manages Docker API requests
	sensorConfig              *SensorConfig                                         // Sensors config
	systemInfo                system.Info                                           // Host system info
	gpuManager                *GPUManager                                           // Manages GPU data
	cache                     *systemDataCache                                      // Cache for system stats based on cache time
	connectionManager         *ConnectionManager                                    // Channel to signal connection events
	handlerRegistry           *HandlerRegistry                                      // Registry for routing incoming messages
	server                    *ssh.Server                                           // SSH server
	dataDir                   string                                                // Directory for persisting data
	keys                      []gossh.PublicKey                                     // SSH public keys
	smartManager              *SmartManager                                         // Manages SMART data
	systemdManager            *systemdManager                                       // Manages systemd services
}

// NewAgent creates a new agent with the given data directory for persisting data.
// If the data directory is not set, it will attempt to find the optimal directory.
func NewAgent(dataDir ...string) (agent *Agent, err error) {
	agent = &Agent{
		fsStats: make(map[string]*system.FsStats),
		cache:   NewSystemDataCache(),
	}

	// Initialize disk I/O previous counters storage
	agent.diskPrev = make(map[uint16]map[string]prevDisk)
	// Initialize per-cache-time network tracking structures
	agent.netIoStats = make(map[uint16]system.NetIoStats)
	agent.netInterfaceDeltaTrackers = make(map[uint16]*deltatracker.DeltaTracker[string, uint64])

	agent.dataDir, err = getDataDir(dataDir...)
	if err != nil {
		slog.Warn("Data directory not found")
	} else {
		slog.Info("Data directory", "path", agent.dataDir)
	}

	agent.memCalc, _ = GetEnv("MEM_CALC")
	agent.sensorConfig = agent.newSensorConfig()
	// Set up slog with a log level determined by the LOG_LEVEL env var
	if logLevelStr, exists := GetEnv("LOG_LEVEL"); exists {
		switch strings.ToLower(logLevelStr) {
		case "debug":
			agent.debug = true
			slog.SetLogLoggerLevel(slog.LevelDebug)
		case "warn":
			slog.SetLogLoggerLevel(slog.LevelWarn)
		case "error":
			slog.SetLogLoggerLevel(slog.LevelError)
		}
	}

	slog.Debug(beszel.Version)

	// initialize system info
	agent.initializeSystemInfo()

	// initialize connection manager
	agent.connectionManager = newConnectionManager(agent)

	// initialize handler registry
	agent.handlerRegistry = NewHandlerRegistry()

	// initialize disk info
	agent.initializeDiskInfo()

	// initialize net io stats
	agent.initializeNetIoStats()

	// initialize docker manager
	agent.dockerManager = newDockerManager(agent)

	agent.systemdManager, err = newSystemdManager()
	if err != nil {
		slog.Debug("Systemd", "err", err)
	}

	agent.smartManager, err = NewSmartManager()
	if err != nil {
		slog.Debug("SMART", "err", err)
	}

	// initialize GPU manager
	agent.gpuManager, err = NewGPUManager()
	if err != nil {
		slog.Debug("GPU", "err", err)
	}

	// if debugging, print stats
	if agent.debug {
		slog.Debug("Stats", "data", agent.gatherStats(0))
	}

	return agent, nil
}

// GetEnv retrieves an environment variable with a "BESZEL_AGENT_" prefix, or falls back to the unprefixed key.
func GetEnv(key string) (value string, exists bool) {
	if value, exists = os.LookupEnv("BESZEL_AGENT_" + key); exists {
		return value, exists
	}
	// Fallback to the old unprefixed key
	return os.LookupEnv(key)
}

func (a *Agent) gatherStats(cacheTimeMs uint16) *system.CombinedData {
	a.Lock()
	defer a.Unlock()

	data, isCached := a.cache.Get(cacheTimeMs)
	if isCached {
		slog.Debug("Cached data", "cacheTimeMs", cacheTimeMs)
		return data
	}

	*data = system.CombinedData{
		Stats: a.getSystemStats(cacheTimeMs),
		Info:  a.systemInfo,
	}
	// slog.Info("System data", "data", data, "cacheTimeMs", cacheTimeMs)

	if a.dockerManager != nil {
		if containerStats, err := a.dockerManager.getDockerStats(cacheTimeMs); err == nil {
			data.Containers = containerStats
			slog.Debug("Containers", "data", data.Containers)
		} else {
			slog.Debug("Containers", "err", err)
		}
	}

	// skip updating systemd services if cache time is not the default 60sec interval
	if a.systemdManager != nil && cacheTimeMs == 60_000 {
		totalCount := uint16(a.systemdManager.getServiceStatsCount())
		if totalCount > 0 {
			numFailed := a.systemdManager.getFailedServiceCount()
			data.Info.Services = []uint16{totalCount, numFailed}
		}
		if a.systemdManager.hasFreshStats {
			data.SystemdServices = a.systemdManager.getServiceStats(nil, false)
		}
	}

	data.Stats.ExtraFs = make(map[string]*system.FsStats)
	data.Info.ExtraFsPct = make(map[string]float64)
	for name, stats := range a.fsStats {
		if !stats.Root && stats.DiskTotal > 0 {
			// Use custom name if available, otherwise use device name
			key := name
			if stats.Name != "" {
				key = stats.Name
			}
			data.Stats.ExtraFs[key] = stats
			// Add percentages to Info struct for dashboard
			if stats.DiskTotal > 0 {
				pct := twoDecimals((stats.DiskUsed / stats.DiskTotal) * 100)
				data.Info.ExtraFsPct[key] = pct
			}
		}
	}
	slog.Debug("Extra FS", "data", data.Stats.ExtraFs)

	a.cache.Set(data, cacheTimeMs)
	return data
}

// StartAgent initializes and starts the agent with optional WebSocket connection
func (a *Agent) Start(serverOptions ServerOptions) error {
	a.keys = serverOptions.Keys
	return a.connectionManager.Start(serverOptions)
}

func (a *Agent) getFingerprint() string {
	// first look for a fingerprint in the data directory
	if a.dataDir != "" {
		if fp, err := os.ReadFile(filepath.Join(a.dataDir, "fingerprint")); err == nil {
			return string(fp)
		}
	}

	// if no fingerprint is found, generate one
	fingerprint, err := host.HostID()
	if err != nil || fingerprint == "" {
		fingerprint = a.systemInfo.Hostname + a.systemInfo.CpuModel
	}

	// hash fingerprint
	sum := sha256.Sum256([]byte(fingerprint))
	fingerprint = hex.EncodeToString(sum[:24])

	// save fingerprint to data directory
	if a.dataDir != "" {
		err = os.WriteFile(filepath.Join(a.dataDir, "fingerprint"), []byte(fingerprint), 0644)
		if err != nil {
			slog.Warn("Failed to save fingerprint", "err", err)
		}
	}

	return fingerprint
}
