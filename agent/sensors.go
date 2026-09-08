package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/system"

	"github.com/shirou/gopsutil/v4/common"
	"github.com/shirou/gopsutil/v4/sensors"
)

var errTemperatureFetchTimeout = errors.New("temperature collection timed out")

// Matches sensors.TemperaturesWithContext to allow for panic recovery (gopsutil/issues/1832)
type getTempsFn func(ctx context.Context) ([]sensors.TemperatureStat, error)

type SensorConfig struct {
	context        context.Context
	sensors        map[string]struct{}
	primarySensor  string
	timeout        time.Duration
	isBlacklist    bool
	hasWildcards   bool
	skipCollection bool
	skipGPU        bool
	sensorShadow   string
	firstRun       bool
}

func (a *Agent) newSensorConfig() *SensorConfig {
	primarySensor, _ := utils.GetEnv("PRIMARY_SENSOR")
	sysSensors, _ := utils.GetEnv("SYS_SENSORS")
	sensorsEnvVal, sensorsSet := utils.GetEnv("SENSORS")
	skipCollection := sensorsSet && sensorsEnvVal == ""
	sensorsTimeout, _ := utils.GetEnv("SENSORS_TIMEOUT")
	skipGPU, _ := utils.GetEnv("SKIP_GPU")

	return a.newSensorConfigWithEnv(primarySensor, sysSensors, sensorsEnvVal, sensorsTimeout, skipCollection, skipGPU == "true")
}

// newSensorConfigWithEnv creates a SensorConfig with the provided environment variables
// sensorsSet indicates if the SENSORS environment variable was explicitly set (even to empty string)
func (a *Agent) newSensorConfigWithEnv(primarySensor, sysSensors, sensorsEnvVal, sensorsTimeout string, skipCollection, skipGPU bool) *SensorConfig {
	timeout := 2 * time.Second
	if sensorsTimeout != "" {
		if d, err := time.ParseDuration(sensorsTimeout); err == nil {
			timeout = d
		} else {
			slog.Warn("Invalid SENSORS_TIMEOUT", "value", sensorsTimeout)
		}
	}

	config := &SensorConfig{
		context:        context.Background(),
		primarySensor:  primarySensor,
		timeout:        timeout,
		skipCollection: skipCollection,
		skipGPU:        skipGPU,
		firstRun:       true,
		sensors:        make(map[string]struct{}),
	}

	// Set sensors context (allows overriding sys location for sensors)
	if sysSensors != "" {
		slog.Info("SYS_SENSORS", "path", sysSensors)
		config.context = context.WithValue(config.context,
			common.EnvKey, common.EnvMap{common.HostSysEnvKey: sysSensors},
		)
	}
	if skipGPU && runtime.GOOS == "linux" {
		// gopsutil reads every temp*_input before results can be filtered, so
		// point it at a shadow tree built from the effective sysfs root instead.
		if shadow, err := buildNonGpuSysShadow(effectiveSysRoot(config.context)); err == nil {
			slog.Info("SKIP_GPU enabled, using non-GPU sensor sysfs shadow", "path", shadow)
			config.sensorShadow = shadow
			config.context = context.WithValue(config.context,
				common.EnvKey, common.EnvMap{common.HostSysEnvKey: shadow},
			)
		} else {
			slog.Warn("SKIP_GPU sensor shadow unavailable, falling back to post-read filtering", "err", err)
		}
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

	temps, err := a.getTempsWithTimeout(getSensorTemps)
	if err != nil {
		// retry once on panic (gopsutil/issues/1832)
		if !errors.Is(err, errTemperatureFetchTimeout) {
			temps, err = a.getTempsWithTimeout(getSensorTemps)
		}
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
		if a.sensorConfig.skipGPU && isGpuSensorKey(sensorName) {
			continue
		}
		// set dashboard temperature
		switch a.sensorConfig.primarySensor {
		case "":
			a.systemInfo.DashboardTemp = max(a.systemInfo.DashboardTemp, sensor.Temperature)
		case sensorName:
			a.systemInfo.DashboardTemp = sensor.Temperature
		}
		systemStats.Temperatures[sensorName] = utils.TwoDecimals(sensor.Temperature)
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

func (a *Agent) getTempsWithTimeout(getTemps getTempsFn) ([]sensors.TemperatureStat, error) {
	type result struct {
		temps []sensors.TemperatureStat
		err   error
	}

	// Use a longer timeout on the first run to allow for initialization
	// (e.g. Windows LHM subprocess startup)
	timeout := a.sensorConfig.timeout
	if a.sensorConfig.firstRun {
		a.sensorConfig.firstRun = false
		timeout = 10 * time.Second
	}

	resultCh := make(chan result, 1)
	go func() {
		temps, err := a.getTempsWithPanicRecovery(getTemps)
		resultCh <- result{temps: temps, err: err}
	}()

	select {
	case res := <-resultCh:
		return res.temps, res.err
	case <-time.After(timeout):
		return nil, errTemperatureFetchTimeout
	}
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

// effectiveSysRoot mirrors gopsutil's HostSys lookup, which lives in its
// internal package: context override, then HOST_SYS env, then /sys.
func effectiveSysRoot(ctx context.Context) string {
	if envMap, ok := ctx.Value(common.EnvKey).(common.EnvMap); ok {
		if v := envMap[common.HostSysEnvKey]; v != "" {
			return v
		}
	}
	if v := os.Getenv("HOST_SYS"); v != "" {
		return v
	}
	return "/sys"
}

func (config *SensorConfig) cleanupSensorShadow() {
	if config.sensorShadow == "" {
		return
	}
	if err := os.RemoveAll(config.sensorShadow); err != nil {
		slog.Warn("Error removing sensor sysfs shadow", "path", config.sensorShadow, "err", err)
		return
	}
	config.sensorShadow = ""
}

func (a *Agent) cleanupSensorShadow() {
	if a.sensorConfig != nil {
		a.sensorConfig.cleanupSensorShadow()
	}
}

func isGpuThermalZone(zoneType string) bool {
	zoneType = strings.ToLower(strings.TrimSpace(zoneType))
	return isGpuChipName(zoneType) || strings.Contains(zoneType, "gpu")
}

// buildNonGpuSysShadow links non-GPU sensor directories into a temp dir. Only
// static chip names and thermal-zone types are read; no sensor values are touched.
func buildNonGpuSysShadow(sysRoot string) (string, error) {
	shadow, err := os.MkdirTemp("", "beszel-sensors-*")
	if err != nil {
		return "", err
	}
	shadowHwmon := filepath.Join(shadow, "class", "hwmon")
	if err := os.MkdirAll(shadowHwmon, 0o755); err != nil {
		os.RemoveAll(shadow)
		return "", err
	}
	entries, err := os.ReadDir(filepath.Join(sysRoot, "class", "hwmon"))
	if err != nil && !os.IsNotExist(err) {
		os.RemoveAll(shadow)
		return "", err
	}
	for _, entry := range entries {
		chipDir := filepath.Join(sysRoot, "class", "hwmon", entry.Name())
		if name, ok := utils.ReadStringFileOK(filepath.Join(chipDir, "name")); !ok || isGpuChipName(name) {
			continue
		}
		if err := os.Symlink(chipDir, filepath.Join(shadowHwmon, entry.Name())); err != nil {
			os.RemoveAll(shadow)
			return "", err
		}
	}

	thermalEntries, err := os.ReadDir(filepath.Join(sysRoot, "class", "thermal"))
	if err != nil {
		if os.IsNotExist(err) {
			return shadow, nil
		}
		os.RemoveAll(shadow)
		return "", err
	}
	shadowThermal := filepath.Join(shadow, "class", "thermal")
	if err := os.MkdirAll(shadowThermal, 0o755); err != nil {
		os.RemoveAll(shadow)
		return "", err
	}
	for _, entry := range thermalEntries {
		if !strings.HasPrefix(entry.Name(), "thermal_zone") {
			continue
		}
		zoneDir := filepath.Join(sysRoot, "class", "thermal", entry.Name())
		zoneType, ok := utils.ReadStringFileOK(filepath.Join(zoneDir, "type"))
		if !ok || isGpuThermalZone(zoneType) {
			continue
		}
		if err := os.Symlink(zoneDir, filepath.Join(shadowThermal, entry.Name())); err != nil {
			os.RemoveAll(shadow)
			return "", err
		}
	}
	return shadow, nil
}
