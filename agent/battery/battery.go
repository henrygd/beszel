//go:build !freebsd

// Package battery provides functions to check if the system has a battery and to get the battery stats.
package battery

import (
	"errors"
	"log/slog"
	"math"

	"github.com/distatus/battery"
)

var (
	systemHasBattery   = false
	haveCheckedBattery = false
)

// HasReadableBattery checks if the system has a battery and returns true if it does.
func HasReadableBattery() bool {
	if haveCheckedBattery {
		return systemHasBattery
	}
	haveCheckedBattery = true
	batteries, err := battery.GetAll()
	for _, bat := range batteries {
		if bat != nil && (bat.Full > 0 || bat.Design > 0) {
			systemHasBattery = true
			break
		}
	}
	if !systemHasBattery {
		slog.Debug("No battery found", "err", err)
	}
	return systemHasBattery
}

// GetBatteryStats returns the current battery percent and charge state
// percent = (current charge of all batteries) / (sum of designed/full capacity of all batteries)
func GetBatteryStats() (batteryPercent uint8, batteryState uint8, err error) {
	if !HasReadableBattery() {
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

	batteryState = math.MaxUint8

	for i, bat := range batteries {
		if partialErrs && errs[i] != nil {
			// if there were some errors, like missing data, skip it
			continue
		}
		if bat == nil || bat.Full == 0 {
			// skip batteries with no capacity. Charge is unlikely to ever be zero, but
			// we can't guarantee that, so don't skip based on charge.
			continue
		}
		totalCapacity += bat.Full
		totalCharge += bat.Current
		if bat.State.Raw >= 0 {
			batteryState = uint8(bat.State.Raw)
		}
	}

	if totalCapacity == 0 || batteryState == math.MaxUint8 {
		// for macs there's sometimes a ghost battery with 0 capacity
		// https://github.com/distatus/battery/issues/34
		// Instead of skipping over those batteries, we'll check for total 0 capacity
		// and return an error. This also prevents a divide by zero.
		return batteryPercent, batteryState, errors.New("no battery capacity")
	}

	batteryPercent = uint8(totalCharge / totalCapacity * 100)
	return batteryPercent, batteryState, nil
}
