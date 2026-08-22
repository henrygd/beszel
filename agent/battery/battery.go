// Package battery provides battery information for the host and connected devices.
package battery

import (
	"errors"
	"sort"
	"strconv"
	"strings"
)

const (
	stateUnknown uint8 = iota
	stateEmpty
	stateFull
	stateCharging
	stateDischarging
	stateIdle
)

// Battery is a readable battery reported by the operating system.
type Battery struct {
	Name                  string
	Percent               uint8
	State                 uint8
	FullChargeCapacity    uint64
	HasFullChargeCapacity bool
	System                bool
}

var errNoBatteries = errors.New("no readable batteries")

// normalizeBatteries supplies stable fallback names and disambiguates duplicates.
func normalizeBatteries(batteries []Battery) []Battery {
	nameCounts := make(map[string]int, len(batteries))
	for i := range batteries {
		// Names come from firmware (e.g. sysfs model_name) and are not guaranteed to
		// be valid UTF-8. Invalid bytes are rejected when the hub decodes the CBOR
		// payload, which drops every metric for the system, so strip them here.
		name := strings.TrimSpace(strings.ToValidUTF8(batteries[i].Name, ""))
		if name == "" {
			name = "Battery " + strconv.Itoa(i+1)
		}
		nameCounts[name]++
		if nameCounts[name] > 1 {
			name += " (" + strconv.Itoa(nameCounts[name]) + ")"
		}
		batteries[i].Name = name
	}
	return batteries
}

// Primary returns the representative battery. Reported full-charge capacity wins,
// then system-scoped devices, then name for deterministic ties.
func Primary(batteries []Battery) (Battery, bool) {
	if len(batteries) == 0 {
		return Battery{}, false
	}
	ordered := append([]Battery(nil), batteries...)
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if a.HasFullChargeCapacity != b.HasFullChargeCapacity {
			return a.HasFullChargeCapacity
		}
		if a.HasFullChargeCapacity && a.FullChargeCapacity != b.FullChargeCapacity {
			return a.FullChargeCapacity > b.FullChargeCapacity
		}
		if a.System != b.System {
			return a.System
		}
		return a.Name < b.Name
	})
	return ordered[0], true
}
