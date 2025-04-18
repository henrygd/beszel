//go:build testing
// +build testing

package agent

import (
	"context"
	"os"
	"testing"

	"github.com/shirou/gopsutil/v4/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsValidSensor(t *testing.T) {
	tests := []struct {
		name          string
		sensorName    string
		config        *SensorConfig
		expectedValid bool
	}{
		{
			name:       "Whitelist - sensor in list",
			sensorName: "cpu_temp",
			config: &SensorConfig{
				sensors:     map[string]struct{}{"cpu_temp": {}},
				isBlacklist: false,
			},
			expectedValid: true,
		},
		{
			name:       "Whitelist - sensor not in list",
			sensorName: "gpu_temp",
			config: &SensorConfig{
				sensors:     map[string]struct{}{"cpu_temp": {}},
				isBlacklist: false,
			},
			expectedValid: false,
		},
		{
			name:       "Blacklist - sensor in list",
			sensorName: "cpu_temp",
			config: &SensorConfig{
				sensors:     map[string]struct{}{"cpu_temp": {}},
				isBlacklist: true,
			},
			expectedValid: false,
		},
		{
			name:       "Blacklist - sensor not in list",
			sensorName: "gpu_temp",
			config: &SensorConfig{
				sensors:     map[string]struct{}{"cpu_temp": {}},
				isBlacklist: true,
			},
			expectedValid: true,
		},
		{
			name:       "Whitelist with wildcard - matching pattern",
			sensorName: "core_0_temp",
			config: &SensorConfig{
				sensors:      map[string]struct{}{"core_*_temp": {}},
				isBlacklist:  false,
				hasWildcards: true,
			},
			expectedValid: true,
		},
		{
			name:       "Whitelist with wildcard - non-matching pattern",
			sensorName: "gpu_temp",
			config: &SensorConfig{
				sensors:      map[string]struct{}{"core_*_temp": {}},
				isBlacklist:  false,
				hasWildcards: true,
			},
			expectedValid: false,
		},
		{
			name:       "Blacklist with wildcard - matching pattern",
			sensorName: "core_0_temp",
			config: &SensorConfig{
				sensors:      map[string]struct{}{"core_*_temp": {}},
				isBlacklist:  true,
				hasWildcards: true,
			},
			expectedValid: false,
		},
		{
			name:       "Blacklist with wildcard - non-matching pattern",
			sensorName: "gpu_temp",
			config: &SensorConfig{
				sensors:      map[string]struct{}{"core_*_temp": {}},
				isBlacklist:  true,
				hasWildcards: true,
			},
			expectedValid: true,
		},
		{
			name:       "Nil sensor config",
			sensorName: "any_temp",
			config: &SensorConfig{
				sensors: nil,
			},
			expectedValid: true,
		},
		{
			name:       "Mixed patterns in whitelist - exact match",
			sensorName: "cpu_temp",
			config: &SensorConfig{
				sensors:      map[string]struct{}{"cpu_temp": {}, "core_*_temp": {}},
				isBlacklist:  false,
				hasWildcards: true,
			},
			expectedValid: true,
		},
		{
			name:       "Mixed patterns in whitelist - wildcard match",
			sensorName: "core_1_temp",
			config: &SensorConfig{
				sensors:      map[string]struct{}{"cpu_temp": {}, "core_*_temp": {}},
				isBlacklist:  false,
				hasWildcards: true,
			},
			expectedValid: true,
		},
		{
			name:       "Mixed patterns in blacklist - exact match",
			sensorName: "cpu_temp",
			config: &SensorConfig{
				sensors:      map[string]struct{}{"cpu_temp": {}, "core_*_temp": {}},
				isBlacklist:  true,
				hasWildcards: true,
			},
			expectedValid: false,
		},
		{
			name:       "Mixed patterns in blacklist - wildcard match",
			sensorName: "core_1_temp",
			config: &SensorConfig{
				sensors:      map[string]struct{}{"cpu_temp": {}, "core_*_temp": {}},
				isBlacklist:  true,
				hasWildcards: true,
			},
			expectedValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidSensor(tt.sensorName, tt.config)
			assert.Equal(t, tt.expectedValid, result, "isValidSensor(%q, config) returned unexpected result", tt.sensorName)
		})
	}
}

func TestNewSensorConfigWithEnv(t *testing.T) {
	agent := &Agent{}

	tests := []struct {
		name           string
		primarySensor  string
		sysSensors     string
		sensors        string
		expectedConfig *SensorConfig
	}{
		{
			name:          "Empty configuration",
			primarySensor: "",
			sysSensors:    "",
			sensors:       "",
			expectedConfig: &SensorConfig{
				context:       context.Background(),
				primarySensor: "",
				sensors:       nil,
				isBlacklist:   false,
				hasWildcards:  false,
			},
		},
		{
			name:          "Primary sensor only",
			primarySensor: "cpu_temp",
			sysSensors:    "",
			sensors:       "",
			expectedConfig: &SensorConfig{
				context:       context.Background(),
				primarySensor: "cpu_temp",
				sensors:       nil,
				isBlacklist:   false,
				hasWildcards:  false,
			},
		},
		{
			name:          "Whitelist sensors",
			primarySensor: "cpu_temp",
			sysSensors:    "",
			sensors:       "cpu_temp,gpu_temp",
			expectedConfig: &SensorConfig{
				context:       context.Background(),
				primarySensor: "cpu_temp",
				sensors: map[string]struct{}{
					"cpu_temp": {},
					"gpu_temp": {},
				},
				isBlacklist:  false,
				hasWildcards: false,
			},
		},
		{
			name:          "Blacklist sensors",
			primarySensor: "cpu_temp",
			sysSensors:    "",
			sensors:       "-cpu_temp,gpu_temp",
			expectedConfig: &SensorConfig{
				context:       context.Background(),
				primarySensor: "cpu_temp",
				sensors: map[string]struct{}{
					"cpu_temp": {},
					"gpu_temp": {},
				},
				isBlacklist:  true,
				hasWildcards: false,
			},
		},
		{
			name:          "Sensors with wildcard",
			primarySensor: "cpu_temp",
			sysSensors:    "",
			sensors:       "cpu_*,gpu_temp",
			expectedConfig: &SensorConfig{
				context:       context.Background(),
				primarySensor: "cpu_temp",
				sensors: map[string]struct{}{
					"cpu_*":    {},
					"gpu_temp": {},
				},
				isBlacklist:  false,
				hasWildcards: true,
			},
		},
		{
			name:          "With SYS_SENSORS path",
			primarySensor: "cpu_temp",
			sysSensors:    "/custom/path",
			sensors:       "cpu_temp",
			expectedConfig: &SensorConfig{
				primarySensor: "cpu_temp",
				sensors: map[string]struct{}{
					"cpu_temp": {},
				},
				isBlacklist:  false,
				hasWildcards: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := agent.newSensorConfigWithEnv(tt.primarySensor, tt.sysSensors, tt.sensors)

			// Check primary sensor
			assert.Equal(t, tt.expectedConfig.primarySensor, result.primarySensor)

			// Check sensor map
			if tt.expectedConfig.sensors == nil {
				assert.Nil(t, result.sensors)
			} else {
				assert.Equal(t, len(tt.expectedConfig.sensors), len(result.sensors))
				for sensor := range tt.expectedConfig.sensors {
					_, exists := result.sensors[sensor]
					assert.True(t, exists, "Sensor %s should exist in the result", sensor)
				}
			}

			// Check flags
			assert.Equal(t, tt.expectedConfig.isBlacklist, result.isBlacklist)
			assert.Equal(t, tt.expectedConfig.hasWildcards, result.hasWildcards)

			// Check context
			if tt.sysSensors != "" {
				// Verify context contains correct values
				envMap, ok := result.context.Value(common.EnvKey).(common.EnvMap)
				require.True(t, ok, "Context should contain EnvMap")
				sysPath, ok := envMap[common.HostSysEnvKey]
				require.True(t, ok, "EnvMap should contain HostSysEnvKey")
				assert.Equal(t, tt.sysSensors, sysPath)
			}
		})
	}
}

func TestNewSensorConfig(t *testing.T) {
	// Save original environment variables
	originalPrimary, hasPrimary := os.LookupEnv("BESZEL_AGENT_PRIMARY_SENSOR")
	originalSys, hasSys := os.LookupEnv("BESZEL_AGENT_SYS_SENSORS")
	originalSensors, hasSensors := os.LookupEnv("BESZEL_AGENT_SENSORS")

	// Restore environment variables after the test
	defer func() {
		// Clean up test environment variables
		os.Unsetenv("BESZEL_AGENT_PRIMARY_SENSOR")
		os.Unsetenv("BESZEL_AGENT_SYS_SENSORS")
		os.Unsetenv("BESZEL_AGENT_SENSORS")

		// Restore original values if they existed
		if hasPrimary {
			os.Setenv("BESZEL_AGENT_PRIMARY_SENSOR", originalPrimary)
		}
		if hasSys {
			os.Setenv("BESZEL_AGENT_SYS_SENSORS", originalSys)
		}
		if hasSensors {
			os.Setenv("BESZEL_AGENT_SENSORS", originalSensors)
		}
	}()

	// Set test environment variables
	os.Setenv("BESZEL_AGENT_PRIMARY_SENSOR", "test_primary")
	os.Setenv("BESZEL_AGENT_SYS_SENSORS", "/test/path")
	os.Setenv("BESZEL_AGENT_SENSORS", "test_sensor1,test_*,test_sensor3")

	agent := &Agent{}
	result := agent.newSensorConfig()

	// Verify results
	assert.Equal(t, "test_primary", result.primarySensor)
	assert.NotNil(t, result.sensors)
	assert.Equal(t, 3, len(result.sensors))
	assert.True(t, result.hasWildcards)
	assert.False(t, result.isBlacklist)

	// Check that sys sensors path is in context
	envMap, ok := result.context.Value(common.EnvKey).(common.EnvMap)
	require.True(t, ok, "Context should contain EnvMap")
	sysPath, ok := envMap[common.HostSysEnvKey]
	require.True(t, ok, "EnvMap should contain HostSysEnvKey")
	assert.Equal(t, "/test/path", sysPath)
}
