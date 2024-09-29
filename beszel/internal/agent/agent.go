// Package agent handles the agent's SSH server and system stats collection.
package agent

import (
	"beszel/internal/entities/container"
	"beszel/internal/entities/system"
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/shirou/gopsutil/v4/common"
)

type Agent struct {
	hostname            string                      // Hostname of the system
	kernelVersion       string                      // Kernel version of the system
	cpuModel            string                      // CPU model of the system
	cores               int                         // Number of cores of the system
	threads             int                         // Number of threads of the system
	debug               bool                        // true if LOG_LEVEL is set to debug
	fsNames             []string                    // List of filesystem device names being monitored
	fsStats             map[string]*system.FsStats  // Keeps track of disk stats for each filesystem
	netInterfaces       map[string]struct{}         // Stores all valid network interfaces
	netIoStats          system.NetIoStats           // Keeps track of bandwidth usage
	containerStatsMap   map[string]*container.Stats // Keeps track of container stats
	containerStatsMutex sync.RWMutex                // Mutex to prevent concurrent access to prevContainerStatsMap
	dockerClient        *http.Client                // HTTP client to query docker api
	apiContainerList    *[]container.ApiInfo        // List of containers from docker host
	sensorsContext      context.Context             // Sensors context to override sys location
	sensorsWhitelist    map[string]struct{}         // List of sensors to monitor
}

func NewAgent() *Agent {
	return &Agent{
		containerStatsMap:   make(map[string]*container.Stats),
		containerStatsMutex: sync.RWMutex{},
		netIoStats:          system.NetIoStats{},
		dockerClient:        newDockerClient(),
		sensorsContext:      context.Background(),
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
			a.sensorsWhitelist[sensor] = struct{}{}
		}
	}

	a.initializeSystemInfo()
	a.initializeDiskInfo()
	a.initializeNetIoStats()

	a.startServer(pubKey, addr)
}

func (a *Agent) gatherStats() system.CombinedData {
	systemInfo, SystemStats := a.getSystemStats()
	systemData := system.CombinedData{
		Stats: SystemStats,
		Info:  systemInfo,
	}
	// add docker stats
	if containerStats, err := a.getDockerStats(); err == nil {
		systemData.Containers = containerStats
	}
	// add extra filesystems
	systemData.Stats.ExtraFs = make(map[string]*system.FsStats)
	for name, stats := range a.fsStats {
		if !stats.Root && stats.DiskTotal > 0 {
			systemData.Stats.ExtraFs[name] = stats
		}
	}
	return systemData
}
