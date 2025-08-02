// Package agent handles configuration management for the agent
package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"beszel/internal/entities/system"
	"log/slog"
)

// AgentConfig represents the configuration pulled from the hub
type AgentConfig struct {
	LogLevel      string            `json:"log_level,omitempty"`
	MemCalc       string            `json:"mem_calc,omitempty"`
	ExtraFs       []string          `json:"extra_fs,omitempty"`
	DataDir       string            `json:"data_dir,omitempty"`
	DockerHost    string            `json:"docker_host,omitempty"`
	Filesystem    string            `json:"filesystem,omitempty"`
	Listen        string            `json:"listen,omitempty"`
	Network       string            `json:"network,omitempty"`
	Nics          string            `json:"nics,omitempty"`
	PrimarySensor string            `json:"primary_sensor,omitempty"`
	Sensors       string            `json:"sensors,omitempty"`
	SysSensors    string            `json:"sys_sensors,omitempty"`
	Environment   map[string]string `json:"environment,omitempty"`
	LastUpdated   time.Time         `json:"last_updated"`
	Version       string            `json:"version"`
}

// ConfigManager handles configuration pulling and management
type ConfigManager struct {
	hubURL     string
	token      string
	config     *AgentConfig
	lastUpdate time.Time
}

// NewConfigManager creates a new configuration manager
func NewConfigManager(hubURL, token string) *ConfigManager {
	return &ConfigManager{
		hubURL: hubURL,
		token:  token,
		config: &AgentConfig{},
	}
}

// PullConfig fetches configuration from the hub
func (cm *ConfigManager) PullConfig() error {
	if cm.hubURL == "" || cm.token == "" {
		slog.Debug("No hub URL or token provided, skipping config pull")
		return nil
	}

	// Construct the API URL
	apiURL := strings.TrimSuffix(cm.hubURL, "/") + "/api/beszel/agent-config"

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Create request
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add authorization header
	req.Header.Set("Authorization", "Bearer "+cm.token)

	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to fetch config: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Read and parse the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	var config AgentConfig
	if err := json.Unmarshal(body, &config); err != nil {
		return fmt.Errorf("failed to parse config JSON: %w", err)
	}

	cm.config = &config
	cm.lastUpdate = time.Now()

	slog.Info("Successfully pulled configuration from hub",
		"log_level", config.LogLevel,
		"mem_calc", config.MemCalc,
		"extra_fs_count", len(config.ExtraFs),
		"data_dir", config.DataDir,
		"docker_host", config.DockerHost,
		"filesystem", config.Filesystem,
		"listen", config.Listen,
		"network", config.Network,
		"nics", config.Nics,
		"primary_sensor", config.PrimarySensor,
		"sensors", config.Sensors,
		"sys_sensors", config.SysSensors,
		"env_vars_count", len(config.Environment))

	return nil
}

// ApplyConfig applies the pulled configuration to the agent
func (cm *ConfigManager) ApplyConfig(agent *Agent) error {
	if cm.config == nil {
		return nil
	}

	// Apply log level
	if cm.config.LogLevel != "" {
		switch strings.ToLower(cm.config.LogLevel) {
		case "debug":
			agent.debug = true
			slog.SetLogLoggerLevel(slog.LevelDebug)
		case "warn":
			slog.SetLogLoggerLevel(slog.LevelWarn)
		case "error":
			slog.SetLogLoggerLevel(slog.LevelError)
		default:
			slog.SetLogLoggerLevel(slog.LevelInfo)
		}
	}

	// Apply memory calculation
	if cm.config.MemCalc != "" {
		agent.memCalc = cm.config.MemCalc
	}

	// Apply extra filesystems
	if len(cm.config.ExtraFs) > 0 {
		// Add extra filesystems to the existing list
		for _, fs := range cm.config.ExtraFs {
			// Check if this filesystem is already in the list
			found := false
			for _, existingFs := range agent.fsNames {
				if existingFs == fs {
					found = true
					break
				}
			}
			if !found {
				agent.fsNames = append(agent.fsNames, fs)
			}

			// Add to fsStats if not already present
			if agent.fsStats[fs] == nil {
				agent.fsStats[fs] = &system.FsStats{
					Mountpoint: fs,
					Root:       false,
				}
			}
		}
	}

	// Apply environment variables
	for key, value := range cm.config.Environment {
		if err := os.Setenv(key, value); err != nil {
			slog.Warn("Failed to set environment variable", "key", key, "error", err)
		}
	}

	// Apply other configuration settings as environment variables
	if cm.config.DataDir != "" {
		os.Setenv("DATA_DIR", cm.config.DataDir)
	}
	if cm.config.DockerHost != "" {
		os.Setenv("DOCKER_HOST", cm.config.DockerHost)
	}
	if cm.config.Filesystem != "" {
		os.Setenv("FILESYSTEM", cm.config.Filesystem)
	}
	if cm.config.Listen != "" {
		os.Setenv("LISTEN", cm.config.Listen)
	}
	if cm.config.Network != "" {
		os.Setenv("NETWORK", cm.config.Network)
	}
	if cm.config.Nics != "" {
		os.Setenv("NICS", cm.config.Nics)
	}
	if cm.config.PrimarySensor != "" {
		os.Setenv("PRIMARY_SENSOR", cm.config.PrimarySensor)
	}
	if cm.config.Sensors != "" {
		os.Setenv("SENSORS", cm.config.Sensors)
	}
	if cm.config.SysSensors != "" {
		os.Setenv("SYS_SENSORS", cm.config.SysSensors)
	}

	slog.Info("Applied configuration from hub")
	return nil
}

// GetConfig returns the current configuration
func (cm *ConfigManager) GetConfig() *AgentConfig {
	return cm.config
}

// GetLastUpdate returns when the configuration was last updated
func (cm *ConfigManager) GetLastUpdate() time.Time {
	return cm.lastUpdate
}
