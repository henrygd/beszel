package systems

import (
	"encoding/json"
	"testing"
	"time"

	"beszel/internal/hub/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetConfigForSystem(t *testing.T) {
	sm, teardown := createTestSystemManager(t)
	defer teardown()

	// Create a test system record
	systemRecord := createTestSystemRecord(sm.hub, "test-system")
	testConfig := `{"log_level": "debug", "mem_calc": "total-available"}`
	systemRecord.Set("agent_config", testConfig)
	require.NoError(t, sm.hub.SaveNoValidate(systemRecord))

	// Test getting config for system
	agentConfig, err := config.GetAgentConfigForSystem(sm.hub, systemRecord.Id)
	require.NoError(t, err, "Should be able to get agent config")
	
	assert.Equal(t, "debug", agentConfig.LogLevel, "Log level should match")
	assert.Equal(t, "total-available", agentConfig.MemCalc, "MemCalc should match")
	assert.Greater(t, agentConfig.Version, uint64(0), "Version should be set")
}

func TestGetConfigSyncStatus(t *testing.T) {
	sm, teardown := createTestSystemManager(t)
	defer teardown()

	// Create test systems
	system1 := createTestSystemRecord(sm.hub, "test-system-1")
	system1.Set("agent_config", `{"log_level": "info"}`)
	require.NoError(t, sm.hub.SaveNoValidate(system1))

	// Add system to manager (this would normally be done by the event hooks)
	testSys := sm.NewSystem(system1.Id)
	testSys.Status = "up"
	require.NoError(t, sm.AddSystem(testSys))

	// Get sync status
	status := sm.GetConfigSyncStatus()
	
	assert.Contains(t, status, system1.Id, "Status should contain our test system")
	
	sysInfo := status[system1.Id]
	assert.Equal(t, system1.Id, sysInfo.SystemID)
	assert.Equal(t, "up", sysInfo.Status)
	assert.False(t, sysInfo.IsConnected, "System should not be connected via WebSocket")
	assert.False(t, sysInfo.SupportsSync, "System should not support sync without WebSocket")
	assert.Nil(t, sysInfo.LastConfigPush, "No config push should have happened yet")
}

func TestAgentConfigVersionIncrement(t *testing.T) {
	// Test that config versions are properly incremented
	config1 := &config.AgentConfig{
		LogLevel: "info",
		Version:  123,
	}

	config1.IncrementVersion()
	
	// Version should be updated to current timestamp
	assert.Greater(t, config1.Version, uint64(123), "Version should be incremented")
	assert.InDelta(t, time.Now().Unix(), int64(config1.Version), 2, "Version should be close to current timestamp")
}

func TestConfigUpdateRequestConversion(t *testing.T) {
	// Test conversion from AgentConfig to ConfigUpdateRequest
	originalConfig := &config.AgentConfig{
		LogLevel:   "debug",
		MemCalc:    "total-available", 
		ExtraFs:    []string{"/home", "/var"},
		DataDir:    "/opt/beszel",
		Environment: map[string]string{"TEST": "value"},
		Version:    456,
	}

	configUpdate := originalConfig.ToConfigUpdateRequest()

	assert.Equal(t, originalConfig.LogLevel, configUpdate.LogLevel)
	assert.Equal(t, originalConfig.MemCalc, configUpdate.MemCalc)
	assert.Equal(t, originalConfig.ExtraFs, configUpdate.ExtraFs)
	assert.Equal(t, originalConfig.DataDir, configUpdate.DataDir)
	assert.Equal(t, originalConfig.Environment, configUpdate.Environment)
	assert.Equal(t, originalConfig.Version, configUpdate.Version)
	assert.False(t, configUpdate.ForceRestart, "ForceRestart should default to false")
}

