package agent

import "github.com/distatus/battery"

// getBatteryStats returns the current battery percent and charge state
func getBatteryStats() (batteryPercent uint8, batteryState uint8, err error) {
	batteries, err := battery.GetAll()
	if err != nil {
		return batteryPercent, batteryState, err
	}
	batteriesTotalCapacity := float64(0)
	batteriesTotalCharge := float64(0)
	for _, bat := range batteries {
		full := bat.Design
		if full == 0 {
			full = bat.Full
		}
		batteriesTotalCapacity += full
		batteriesTotalCharge += bat.Current
	}
	batteryPercent = uint8(batteriesTotalCharge / batteriesTotalCapacity * 100)
	batteryState = uint8(batteries[0].State.Raw)
	return batteryPercent, batteryState, nil
}
