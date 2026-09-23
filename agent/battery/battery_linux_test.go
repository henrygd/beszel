//go:build testing && linux

package battery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeBattery struct{ id, name, capacity, status, full, scope string }

func setupFakeSysfs(t *testing.T) (string, func(fakeBattery)) {
	t.Helper()
	root := t.TempDir()
	previousRoot := batteryRoot
	batteryRoot = root
	t.Cleanup(func() { batteryRoot = previousRoot })
	write := func(path, value string) {
		t.Helper()
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(value), 0o644))
	}
	add := func(b fakeBattery) {
		t.Helper()
		dir := filepath.Join(root, b.id)
		write(filepath.Join(dir, "type"), "Battery")
		if b.capacity != "" {
			write(filepath.Join(dir, "capacity"), b.capacity)
		}
		write(filepath.Join(dir, "status"), b.status)
		if b.name != "" {
			write(filepath.Join(dir, "model_name"), b.name)
		}
		if b.full != "" {
			write(filepath.Join(dir, "energy_full"), b.full)
		}
		if b.scope != "" {
			write(filepath.Join(dir, "scope"), b.scope)
		}
	}
	return root, add
}

func TestParseSysfsState(t *testing.T) {
	assert.Equal(t, stateEmpty, parseSysfsState("Empty"))
	assert.Equal(t, stateFull, parseSysfsState("Full"))
	assert.Equal(t, stateCharging, parseSysfsState("Charging"))
	assert.Equal(t, stateDischarging, parseSysfsState("Discharging"))
	assert.Equal(t, stateIdle, parseSysfsState("Not charging"))
	assert.Equal(t, stateUnknown, parseSysfsState("SomethingElse"))
}

func TestGetBatteryStatsMultipleNamedAndPrimary(t *testing.T) {
	_, add := setupFakeSysfs(t)
	add(fakeBattery{id: "BAT0", name: "Primary", capacity: "105", status: "Charging", full: "5000", scope: "System"})
	add(fakeBattery{id: "hidpp_battery_0", name: "MX Keys S", capacity: "55", status: "Unknown", full: "900", scope: "Device"})
	batteries, err := GetBatteryStats()
	require.NoError(t, err)
	require.Len(t, batteries, 2)
	assert.Equal(t, "Primary", batteries[0].Name)
	assert.Equal(t, uint8(100), batteries[0].Percent)
	assert.Equal(t, stateUnknown, batteries[1].State)
	primary, ok := Primary(batteries)
	require.True(t, ok)
	assert.Equal(t, "Primary", primary.Name)
}

func TestGetBatteryStatsFallbackDuplicatesAndUnreadable(t *testing.T) {
	root, add := setupFakeSysfs(t)
	add(fakeBattery{id: "BAT0", name: "Keyboard", capacity: "80", status: "Discharging"})
	add(fakeBattery{id: "BAT1", name: "Keyboard", capacity: "-4", status: "SomethingWeird"})
	add(fakeBattery{id: "BAT2", capacity: "not-a-number", status: "Charging"})
	add(fakeBattery{id: "BAT3", capacity: "42", status: "Full"})
	ac := filepath.Join(root, "AC0")
	require.NoError(t, os.MkdirAll(ac, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ac, "type"), []byte("Mains"), 0o644))
	batteries, err := GetBatteryStats()
	require.NoError(t, err)
	require.Len(t, batteries, 3)
	assert.Equal(t, "Keyboard", batteries[0].Name)
	assert.Equal(t, "Keyboard (2)", batteries[1].Name)
	assert.Equal(t, uint8(0), batteries[1].Percent)
	assert.Equal(t, "BAT3", batteries[2].Name)
}

func TestGetBatteryStatsHotPlugReenumerates(t *testing.T) {
	_, add := setupFakeSysfs(t)
	_, err := GetBatteryStats()
	assert.Error(t, err)
	assert.False(t, HasReadableBattery())
	add(fakeBattery{id: "BAT0", capacity: "64", status: "Discharging"})
	batteries, err := GetBatteryStats()
	require.NoError(t, err)
	assert.True(t, HasReadableBattery())
	require.Len(t, batteries, 1)
	assert.Equal(t, uint8(64), batteries[0].Percent)
}

func TestGetBatteryStatsNoReadableCapacity(t *testing.T) {
	_, add := setupFakeSysfs(t)
	add(fakeBattery{id: "BAT0", status: "Charging"})
	_, err := GetBatteryStats()
	assert.Error(t, err)
	assert.False(t, HasReadableBattery())
}
