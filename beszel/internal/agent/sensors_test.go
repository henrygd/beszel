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
			name:       "No sensors configured",
			sensorName: "any_temp",
			config: &SensorConfig{
				sensors:        map[string]struct{}{},
				isBlacklist:    false,
				hasWildcards:   false,
				skipCollection: false,
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
		skipCollection bool
		expectedConfig *SensorConfig
	}{
		{
			name:          "Empty configuration",
			primarySensor: "",
			sysSensors:    "",
			sensors:       "",
			expectedConfig: &SensorConfig{
				context:        context.Background(),
				primarySensor:  "",
				sensors:        map[string]struct{}{},
				isBlacklist:    false,
				hasWildcards:   false,
				skipCollection: false,
			},
		},
		{
			name:           "Explicitly set to empty string",
			primarySensor:  "",
			sysSensors:     "",
			sensors:        "",
			skipCollection: true,
			expectedConfig: &SensorConfig{
				context:        context.Background(),
				primarySensor:  "",
				sensors:        map[string]struct{}{},
				isBlacklist:    false,
				hasWildcards:   false,
				skipCollection: true,
			},
		},
		{
			name:          "Primary sensor only - should create sensor map",
			primarySensor: "cpu_temp",
			sysSensors:    "",
			sensors:       "",
			expectedConfig: &SensorConfig{
				context:       context.Background(),
				primarySensor: "cpu_temp",
				sensors:       map[string]struct{}{},
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
			name:          "Sensors with whitespace",
			primarySensor: "cpu_temp",
			sysSensors:    "",
			sensors:       "cpu_*, gpu_temp",
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
			result := agent.newSensorConfigWithEnv(tt.primarySensor, tt.sysSensors, tt.sensors, tt.skipCollection)

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

func TestScaleTemperature(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected float64
		desc     string
	}{
		// Normal temperatures (no scaling needed)
		{"normal_cpu_temp", 45.0, 45.0, "Normal CPU temperature"},
		{"normal_room_temp", 25.0, 25.0, "Normal room temperature"},
		{"high_cpu_temp", 85.0, 85.0, "High CPU temperature"},
		// Zero temperature
		{"zero_temp", 0.0, 0.0, "Zero temperature"},
		// Fractional values that should use 100x scaling
		{"fractional_45c", 0.45, 45.0, "0.45 should become 45°C (100x)"},
		{"fractional_25c", 0.25, 25.0, "0.25 should become 25°C (100x)"},
		{"fractional_60c", 0.60, 60.0, "0.60 should become 60°C (100x)"},
		{"fractional_75c", 0.75, 75.0, "0.75 should become 75°C (100x)"},
		{"fractional_30c", 0.30, 30.0, "0.30 should become 30°C (100x)"},
		// Fractional values that should use 1000x scaling
		{"millifractional_45c", 0.045, 45.0, "0.045 should become 45°C (1000x)"},
		{"millifractional_25c", 0.025, 25.0, "0.025 should become 25°C (1000x)"},
		{"millifractional_60c", 0.060, 60.0, "0.060 should become 60°C (1000x)"},
		{"millifractional_75c", 0.075, 75.0, "0.075 should become 75°C (1000x)"},
		{"millifractional_35c", 0.035, 35.0, "0.035 should become 35°C (1000x)"},
		// Edge cases - values outside reasonable range
		{"very_low_fractional", 0.01, 1.0, "0.01 should default to 100x scaling (1°C)"},
		{"very_high_fractional", 0.99, 99.0, "0.99 should default to 100x scaling (99°C)"},
		{"extremely_low", 0.001, 0.1, "0.001 should default to 100x scaling (0.1°C)"},
		// Boundary cases around the reasonable range (15-95°C)
		{"boundary_low_100x", 0.15, 15.0, "0.15 should use 100x scaling (15°C)"},
		{"boundary_high_100x", 0.95, 95.0, "0.95 should use 100x scaling (95°C)"},
		{"boundary_low_1000x", 0.015, 15.0, "0.015 should use 1000x scaling (15°C)"},
		{"boundary_high_1000x", 0.095, 95.0, "0.095 should use 1000x scaling (95°C)"},
		// Values just outside reasonable range
		{"just_below_range_100x", 0.14, 14.0, "0.14 should default to 100x (14°C)"},
		{"just_above_range_100x", 0.96, 96.0, "0.96 should default to 100x (96°C)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := scaleTemperature(tt.input)
			assert.InDelta(t, tt.expected, result, 0.001,
				"scaleTemperature(%v) = %v, expected %v (%s)",
				tt.input, result, tt.expected, tt.desc)
		})
	}
}

func TestScaleTemperatureLogic(t *testing.T) {
	// Test the logic flow for ambiguous cases
	t.Run("prefers_100x_when_both_valid", func(t *testing.T) {
		// 0.5 could be 50°C (100x) or 500°C (1000x)
		// Should prefer 100x since it's tried first and is in range
		result := scaleTemperature(0.5)
		expected := 50.0
		assert.InDelta(t, expected, result, 0.001,
			"scaleTemperature(0.5) = %v, expected %v (should prefer 100x scaling)",
			result, expected)
	})

	t.Run("uses_1000x_when_100x_too_low", func(t *testing.T) {
		// 0.05 -> 5°C (100x, too low) or 50°C (1000x, in range)
		// Should use 1000x since 100x is below reasonable range
		result := scaleTemperature(0.05)
		expected := 50.0
		assert.InDelta(t, expected, result, 0.001,
			"scaleTemperature(0.05) = %v, expected %v (should use 1000x scaling)",
			result, expected)
	})

	t.Run("defaults_to_100x_when_both_invalid", func(t *testing.T) {
		// 0.005 -> 0.5°C (100x, too low) or 5°C (1000x, too low)
		// Should default to 100x scaling
		result := scaleTemperature(0.005)
		expected := 0.5
		assert.InDelta(t, expected, result, 0.001,
			"scaleTemperature(0.005) = %v, expected %v (should default to 100x)",
			result, expected)
	})
}
