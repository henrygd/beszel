package agent

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/system"
)

type fanSensor struct {
	key, path string
}

var getFanSensors = newFanSensorCache(hwmonRoot)

func newFanSensorCache(root string) func() ([]fanSensor, error) {
	return sync.OnceValues(func() ([]fanSensor, error) {
		return discoverHwmonFans(root)
	})
}

// updateFans populates systemStats.Fans from the host's hwmon sysfs tree.
// No-op on platforms where hwmon isn't available (see fans_other.go).
func (a *Agent) updateFans(systemStats *system.Stats) {
	if hwmonRoot == "" {
		return
	}
	sensors, err := getFanSensors()
	if err != nil {
		slog.Debug("Error reading fans", "err", err)
		return
	}
	fans := readFanSensors(sensors)
	if len(fans) == 0 {
		return
	}
	systemStats.Fans = fans
	// Note: Commented out because we don't currently use this value in the UI.
	// Compute the single "dashboard" value used by the FanSpeed alert.
	// Per-sensor RPMs live in Stats.Fans and drive the multi-line FanChart
	// in the UI; the alert path only needs one number to compare against
	// the user's threshold, so we use the highest RPM across all fans
	// a.systemInfo.DashboardFan = 0
	// for _, rpm := range fans {
	// 	if rpm > a.systemInfo.DashboardFan {
	// 		a.systemInfo.DashboardFan = rpm
	// 	}
	// }
}

// readHwmonFans walks the given hwmon root (typically /sys/class/hwmon) and
// returns a map of "<chip>_<label-or-fan-idx>" → RPM for every fan*_input
// file it finds. Zero RPM is retained because it can represent a real fan that
// has stopped; negative and malformed readings are ignored.
func readHwmonFans(root string) (map[string]uint16, error) {
	sensors, err := discoverHwmonFans(root)
	if err != nil {
		return nil, err
	}
	return readFanSensors(sensors), nil
}

func discoverHwmonFans(root string) ([]fanSensor, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var sensors []fanSensor
	for _, entry := range entries {
		chipDir := filepath.Join(root, entry.Name())
		sensorDir := chipDir
		inputs, _ := filepath.Glob(filepath.Join(sensorDir, "fan*_input"))

		// Some legacy hwmon drivers (notably applesmc) register a hwmon class
		// device but create fan attributes on the parent platform device. In
		// sysfs that parent is exposed through hwmonN/device.
		if len(inputs) == 0 {
			deviceDir := filepath.Join(chipDir, "device")
			if deviceInputs, _ := filepath.Glob(filepath.Join(deviceDir, "fan*_input")); len(deviceInputs) > 0 {
				sensorDir = deviceDir
				inputs = deviceInputs
			}
		}

		chipName := utils.ReadStringFile(filepath.Join(sensorDir, "name"))
		if chipName == "" {
			chipName = utils.ReadStringFile(filepath.Join(chipDir, "name"))
		}
		if chipName == "" {
			chipName = entry.Name()
		}
		for _, inputPath := range inputs {
			base := strings.TrimSuffix(filepath.Base(inputPath), "_input")
			label := utils.ReadStringFile(filepath.Join(sensorDir, base+"_label"))
			key := chipName + "_" + base
			if label != "" {
				key = chipName + "_" + label
			}
			sensors = append(sensors, fanSensor{key, inputPath})
		}
	}
	return sensors, nil
}

func readFanSensors(sensors []fanSensor) map[string]uint16 {
	fans := make(map[string]uint16, len(sensors))
	for _, sensor := range sensors {
		if rpm, ok := utils.ReadUintFile(sensor.path); ok {
			fans[sensor.key] = uint16(rpm)
		}
	}
	return fans
}
