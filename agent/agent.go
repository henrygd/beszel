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
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/shirou/gopsutil/v4/host"
	gossh "golang.org/x/crypto/ssh"
)

type Agent struct {
	sync.Mutex                                   // Used to lock agent while collecting data
	debug             bool                       // true if LOG_LEVEL is set to debug
	zfs               bool                       // true if system has arcstats
	memCalc           string                     // Memory calculation formula
	fsNames           []string                   // List of filesystem device names being monitored
	fsStats           map[string]*system.FsStats // Keeps track of disk stats for each filesystem
	netInterfaces     map[string]struct{}        // Stores all valid network interfaces
	netIoStats        system.NetIoStats          // Keeps track of bandwidth usage
	dockerManager     *dockerManager             // Manages Docker API requests
	systemdManager    *systemdManager            // Manages systemd services
	sensorConfig      *SensorConfig              // Sensors config
	systemInfo        system.Info                // Host system info
	gpuManager        *GPUManager                // Manages GPU data
	cache             *SessionCache              // Cache for system stats based on primary session ID
	connectionManager *ConnectionManager         // Channel to signal connection events
	server            *ssh.Server                // SSH server
	dataDir           string                     // Directory for persisting data
	keys              []gossh.PublicKey          // SSH public keys
}

// NewAgent creates a new agent with the given data directory for persisting data.
// If the data directory is not set, it will attempt to find the optimal directory.
func NewAgent(dataDir ...string) (agent *Agent, err error) {
	agent = &Agent{
		fsStats: make(map[string]*system.FsStats),
		cache:   NewSessionCache(69 * time.Second),
	}

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

	// initialize disk info
	agent.initializeDiskInfo()

	// initialize net io stats
	agent.initializeNetIoStats()

	// initialize docker manager
	agent.dockerManager = newDockerManager(agent)

	// initialize systemd manager
	if sm, err := newSystemdManager(); err != nil {
		slog.Debug("Systemd", "err", err)
	} else {
		agent.systemdManager = sm
	}

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

func (a *Agent) gatherStats(sessionID string) *system.CombinedData {
	a.Lock()
	defer a.Unlock()

	data, isCached := a.cache.Get(sessionID)
	if isCached {
		slog.Debug("Cached data", "session", sessionID)
		return data
	}

	*data = system.CombinedData{
		Stats: a.getSystemStats(),
		Info:  a.systemInfo,
	}
	slog.Debug("System data", "data", data)

	if a.dockerManager != nil {
		if containerStats, err := a.dockerManager.getDockerStats(); err == nil {
			data.Containers = containerStats
			slog.Debug("Containers", "data", data.Containers)
		} else {
			slog.Debug("Containers", "err", err)
		}
	}

	if a.systemdManager != nil {
		data.SystemdServices = a.systemdManager.getServiceStats()
		slog.Debug("Systemd services", "data", data.SystemdServices)
	}

	data.Stats.ExtraFs = make(map[string]*system.FsStats)
	for name, stats := range a.fsStats {
		if !stats.Root && stats.DiskTotal > 0 {
			data.Stats.ExtraFs[name] = stats
		}
	}
	slog.Debug("Extra FS", "data", data.Stats.ExtraFs)

	a.cache.Set(sessionID, data)
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
