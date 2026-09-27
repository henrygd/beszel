//go:build linux

package battery

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/henrygd/beszel/agent/utils"
)

var batteryRoot = "/sys/class/power_supply"

// HasReadableBattery reports whether collection currently finds a readable battery.
func HasReadableBattery() bool {
	batteries, _ := GetBatteryStats()
	return len(batteries) > 0
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

// GetBatteryStats re-enumerates power supplies and returns every readable battery.
func GetBatteryStats() ([]Battery, error) {
	entries, err := os.ReadDir(batteryRoot)
	if err != nil {
		return nil, err
	}
	batteries := make([]Battery, 0, len(entries))
	for _, entry := range entries {
		path := filepath.Join(batteryRoot, entry.Name())
		if utils.ReadStringFile(filepath.Join(path, "type")) != "Battery" {
			continue
		}
		capStr, ok := utils.ReadStringFileOK(filepath.Join(path, "capacity"))
		if !ok {
			continue
		}
		cap, parseErr := strconv.Atoi(capStr)
		if parseErr != nil {
			continue
		}
		cap = min(max(cap, 0), 100)
		name := utils.ReadStringFile(filepath.Join(path, "model_name"))
		if name == "" {
			name = utils.ReadStringFile(filepath.Join(path, "model"))
		}
		if name == "" {
			name = entry.Name()
		}
		battery := Battery{
			Name:    name,
			Percent: uint8(cap),
			State:   parseSysfsState(utils.ReadStringFile(filepath.Join(path, "status"))),
			System:  utils.ReadStringFile(filepath.Join(path, "scope")) != "Device",
		}
		for _, fullName := range []string{"charge_full", "energy_full"} {
			if parsed, ok := utils.ReadUintFile(filepath.Join(path, fullName)); ok && parsed > 0 {
				battery.FullChargeCapacity = parsed
				battery.HasFullChargeCapacity = true
				break
			}
		}
		batteries = append(batteries, battery)
	}
	if len(batteries) == 0 {
		return nil, errNoBatteries
	}
	return normalizeBatteries(batteries), nil
}
