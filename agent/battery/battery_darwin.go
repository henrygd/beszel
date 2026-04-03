//go:build darwin

package battery

import (
	"errors"
	"log/slog"
	"math"
	"os/exec"
	"sync"

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

// HasReadableBattery checks if the system has a battery and returns true if it does.
var HasReadableBattery = sync.OnceValue(func() bool {
	systemHasBattery := false
	batteries, err := readMacBatteries()
	slog.Debug("Batteries", "batteries", batteries, "err", err)
	for _, bat := range batteries {
		if bat.MaxCapacity > 0 {
			systemHasBattery = true
			break
		}
	}
	return systemHasBattery
})

// GetBatteryStats returns the current battery percent and charge state.
// Uses CurrentCapacity/MaxCapacity to match the value macOS displays.
func GetBatteryStats() (batteryPercent uint8, batteryState uint8, err error) {
	if !HasReadableBattery() {
		return batteryPercent, batteryState, errors.ErrUnsupported
	}
	batteries, err := readMacBatteries()
	if len(batteries) == 0 {
		return batteryPercent, batteryState, errors.New("no batteries")
	}

	totalCapacity := 0
	totalCharge := 0
	batteryState = math.MaxUint8

	for _, bat := range batteries {
		if bat.MaxCapacity == 0 {
			// skip ghost batteries with 0 capacity
			// https://github.com/distatus/battery/issues/34
			continue
		}
		totalCapacity += bat.MaxCapacity
		totalCharge += min(bat.CurrentCapacity, bat.MaxCapacity)

		switch {
		case !bat.ExternalConnected:
			batteryState = stateDischarging
		case bat.IsCharging:
			batteryState = stateCharging
		case bat.CurrentCapacity == 0:
			batteryState = stateEmpty
		case !bat.FullyCharged:
			batteryState = stateIdle
		default:
			batteryState = stateFull
		}
	}

	if totalCapacity == 0 || batteryState == math.MaxUint8 {
		return batteryPercent, batteryState, errors.New("no battery capacity")
	}

	batteryPercent = uint8(float64(totalCharge) / float64(totalCapacity) * 100)
	return batteryPercent, batteryState, nil
}
