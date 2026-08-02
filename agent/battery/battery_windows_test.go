//go:build testing && windows

package battery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Windows BATTERY_STATUS.PowerState bit values (Poclass.h), documented at
// https://learn.microsoft.com/en-us/windows/win32/power/battery-status-str
const (
	batteryPowerOnLine = 0x00000001 // on AC power - says nothing about charge level
	batteryDischarging = 0x00000002
	batteryCharging    = 0x00000004
	batteryCritical    = 0x00000008
)

func TestReadWinBatteryStateChargingTakesPriority(t *testing.T) {
	// Charging while also on AC power (the normal case while plugged in and
	// topping up) must report Charging, not Full or Idle.
	state := readWinBatteryState(batteryCharging|batteryPowerOnLine, 50, 100)
	assert.Equal(t, stateCharging, state)
}

func TestReadWinBatteryStateCriticalMapsToEmpty(t *testing.T) {
	state := readWinBatteryState(batteryCritical, 1, 100)
	assert.Equal(t, stateEmpty, state)
}

func TestReadWinBatteryStateDischarging(t *testing.T) {
	state := readWinBatteryState(batteryDischarging, 60, 100)
	assert.Equal(t, stateDischarging, state)
}

func TestReadWinBatteryStateOnACAtFullCapacityIsFull(t *testing.T) {
	state := readWinBatteryState(batteryPowerOnLine, 100, 100)
	assert.Equal(t, stateFull, state)
}

// This is the bug: BATTERY_POWER_ON_LINE means "on AC power", not "battery
// full". A laptop with an OEM charge limit (e.g. capped at 80% to preserve
// battery health) reports exactly this PowerState - plugged in, not actively
// charging because it's already at its cap, not discharging, not critical -
// while sitting well below 100%. That must not be reported as Full.
func TestReadWinBatteryStateOnACBelowFullCapacityIsIdleNotFull(t *testing.T) {
	state := readWinBatteryState(batteryPowerOnLine, 80, 100)
	assert.Equal(t, stateIdle, state, "on AC power below full capacity should be Idle, not Full")
	assert.NotEqual(t, stateFull, state)
}

func TestReadWinBatteryStateNoBitsSetIsUnknown(t *testing.T) {
	state := readWinBatteryState(0, 50, 100)
	assert.Equal(t, stateUnknown, state)
}

func TestReadWinBatteryStateZeroFullCapacityDoesNotFalselyReportFull(t *testing.T) {
	// full == 0 means capacity is unknown; the full>=current comparison
	// must not spuriously match (0 >= 0).
	state := readWinBatteryState(batteryPowerOnLine, 0, 0)
	assert.Equal(t, stateIdle, state)
}
