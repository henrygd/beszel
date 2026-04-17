//go:build linux

package battery

import (
	"errors"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/henrygd/beszel/agent/utils"
)

// getBatteryPaths returns the paths of all batteries in /sys/class/power_supply
var getBatteryPaths func() ([]string, error)

// HasReadableBattery checks if the system has a battery and returns true if it does.
var HasReadableBattery func() bool

func init() {
	resetBatteryState("/sys/class/power_supply")
}

// resetBatteryState resets the sync.Once functions to a fresh state.
// Tests call this after swapping sysfsPowerSupply so the new path is picked up.
func resetBatteryState(sysfsPowerSupplyPath string) {
	getBatteryPaths = sync.OnceValues(func() ([]string, error) {
		entries, err := os.ReadDir(sysfsPowerSupplyPath)
		if err != nil {
			return nil, err
		}
		var paths []string
		for _, e := range entries {
			path := filepath.Join(sysfsPowerSupplyPath, e.Name())
			if utils.ReadStringFile(filepath.Join(path, "type")) == "Battery" {
				paths = append(paths, path)
			}
		}
		return paths, nil
	})
	HasReadableBattery = sync.OnceValue(func() bool {
		systemHasBattery := false
		paths, err := getBatteryPaths()
		for _, path := range paths {
			if _, ok := utils.ReadStringFileOK(filepath.Join(path, "capacity")); ok {
				systemHasBattery = true
				break
			}
		}
		if !systemHasBattery {
			slog.Debug("No battery found", "err", err)
		}
		return systemHasBattery
	})
}

func parseSysfsState(status string) uint8 {
	switch status {
	case "Empty":
		return stateEmpty
	case "Full":
		return stateFull
	case "Charging":
		return stateCharging
	case "Discharging":
		return stateDischarging
	case "Not charging":
		return stateIdle
	default:
		return stateUnknown
	}
}

// GetBatteryStats returns the current battery percent and charge state.
// Reads /sys/class/power_supply/*/capacity directly so the kernel-reported
// value is used, which is always 0-100 and matches what the OS displays.
func GetBatteryStats() (batteryPercent uint8, batteryState uint8, err error) {
	if !HasReadableBattery() {
		return batteryPercent, batteryState, errors.ErrUnsupported
	}
	paths, err := getBatteryPaths()
	if err != nil {
		return batteryPercent, batteryState, err
	}
	if len(paths) == 0 {
		return batteryPercent, batteryState, errors.New("no batteries")
	}

	batteryState = math.MaxUint8
	totalPercent := 0
	count := 0

	for _, path := range paths {
		capStr, ok := utils.ReadStringFileOK(filepath.Join(path, "capacity"))
		if !ok {
			continue
		}
		cap, parseErr := strconv.Atoi(capStr)
		if parseErr != nil {
			continue
		}
		totalPercent += cap
		count++

		state := parseSysfsState(utils.ReadStringFile(filepath.Join(path, "status")))
		if state != stateUnknown {
			batteryState = state
		}
	}

	if count == 0 || batteryState == math.MaxUint8 {
		return batteryPercent, batteryState, errors.New("no battery capacity")
	}

	batteryPercent = uint8(totalPercent / count)
	return batteryPercent, batteryState, nil
}
