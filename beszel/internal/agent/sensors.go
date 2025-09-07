package agent

import (
	"context"
	"fmt"
	"log/slog"
	"path"
	"runtime"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/henrygd/beszel/internal/entities/system"

	"github.com/shirou/gopsutil/v4/common"
	"github.com/shirou/gopsutil/v4/sensors"
)

type SensorConfig struct {
	context        context.Context
	sensors        map[string]struct{}
	primarySensor  string
	isBlacklist    bool
	hasWildcards   bool
	skipCollection bool
}

func (a *Agent) newSensorConfig() *SensorConfig {
	primarySensor, _ := GetEnv("PRIMARY_SENSOR")
	sysSensors, _ := GetEnv("SYS_SENSORS")
	sensorsEnvVal, sensorsSet := GetEnv("SENSORS")
	skipCollection := sensorsSet && sensorsEnvVal == ""

	return a.newSensorConfigWithEnv(primarySensor, sysSensors, sensorsEnvVal, skipCollection)
}

// Matches sensors.TemperaturesWithContext to allow for panic recovery (gopsutil/issues/1832)
type getTempsFn func(ctx context.Context) ([]sensors.TemperatureStat, error)

// newSensorConfigWithEnv creates a SensorConfig with the provided environment variables
// sensorsSet indicates if the SENSORS environment variable was explicitly set (even to empty string)
func (a *Agent) newSensorConfigWithEnv(primarySensor, sysSensors, sensorsEnvVal string, skipCollection bool) *SensorConfig {
	config := &SensorConfig{
		context:        context.Background(),
		primarySensor:  primarySensor,
		skipCollection: skipCollection,
		sensors:        make(map[string]struct{}),
	}

	// Set sensors context (allows overriding sys location for sensors)
	if sysSensors != "" {
		slog.Info("SYS_SENSORS", "path", sysSensors)
		config.context = context.WithValue(config.context,
			common.EnvKey, common.EnvMap{common.HostSysEnvKey: sysSensors},
		)
	}

	// handle blacklist
	if strings.HasPrefix(sensorsEnvVal, "-") {
		config.isBlacklist = true
		sensorsEnvVal = sensorsEnvVal[1:]
	}

	for sensor := range strings.SplitSeq(sensorsEnvVal, ",") {
		sensor = strings.TrimSpace(sensor)
		if sensor != "" {
			config.sensors[sensor] = struct{}{}
			if strings.Contains(sensor, "*") {
				config.hasWildcards = true
			}
		}
	}

	return config
}

// updateTemperatures updates the agent with the latest sensor temperatures
func (a *Agent) updateTemperatures(systemStats *system.Stats) {
	// skip if sensors whitelist is set to empty string
	if a.sensorConfig.skipCollection {
		slog.Debug("Skipping temperature collection")
		return
	}

	// reset high temp
	a.systemInfo.DashboardTemp = 0

	temps, err := a.getTempsWithPanicRecovery(getSensorTemps)
	if err != nil {
		// retry once on panic (gopsutil/issues/1832)
		temps, err = a.getTempsWithPanicRecovery(getSensorTemps)
		if err != nil {
			slog.Warn("Error updating temperatures", "err", err)
			if len(systemStats.Temperatures) > 0 {
				systemStats.Temperatures = make(map[string]float64)
			}
			return
		}
	}
	slog.Debug("Temperature", "sensors", temps)

	// return if no sensors
	if len(temps) == 0 {
		return
	}

	systemStats.Temperatures = make(map[string]float64, len(temps))
	for i, sensor := range temps {
		// check for malformed strings on darwin (gopsutil/issues/1832)
		if runtime.GOOS == "darwin" && !utf8.ValidString(sensor.SensorKey) {
			continue
		}

		// scale temperature
		if sensor.Temperature != 0 && sensor.Temperature < 1 {
			sensor.Temperature = scaleTemperature(sensor.Temperature)
		}
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
		switch a.sensorConfig.primarySensor {
		case "":
			a.systemInfo.DashboardTemp = max(a.systemInfo.DashboardTemp, sensor.Temperature)
		case sensorName:
			a.systemInfo.DashboardTemp = sensor.Temperature
		}
		systemStats.Temperatures[sensorName] = twoDecimals(sensor.Temperature)
	}
}

// getTempsWithPanicRecovery wraps sensors.TemperaturesWithContext to recover from panics (gopsutil/issues/1832)
func (a *Agent) getTempsWithPanicRecovery(getTemps getTempsFn) (temps []sensors.TemperatureStat, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	// get sensor data (error ignored intentionally as it may be only with one sensor)
	temps, _ = getTemps(a.sensorConfig.context)
	return
}

// isValidSensor checks if a sensor is valid based on the sensor name and the sensor config
func isValidSensor(sensorName string, config *SensorConfig) bool {
	// if no sensors configured, everything is valid
	if len(config.sensors) == 0 {
		return true
	}

	// Exact match - return true if whitelist, false if blacklist
	if _, exactMatch := config.sensors[sensorName]; exactMatch {
		return !config.isBlacklist
	}

	// If no wildcards, return true if blacklist, false if whitelist
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

// scaleTemperature scales temperatures in fractional values to reasonable Celsius values
func scaleTemperature(temp float64) float64 {
	if temp > 1 {
		return temp
	}
	scaled100 := temp * 100
	scaled1000 := temp * 1000

	if scaled100 >= 15 && scaled100 <= 95 {
		return scaled100
	} else if scaled1000 >= 15 && scaled1000 <= 95 {
		return scaled1000
	}
	return scaled100
}
