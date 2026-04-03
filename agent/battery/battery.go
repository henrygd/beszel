// Package battery provides functions to check if the system has a battery and return the charge state and percentage.
package battery

const (
	stateUnknown uint8 = iota
	stateEmpty
	stateFull
	stateCharging
	stateDischarging
	stateIdle
)
