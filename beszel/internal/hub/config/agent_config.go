// Package config provides functions for agent configuration management
package config

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"beszel/internal/common"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// AgentConfig represents the configuration that can be pulled by agents
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
		// If version is not present or invalid, use current timestamp
		c.Version = uint64(time.Now().Unix())
	}

	return nil
}

// GetAgentConfig returns the agent configuration as JSON
func GetAgentConfig(e *core.RequestEvent) error {
	// Check if the request has a valid token
	token := e.Request.Header.Get("Authorization")
	if token == "" {
		token = e.Request.URL.Query().Get("token")
	}

	if token == "" {
		return apis.NewForbiddenError("No token provided", nil)
	}

	// Remove "Bearer " prefix if present
	if len(token) > 7 && token[:7] == "Bearer " {
		token = token[7:]
	}

	// Validate token by checking if it exists in fingerprints
	fingerprint, err := e.App.FindFirstRecordByFilter("fingerprints", "token = {:token}", map[string]any{"token": token})
	if err != nil {
		return apis.NewForbiddenError("Invalid token", nil)
	}

	// Get the system associated with this token
	systemID := fingerprint.GetString("system")
	system, err := e.App.FindRecordById("systems", systemID)
	if err != nil {
		return apis.NewNotFoundError("System not found", nil)
	}

	// Create agent configuration
	config := AgentConfig{
		LogLevel:    "info", // Default log level
		MemCalc:     "",     // Default memory calculation
		ExtraFs:     []string{},
		Environment: make(map[string]string),
		LastUpdated: time.Now(),
		Version:     uint64(time.Now().Unix()), // Unix timestamp as version
	}

	// Check if there's a specific configuration for this system
	// This could be stored in a separate collection or as metadata
	systemConfig := system.GetString("agent_config")
	if systemConfig != "" {
		var customConfig AgentConfig
		if err := json.Unmarshal([]byte(systemConfig), &customConfig); err == nil {
			// Merge custom config with defaults
			if customConfig.LogLevel != "" {
				config.LogLevel = customConfig.LogLevel
			}
			if customConfig.MemCalc != "" {
				config.MemCalc = customConfig.MemCalc
			}
			if len(customConfig.ExtraFs) > 0 {
				config.ExtraFs = customConfig.ExtraFs
			}
			if len(customConfig.Environment) > 0 {
				config.Environment = customConfig.Environment
			}
			// Preserve version if it exists and is valid
			if customConfig.Version > 0 {
				config.Version = customConfig.Version
			}
		}
	}

	// Set response headers
	e.Response.Header().Set("Content-Type", "application/json")
	e.Response.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	e.Response.Header().Set("Pragma", "no-cache")
	e.Response.Header().Set("Expires", "0")

	return e.JSON(http.StatusOK, config)
}

// ToConfigUpdateRequest converts AgentConfig to ConfigUpdateRequest for WebSocket push
func (c *AgentConfig) ToConfigUpdateRequest() common.ConfigUpdateRequest {
	return common.ConfigUpdateRequest{
		LogLevel:      c.LogLevel,
		MemCalc:       c.MemCalc,
		ExtraFs:       c.ExtraFs,
		DataDir:       c.DataDir,
		DockerHost:    c.DockerHost,
		Filesystem:    c.Filesystem,
		Listen:        c.Listen,
		Network:       c.Network,
		Nics:          c.Nics,
		PrimarySensor: c.PrimarySensor,
		Sensors:       c.Sensors,
		SysSensors:    c.SysSensors,
		Environment:   c.Environment,
		Version:       c.Version,
		ForceRestart:  false, // Can be set based on what changed
	}
}

// IncrementVersion creates a new version number based on current timestamp
func (c *AgentConfig) IncrementVersion() {
	c.Version = uint64(time.Now().Unix())
	c.LastUpdated = time.Now()
}

// GetAgentConfigForSystem retrieves configuration for a specific system ID
func GetAgentConfigForSystem(app core.App, systemID string) (*AgentConfig, error) {
	system, err := app.FindRecordById("systems", systemID)
	if err != nil {
		return nil, err
	}

	// Create default configuration
	config := &AgentConfig{
		LogLevel:    "info",
		MemCalc:     "",
		ExtraFs:     []string{},
		Environment: make(map[string]string),
		LastUpdated: time.Now(),
		Version:     uint64(time.Now().Unix()),
	}

	// Apply system-specific configuration if it exists
	systemConfig := system.GetString("agent_config")
	if systemConfig != "" {
		var customConfig AgentConfig
		if err := json.Unmarshal([]byte(systemConfig), &customConfig); err == nil {
			// Merge custom config with defaults
			if customConfig.LogLevel != "" {
				config.LogLevel = customConfig.LogLevel
			}
			if customConfig.MemCalc != "" {
				config.MemCalc = customConfig.MemCalc
			}
			if len(customConfig.ExtraFs) > 0 {
				config.ExtraFs = customConfig.ExtraFs
			}
			if len(customConfig.Environment) > 0 {
				config.Environment = customConfig.Environment
			}
			// Preserve version if it exists
			if customConfig.Version > 0 {
				config.Version = customConfig.Version
			}
		}
	}

	return config, nil
}

// UpdateSystemConfig updates the configuration for a specific system and increments version
func UpdateSystemConfig(app core.App, systemID string, newConfig *AgentConfig) error {
	system, err := app.FindRecordById("systems", systemID)
	if err != nil {
		return err
	}

	// Increment version for the new configuration
	newConfig.IncrementVersion()

	// Convert to JSON and save
	configJSON, err := json.Marshal(newConfig)
	if err != nil {
		return err
	}

	system.Set("agent_config", string(configJSON))
	return app.Save(system)
}
