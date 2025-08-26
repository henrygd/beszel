// Package agent handles configuration management for the agent
package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"beszel/internal/common"
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
	Version       uint64            `json:"version"`
}

// UnmarshalJSON provides custom JSON unmarshaling to handle version field that might be stored as string
func (c *AgentConfig) UnmarshalJSON(data []byte) error {
	// Create a temporary struct with Version as interface{} to handle both string and uint64
	type Alias AgentConfig
	aux := &struct {
		Version interface{} `json:"version"`
		*Alias
	}{
		Alias: (*Alias)(c),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// Handle version field conversion
	switch v := aux.Version.(type) {
	case string:
		if parsed, err := strconv.ParseUint(v, 10, 64); err == nil {
			c.Version = parsed
		}
	case float64:
		c.Version = uint64(v)
	case uint64:
		c.Version = v
	default:
		// If version is not present or invalid, use 0
		c.Version = 0
	}

	return nil
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

// GetCurrentVersion returns the current configuration version
func (cm *ConfigManager) GetCurrentVersion() uint64 {
	if cm.config == nil {
		return 0
	}
	return cm.config.Version
}

// ApplyWebSocketConfigUpdate applies configuration from WebSocket push and returns if restart is needed
func (cm *ConfigManager) ApplyWebSocketConfigUpdate(configUpdate common.ConfigUpdateRequest, agent *Agent) (bool, error) {
	// Check if this is a newer version
	if configUpdate.Version <= cm.GetCurrentVersion() {
		slog.Debug("Received config update with same or older version", "received", configUpdate.Version, "current", cm.GetCurrentVersion())
		return false, nil
	}

	slog.Info("Applying WebSocket configuration update", "version", configUpdate.Version, "current_version", cm.GetCurrentVersion())

	// Convert ConfigUpdateRequest to AgentConfig
	newConfig := &AgentConfig{
		LogLevel:      configUpdate.LogLevel,
		MemCalc:       configUpdate.MemCalc,
		ExtraFs:       configUpdate.ExtraFs,
		DataDir:       configUpdate.DataDir,
		DockerHost:    configUpdate.DockerHost,
		Filesystem:    configUpdate.Filesystem,
		Listen:        configUpdate.Listen,
		Network:       configUpdate.Network,
		Nics:          configUpdate.Nics,
		PrimarySensor: configUpdate.PrimarySensor,
		Sensors:       configUpdate.Sensors,
		SysSensors:    configUpdate.SysSensors,
		Environment:   configUpdate.Environment,
		Version:       configUpdate.Version,
		LastUpdated:   time.Now(),
	}

	// Store the old config for comparison
	oldConfig := cm.config
	cm.config = newConfig
	cm.lastUpdate = time.Now()

	// Apply the new configuration
	if err := cm.ApplyConfig(agent); err != nil {
		slog.Error("Failed to apply WebSocket config update", "error", err)
		// Restore old config on failure
		cm.config = oldConfig
		return false, err
	}

	// Check if restart is needed by comparing critical settings
	restartNeeded := cm.needsRestart(oldConfig, newConfig) || configUpdate.ForceRestart

	slog.Info("Successfully applied WebSocket configuration update", "restart_needed", restartNeeded)
	return restartNeeded, nil
}

// needsRestart determines if the agent needs to restart based on configuration changes
func (cm *ConfigManager) needsRestart(oldConfig, newConfig *AgentConfig) bool {
	if oldConfig == nil {
		return false
	}

	// Critical settings that require restart
	restartFields := []struct {
		old, new string
	}{
		{oldConfig.Listen, newConfig.Listen},
		{oldConfig.DataDir, newConfig.DataDir},
		{oldConfig.DockerHost, newConfig.DockerHost},
		{oldConfig.Filesystem, newConfig.Filesystem},
		{oldConfig.Network, newConfig.Network},
		{oldConfig.Nics, newConfig.Nics},
	}

	for _, field := range restartFields {
		if field.old != field.new {
			slog.Debug("Restart needed due to configuration change", "old", field.old, "new", field.new)
			return true
		}
	}

	// Check if extra filesystems changed
	if len(oldConfig.ExtraFs) != len(newConfig.ExtraFs) {
		return true
	}
	for i, fs := range oldConfig.ExtraFs {
		if i >= len(newConfig.ExtraFs) || fs != newConfig.ExtraFs[i] {
			return true
		}
	}

	return false
}
