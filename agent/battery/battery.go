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
var hasReportedBatteryError []bool = []

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
// percent = (current charge of all batteries) / (sum of designed/full capacity of all batteries)
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
	errs, partialErrs := err.(battery.Errors)

	for i, bat := range batteries {
		if partialErrs && errs[i] != nil {
			if ! hasReportedBatteryError[i] {
				fmt.Fprintf(os.Stderr, "Error getting info for BAT%d: %s\n", i, errs[i])
				hasReportedBatteryError[i] = true
				continue
			}
		}
		hasReportedBatteryError[i] = true
		// we don't need to nil check here because we skip batteries with incomplete stats
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
