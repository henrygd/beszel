// Package agent handles the agent's SSH server and system stats collection.
package agent

import (
	"beszel"
	"beszel/internal/entities/system"
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/shirou/gopsutil/v4/common"
)

type Agent struct {
	sync.Mutex                                  // Used to lock agent while collecting data
	debug            bool                       // true if LOG_LEVEL is set to debug
	zfs              bool                       // true if system has arcstats
	memCalc          string                     // Memory calculation formula
	fsNames          []string                   // List of filesystem device names being monitored
	fsStats          map[string]*system.FsStats // Keeps track of disk stats for each filesystem
	netInterfaces    map[string]struct{}        // Stores all valid network interfaces
	netIoStats       system.NetIoStats          // Keeps track of bandwidth usage
	dockerManager    *dockerManager             // Manages Docker API requests
	sensorsContext   context.Context            // Sensors context to override sys location
	sensorsWhitelist map[string]struct{}        // List of sensors to monitor
	systemInfo       system.Info                // Host system info
	gpuManager       *GPUManager                // Manages GPU data
}

func NewAgent() *Agent {
	agent := &Agent{
		fsStats: make(map[string]*system.FsStats),
	}
	agent.memCalc, _ = GetEnv("MEM_CALC")

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

	// Set sensors context (allows overriding sys location for sensors)
	if sysSensors, exists := GetEnv("SYS_SENSORS"); exists {
		slog.Info("SYS_SENSORS", "path", sysSensors)
		agent.sensorsContext = context.WithValue(agent.sensorsContext,
			common.EnvKey, common.EnvMap{common.HostSysEnvKey: sysSensors},
		)
	} else {
		agent.sensorsContext = context.Background()
	}

	// Set sensors whitelist
	if sensors, exists := GetEnv("SENSORS"); exists {
		agent.sensorsWhitelist = make(map[string]struct{})
		for _, sensor := range strings.Split(sensors, ",") {
			if sensor != "" {
				agent.sensorsWhitelist[sensor] = struct{}{}
			}
		}
	}

	// initialize system info / docker manager
	agent.initializeSystemInfo()
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
		slog.Debug("Stats", "data", agent.gatherStats())
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

func (a *Agent) gatherStats() system.CombinedData {
	a.Lock()
	defer a.Unlock()
	slog.Debug("Getting stats")
	systemData := system.CombinedData{
		Stats: a.getSystemStats(),
		Info:  a.systemInfo,
	}
	slog.Debug("System stats", "data", systemData)
	// add docker stats
	if containerStats, err := a.dockerManager.getDockerStats(); err == nil {
		systemData.Containers = containerStats
		slog.Debug("Docker stats", "data", systemData.Containers)
	} else {
		slog.Debug("Error getting docker stats", "err", err)
	}
	// add extra filesystems
	systemData.Stats.ExtraFs = make(map[string]*system.FsStats)
	for name, stats := range a.fsStats {
		if !stats.Root && stats.DiskTotal > 0 {
			systemData.Stats.ExtraFs[name] = stats
		}
	}
	slog.Debug("Extra filesystems", "data", systemData.Stats.ExtraFs)
	return systemData
}
