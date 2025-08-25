package agent

import "github.com/distatus/battery"

// getBatteryStats returns the current battery percent and charge state
func getBatteryStats() (batteryPercent uint8, batteryState uint8, err error) {
	bat, err := battery.Get(0)
	if err != nil {
		return batteryPercent, batteryState, err
	}
	full := bat.Design
	if full == 0 {
		full = bat.Full
	}
	batteryPercent = uint8(bat.Current / full * 100)
	batteryState = uint8(bat.State.Raw)
	return batteryPercent, batteryState, nil
}
