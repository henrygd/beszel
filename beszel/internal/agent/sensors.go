package agent

import (
	"beszel/internal/entities/system"
	"context"
	"log/slog"
	"path"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v4/common"
	"github.com/shirou/gopsutil/v4/sensors"
)

type SensorConfig struct {
	context       context.Context
	sensors       map[string]struct{}
	primarySensor string
	isBlacklist   bool
	hasWildcards  bool
}

func (a *Agent) newSensorConfig() *SensorConfig {
	primarySensor, _ := GetEnv("PRIMARY_SENSOR")
	sysSensors, _ := GetEnv("SYS_SENSORS")
	sensors, _ := GetEnv("SENSORS")

	return a.newSensorConfigWithEnv(primarySensor, sysSensors, sensors)
}

// newSensorConfigWithEnv creates a SensorConfig with the provided environment variables
func (a *Agent) newSensorConfigWithEnv(primarySensor, sysSensors, sensors string) *SensorConfig {
	config := &SensorConfig{
		context:       context.Background(),
		primarySensor: primarySensor,
	}

	// Set sensors context (allows overriding sys location for sensors)
	if sysSensors != "" {
		slog.Info("SYS_SENSORS", "path", sysSensors)
		config.context = context.WithValue(config.context,
			common.EnvKey, common.EnvMap{common.HostSysEnvKey: sysSensors},
		)
	}

	// Set sensors whitelist
	if sensors != "" {
		// handle blacklist
		if strings.HasPrefix(sensors, "-") {
			config.isBlacklist = true
			sensors = sensors[1:]
		}

		config.sensors = make(map[string]struct{})
		for sensor := range strings.SplitSeq(sensors, ",") {
			sensor = strings.TrimSpace(sensor)
			if sensor != "" {
				config.sensors[sensor] = struct{}{}
				if strings.Contains(sensor, "*") {
					config.hasWildcards = true
				}
			}
		}
	}

	return config
}

// updateTemperatures updates the agent with the latest sensor temperatures
func (a *Agent) updateTemperatures(systemStats *system.Stats) {
	// skip if sensors whitelist is set to empty string
	if a.sensorConfig.sensors != nil && len(a.sensorConfig.sensors) == 0 {
		slog.Debug("Skipping temperature collection")
		return
	}

	// reset high temp
	a.systemInfo.DashboardTemp = 0

	// get sensor data
	temps, _ := sensors.TemperaturesWithContext(a.sensorConfig.context)
	slog.Debug("Temperature", "sensors", temps)

	// return if no sensors
	if len(temps) == 0 {
		return
	}

	systemStats.Temperatures = make(map[string]float64, len(temps))
	for i, sensor := range temps {
		// skip if temperature is unreasonable
		if sensor.Temperature <= 0 || sensor.Temperature >= 200 {
			continue
		}
		sensorName := sensor.SensorKey
		if _, ok := systemStats.Temperatures[sensorName]; ok {
			// if key already exists, append int to key
			sensorName = sensorName + "_" + strconv.Itoa(i)
		}
		// skip if not in whitelist or blacklist
		if !isValidSensor(sensorName, a.sensorConfig) {
			continue
		}
		// set dashboard temperature
		if a.sensorConfig.primarySensor == "" {
			a.systemInfo.DashboardTemp = max(a.systemInfo.DashboardTemp, sensor.Temperature)
		} else if a.sensorConfig.primarySensor == sensorName {
			a.systemInfo.DashboardTemp = sensor.Temperature
		}
		systemStats.Temperatures[sensorName] = twoDecimals(sensor.Temperature)
	}
}

// isValidSensor checks if a sensor is valid based on the sensor name and the sensor config
func isValidSensor(sensorName string, config *SensorConfig) bool {
	// If no sensors configuration, everything is valid
	if config.sensors == nil {
		return true
	}

	// Exact match - return true if whitelist, false if blacklist
	if _, exactMatch := config.sensors[sensorName]; exactMatch {
		return !config.isBlacklist
	}

	// If no wildcards, return false if blacklist, true if whitelist
	if !config.hasWildcards {
		return config.isBlacklist
	}

	// Check for wildcard patterns
	for pattern := range config.sensors {
		if !strings.Contains(pattern, "*") {
			continue
		}
		if match, _ := path.Match(pattern, sensorName); match {
			return !config.isBlacklist
		}
	}

	return config.isBlacklist
}
