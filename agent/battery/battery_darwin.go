//go:build darwin

package battery

import (
	"os/exec"

	"howett.net/plist"
)

type macBattery struct {
	CurrentCapacity   int  `plist:"CurrentCapacity"`
	MaxCapacity       int  `plist:"MaxCapacity"`
	FullyCharged      bool `plist:"FullyCharged"`
	IsCharging        bool `plist:"IsCharging"`
	ExternalConnected bool `plist:"ExternalConnected"`
}

func readMacBatteries() ([]macBattery, error) {
	out, err := exec.Command("ioreg", "-n", "AppleSmartBattery", "-r", "-a").Output()
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	var batteries []macBattery
	if _, err := plist.Unmarshal(out, &batteries); err != nil {
		return nil, err
	}
	return batteries, nil
}

func HasReadableBattery() bool {
	batteries, _ := GetBatteryStats()
	return len(batteries) > 0
}

// GetBatteryStats returns every readable battery reported by macOS.
func GetBatteryStats() ([]Battery, error) {
	batteries, err := readMacBatteries()
	if err != nil {
		return nil, err
	}
	if len(batteries) == 0 {
		return nil, errNoBatteries
	}
	result := make([]Battery, 0, len(batteries))
	for _, bat := range batteries {
		if bat.MaxCapacity <= 0 {
			// skip ghost batteries with 0 capacity
			// https://github.com/distatus/battery/issues/34
			continue
		}
		percent := min(max(float64(bat.CurrentCapacity)/float64(bat.MaxCapacity)*100, 0), 100)
		state := stateUnknown
		switch {
		case !bat.ExternalConnected:
			state = stateDischarging
		case bat.IsCharging:
			state = stateCharging
		case bat.CurrentCapacity == 0:
			state = stateEmpty
		case !bat.FullyCharged:
			state = stateIdle
		default:
			state = stateFull
		}
		result = append(result, Battery{Name: "Primary", Percent: uint8(percent), State: state,
			FullChargeCapacity: uint64(bat.MaxCapacity), HasFullChargeCapacity: true, System: true})
	}
	if len(result) == 0 {
		return nil, errNoBatteries
	}
	return normalizeBatteries(result), nil
}
