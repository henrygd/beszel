// Package agent handles the agent's SSH server and system stats collection.
package agent

import (
	"beszel"
	"beszel/internal/entities/system"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

type Agent struct {
	sync.Mutex                               // Used to lock agent while collecting data
	debug         bool                       // true if LOG_LEVEL is set to debug
	zfs           bool                       // true if system has arcstats
	memCalc       string                     // Memory calculation formula
	fsNames       []string                   // List of filesystem device names being monitored
	fsStats       map[string]*system.FsStats // Keeps track of disk stats for each filesystem
	netInterfaces map[string]struct{}        // Stores all valid network interfaces
	netIoStats    system.NetIoStats          // Keeps track of bandwidth usage
	dockerManager *dockerManager             // Manages Docker API requests
	sensorConfig  *SensorConfig              // Sensors config
	systemInfo    system.Info                // Host system info
	gpuManager    *GPUManager                // Manages GPU data
	cache         *SessionCache              // Cache for system stats based on primary session ID
}

func NewAgent() *Agent {
	agent := &Agent{
		fsStats: make(map[string]*system.FsStats),
		cache:   NewSessionCache(69 * time.Second),
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

	// initialize system info / docker manager
	agent.initializesystemInfo()
	agent.initializeDiskInfo()
	agent.initializeNetIoStats()
	agent.dockerManager = newDockerManager(agent)

	// initialize GPU manager
	if gm, err := NewGPUManager(); err != nil {
		slog.Debug("GPU", "err", err)
	} else {
		agent.gpuManager = gm
	}

	// if debugging, print stats
	if agent.debug {
		slog.Debug("Stats", "data", agent.gatherStats(""))
	}

	return agent
}

// GetEnv retrieves an environment variable with a "BESZEL_AGENT_" prefix, or falls back to the unprefixed key.
func GetEnv(key string) (value string, exists bool) {
	if value, exists = os.LookupEnv("BESZEL_AGENT_" + key); exists {
		return value, exists
	}
	// Fallback to the old unprefixed key
	return os.LookupEnv(key)
}

func (a *Agent) gatherStats(sessionID string) *system.CombinedData {
	a.Lock()
	defer a.Unlock()

	cachedData, ok := a.cache.Get(sessionID)
	if ok {
		slog.Debug("Cached stats", "session", sessionID)
		return cachedData
	}

	*cachedData = system.CombinedData{
		Stats: a.getSystemStats(),
		Info:  a.systemInfo,
	}
	slog.Debug("System stats", "data", cachedData)

	if a.dockerManager != nil {
		if containerStats, err := a.dockerManager.getDockerStats(); err == nil {
			cachedData.Containers = containerStats
			slog.Debug("Docker stats", "data", cachedData.Containers)
		} else {
			slog.Debug("Docker stats", "err", err)
		}
	}

	cachedData.Stats.ExtraFs = make(map[string]*system.FsStats)
	for name, stats := range a.fsStats {
		if !stats.Root && stats.DiskTotal > 0 {
			cachedData.Stats.ExtraFs[name] = stats
		}
	}
	slog.Debug("Extra filesystems", "data", cachedData.Stats.ExtraFs)

	a.cache.Set(sessionID, cachedData)
	return cachedData
}
