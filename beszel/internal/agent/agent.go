// Package agent handles the agent's SSH server and system stats collection.
package agent

import (
	"beszel"
	"beszel/internal/entities/system"
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/shirou/gopsutil/v4/common"
)

type Agent struct {
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
	return &Agent{
		sensorsContext: context.Background(),
		memCalc:        os.Getenv("MEM_CALC"),
		fsStats:        make(map[string]*system.FsStats),
	}
}

func (a *Agent) Run(pubKey []byte, addr string) {
	// Set up slog with a log level determined by the LOG_LEVEL env var
	if logLevelStr, exists := os.LookupEnv("LOG_LEVEL"); exists {
		switch strings.ToLower(logLevelStr) {
		case "debug":
			a.debug = true
			slog.SetLogLoggerLevel(slog.LevelDebug)
		case "warn":
			slog.SetLogLoggerLevel(slog.LevelWarn)
		case "error":
			slog.SetLogLoggerLevel(slog.LevelError)
		}
	}

	slog.Debug(beszel.Version)

	// Set sensors context (allows overriding sys location for sensors)
	if sysSensors, exists := os.LookupEnv("SYS_SENSORS"); exists {
		slog.Info("SYS_SENSORS", "path", sysSensors)
		a.sensorsContext = context.WithValue(a.sensorsContext,
			common.EnvKey, common.EnvMap{common.HostSysEnvKey: sysSensors},
		)
	}

	// Set sensors whitelist
	if sensors, exists := os.LookupEnv("SENSORS"); exists {
		a.sensorsWhitelist = make(map[string]struct{})
		for _, sensor := range strings.Split(sensors, ",") {
			if sensor != "" {
				a.sensorsWhitelist[sensor] = struct{}{}
			}
		}
	}

	// initialize system info / docker manager
	a.initializeSystemInfo()
	a.initializeDiskInfo()
	a.initializeNetIoStats()
	a.dockerManager = newDockerManager(a)

	// initialize GPU manager
	if gm, err := NewGPUManager(); err != nil {
		slog.Debug("GPU", "err", err)
	} else {
		a.gpuManager = gm
	}

	// if debugging, print stats
	if a.debug {
		slog.Debug("Stats", "data", a.gatherStats())
	}

	a.startServer(pubKey, addr)
}

func (a *Agent) gatherStats() system.CombinedData {
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
