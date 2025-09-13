//go:build freebsd

package battery

import "errors"

func HasReadableBattery() bool {
	return false
}

func GetBatteryStats() (uint8, uint8, error) {
	return 0, 0, errors.ErrUnsupported
}
