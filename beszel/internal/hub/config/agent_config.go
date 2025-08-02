// Package config provides functions for agent configuration management
package config

import (
	"encoding/json"
	"net/http"
	"time"

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
	Version       string            `json:"version"`
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
		Version:     "1.0.0", // TODO: Get from beszel.Version
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
		}
	}

	// Set response headers
	e.Response.Header().Set("Content-Type", "application/json")
	e.Response.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	e.Response.Header().Set("Pragma", "no-cache")
	e.Response.Header().Set("Expires", "0")

	return e.JSON(http.StatusOK, config)
}
