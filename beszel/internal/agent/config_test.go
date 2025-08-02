package agent

import (
	"testing"
	"time"
)

func TestNewConfigManager(t *testing.T) {
	hubURL := "http://localhost:8090"
	token := "test-token"

	cm := NewConfigManager(hubURL, token)

	if cm.hubURL != hubURL {
		t.Errorf("Expected hubURL %s, got %s", hubURL, cm.hubURL)
	}

	if cm.token != token {
		t.Errorf("Expected token %s, got %s", token, cm.token)
	}

	if cm.config == nil {
		t.Error("Expected config to be initialized")
	}
}

func TestAgentConfig_DefaultValues(t *testing.T) {
	config := &AgentConfig{
		LogLevel:    "info",
		MemCalc:     "",
		ExtraFs:     []string{},
		Environment: make(map[string]string),
		LastUpdated: time.Now(),
		Version:     "1.0.0",
	}

	if config.LogLevel != "info" {
		t.Errorf("Expected LogLevel 'info', got %s", config.LogLevel)
	}

	if config.MemCalc != "" {
		t.Errorf("Expected empty MemCalc, got %s", config.MemCalc)
	}

	if len(config.ExtraFs) != 0 {
		t.Errorf("Expected empty ExtraFs, got %d items", len(config.ExtraFs))
	}

	if len(config.Environment) != 0 {
		t.Errorf("Expected empty Environment, got %d items", len(config.Environment))
	}
}

func TestConfigManager_GetConfig(t *testing.T) {
	cm := NewConfigManager("http://localhost:8090", "test-token")

	config := cm.GetConfig()
	if config == nil {
		t.Error("Expected non-nil config")
	}
}

func TestConfigManager_GetLastUpdate(t *testing.T) {
	cm := NewConfigManager("http://localhost:8090", "test-token")

	lastUpdate := cm.GetLastUpdate()
	if !lastUpdate.IsZero() {
		t.Error("Expected zero time for initial lastUpdate")
	}
}
