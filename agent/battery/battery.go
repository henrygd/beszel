//go:build !freebsd

// Package battery provides functions to check if the system has a battery and to get the battery stats.
package battery

import (
	"errors"
	"log/slog"

	"github.com/distatus/battery"
)

var systemHasBattery = false
var haveCheckedBattery = false

// HasReadableBattery checks if the system has a battery and returns true if it does.
func HasReadableBattery() bool {
	if haveCheckedBattery {
		return systemHasBattery
	}
	haveCheckedBattery = true
	bat, err := battery.Get(0)
	systemHasBattery = err == nil && bat != nil && bat.Design != 0 && bat.Full != 0
	if !systemHasBattery {
		slog.Debug("No battery found", "err", err)
	}
	return systemHasBattery
}

// GetBatteryStats returns the current battery percent and charge state
func GetBatteryStats() (batteryPercent uint8, batteryState uint8, err error) {
	if !systemHasBattery {
		return batteryPercent, batteryState, errors.ErrUnsupported
	}
	batteries, err := battery.GetAll()
	if err != nil || len(batteries) == 0 {
		return batteryPercent, batteryState, err
	}
	totalCapacity := float64(0)
	totalCharge := float64(0)
	for _, bat := range batteries {
		if bat.Design != 0 {
			totalCapacity += bat.Design
		} else {
			totalCapacity += bat.Full
		}
		totalCharge += bat.Current
	}
	batteryPercent = uint8(totalCharge / totalCapacity * 100)
	batteryState = uint8(batteries[0].State.Raw)
	return batteryPercent, batteryState, nil
}
