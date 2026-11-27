package agent

import (
	"log/slog"
	"math"
	"time"

	"github.com/henrygd/beszel/agent/ipmi"
	"github.com/henrygd/beszel/agent/utils"
	"github.com/shirou/gopsutil/v4/sensors"
)

// ipmiSource returns the latest BMC readings, waiting up to wait for a refresh.
type ipmiSource interface {
	Get(wait time.Duration) ipmi.Readings
}

// newIPMISource opens the local BMC when IPMI=true. BMC sensors (inlet and
// board temperatures, chassis fans) are often not exposed through hwmon.
func newIPMISource() ipmiSource {
	if enabled, _ := utils.GetEnv("IPMI"); enabled != "true" {
		return nil
	}
	collector, err := ipmi.Open()
	if err != nil {
		slog.Warn("IPMI sensors unavailable", "err", err)
		return nil
	}
	slog.Info("IPMI sensors enabled")
	// Start reading the SDR repository now; the first scan can take a few seconds.
	collector.Get(0)
	return collector
}

// ipmiReadings returns the latest BMC readings, or nothing if IPMI is disabled.
func (config *SensorConfig) ipmiReadings() ipmi.Readings {
	if config == nil || config.ipmi == nil {
		return ipmi.Readings{}
	}
	return config.ipmi.Get(config.timeout)
}

// ipmiTemps returns BMC temperatures in the form returned by gopsutil, so they
// pass through the same naming, filtering and dashboard logic.
func (config *SensorConfig) ipmiTemps() []sensors.TemperatureStat {
	readings := config.ipmiReadings().Temperatures
	if len(readings) == 0 {
		return nil
	}
	temps := make([]sensors.TemperatureStat, 0, len(readings))
	for _, r := range readings {
		temps = append(temps, sensors.TemperatureStat{SensorKey: r.Name, Temperature: r.Value})
	}
	return temps
}

// addIPMIFans merges BMC fan speeds allowed by FAN_SENSORS into fans.
func (config *SensorConfig) addIPMIFans(fans map[string]uint16) map[string]uint16 {
	for _, r := range config.ipmiReadings().Fans {
		if !config.fanSensors.allows(r.Name) {
			continue
		}
		if fans == nil {
			fans = make(map[string]uint16)
		}
		if _, exists := fans[r.Name]; exists {
			continue // keep the host reading when a hwmon fan has the same key
		}
		fans[r.Name] = uint16(min(math.Round(max(r.Value, 0)), math.MaxUint16))
	}
	return fans
}
