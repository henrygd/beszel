//go:build !freebsd

// Package battery provides functions to check if the system has a battery and to get the battery stats.
package battery

import (
	"errors"
	"fmt"
	"os"
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
	batteries,err := battery.GetAll()
	if err != nil {
		// even if there's errors getting some batteries, the system
		// definitely has a battery if the list is not empty.
		// This list will include everything `battery` can find,
		// including things like bluetooth devices.
		fmt.Fprintln(os.Stderr, err)
	}
	systemHasBattery = len(batteries) > 0
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
	// we'll handle errors later by skipping batteries with errors, rather
	// than skipping everything because of the presence of some errors.
	if len(batteries) == 0 {
		return batteryPercent, batteryState, errors.New("no batteries")
	}

	totalCapacity := float64(0)
	totalCharge := float64(0)
	errs, partialErrs := err.(battery.Errors)

	for i, bat := range batteries {
		if partialErrs && errs[i] != nil {
			// if there were some errors, like missing data, skip it
			continue
		}
		totalCapacity += bat.Full
		totalCharge += bat.Current
	}
	batteryPercent = uint8(totalCharge / totalCapacity * 100)
	batteryState = uint8(batteries[0].State.Raw)
	return batteryPercent, batteryState, nil
}
