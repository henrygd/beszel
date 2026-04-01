//go:build !freebsd && !darwin && !linux && !windows

// Package battery provides functions to check if the system has a battery and to get the battery stats.
package battery

import "errors"

func HasReadableBattery() bool {
	return false
}

func GetBatteryStats() (uint8, uint8, error) {
	return 0, 0, errors.ErrUnsupported
}
