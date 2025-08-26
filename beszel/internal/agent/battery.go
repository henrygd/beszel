package agent

import "github.com/distatus/battery"

// getBatteryStats returns the current battery percent and charge state
func getBatteryStats() (batteryPercent uint8, batteryState uint8, err error) {
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
