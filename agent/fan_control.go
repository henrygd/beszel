package agent

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/system"
)

var getFanControlSensors = sync.OnceValues(func() ([]fanSensor, error) {
	return discoverHwmonFanControls(hwmonRoot)
})

func (a *Agent) updateFanControls(stats *system.Stats) {
	sensors, err := getFanControlSensors()
	if err != nil {
		slog.Debug("Error reading fan controls", "err", err)
		return
	}
	if a.sensorConfig != nil && a.sensorConfig.skipGPU {
		sensors = filterGpuFans(sensors)
	}
	stats.FanControl = readFanControls(sensors)
}

// discoverHwmonFanControls discovers control channels independently of RPM
// inputs. Linux exposes both PWM and GPIO fan controls through pwmN (0-255).
func discoverHwmonFanControls(root string) ([]fanSensor, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var sensors []fanSensor
	keyCounts := make(map[string]int)
	for _, entry := range entries {
		chipDir := filepath.Join(root, entry.Name())
		sensorDir := chipDir
		inputs := fanControlPaths(sensorDir)
		if len(inputs) == 0 {
			sensorDir = filepath.Join(chipDir, "device")
			inputs = fanControlPaths(sensorDir)
		}
		chipName := utils.ReadStringFile(filepath.Join(sensorDir, "name"))
		if chipName == "" {
			chipName = utils.ReadStringFile(filepath.Join(chipDir, "name"))
		}
		if chipName == "" {
			chipName = entry.Name()
		}
		for _, input := range inputs {
			// A pwmN channel need not control fanN, so do not borrow RPM labels.
			key := chipName + "_" + filepath.Base(input)
			keyCounts[key]++
			sensors = append(sensors, fanSensor{key: key, path: input, chip: chipName})
		}
	}
	// Identically named chips must not silently overwrite each other's data.
	for i := range sensors {
		if keyCounts[sensors[i].key] > 1 {
			dir := filepath.Dir(sensors[i].path)
			if filepath.Base(dir) == "device" {
				dir = filepath.Dir(dir)
			}
			sensors[i].key += "_" + filepath.Base(dir)
		}
	}
	return sensors, nil
}

func fanControlPaths(dir string) []string {
	candidates, _ := filepath.Glob(filepath.Join(dir, "pwm*"))
	inputs := make([]string, 0, len(candidates))
	for _, path := range candidates {
		index := strings.TrimPrefix(filepath.Base(path), "pwm")
		n, err := strconv.Atoi(index)
		// Exclude pwmN_enable, pwmN_mode, pwmN_freq and other attributes.
		if err == nil && n > 0 && strconv.Itoa(n) == index {
			inputs = append(inputs, path)
		}
	}
	return inputs
}

func readFanControls(sensors []fanSensor) map[string]float64 {
	var levels map[string]float64
	for _, sensor := range sensors {
		value, ok := utils.ReadUintFile(sensor.path)
		if !ok || value > 255 {
			continue
		}
		if levels == nil {
			levels = make(map[string]float64)
		}
		// Zero is a valid control level, not an unavailable reading. This is
		// the driver's reported control setting, not measured fan rotation.
		levels[sensor.key] = utils.TwoDecimals(float64(value) * 100 / 255)
	}
	return levels
}
