//go:build testing && linux

package battery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// setupFakeSysfs creates a temporary sysfs-like tree under t.TempDir(),
// swaps sysfsPowerSupply, resets the sync.Once caches, and restores
// everything on cleanup. Returns a helper to create battery directories.
func setupFakeSysfs(t *testing.T) (tmpDir string, addBattery func(name, capacity, status string)) {
	t.Helper()

	tmp := t.TempDir()
	resetBatteryState(tmp)

	write := func(path, content string) {
		t.Helper()
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	addBattery = func(name, capacity, status string) {
		t.Helper()
		batDir := filepath.Join(tmp, name)
		write(filepath.Join(batDir, "type"), "Battery")
		write(filepath.Join(batDir, "capacity"), capacity)
		write(filepath.Join(batDir, "status"), status)
	}

	return tmp, addBattery
}

func TestParseSysfsState(t *testing.T) {
	tests := []struct {
		input string
		want  uint8
	}{
		{"Empty", stateEmpty},
		{"Full", stateFull},
		{"Charging", stateCharging},
		{"Discharging", stateDischarging},
		{"Not charging", stateIdle},
		{"", stateUnknown},
		{"SomethingElse", stateUnknown},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, parseSysfsState(tt.input), "parseSysfsState(%q)", tt.input)
	}
}

func TestGetBatteryStats_SingleBattery(t *testing.T) {
	_, addBattery := setupFakeSysfs(t)
	addBattery("BAT0", "72", "Discharging")

	pct, state, err := GetBatteryStats()
	assert.NoError(t, err)
	assert.Equal(t, uint8(72), pct)
	assert.Equal(t, stateDischarging, state)
}

func TestGetBatteryStats_MultipleBatteries(t *testing.T) {
	_, addBattery := setupFakeSysfs(t)
	addBattery("BAT0", "80", "Charging")
	addBattery("BAT1", "40", "Charging")

	pct, state, err := GetBatteryStats()
	assert.NoError(t, err)
	// average of 80 and 40 = 60
	assert.EqualValues(t, 60, pct)
	assert.Equal(t, stateCharging, state)
}

func TestGetBatteryStats_FullBattery(t *testing.T) {
	_, addBattery := setupFakeSysfs(t)
	addBattery("BAT0", "100", "Full")

	pct, state, err := GetBatteryStats()
	assert.NoError(t, err)
	assert.Equal(t, uint8(100), pct)
	assert.Equal(t, stateFull, state)
}

func TestGetBatteryStats_EmptyBattery(t *testing.T) {
	_, addBattery := setupFakeSysfs(t)
	addBattery("BAT0", "0", "Empty")

	pct, state, err := GetBatteryStats()
	assert.NoError(t, err)
	assert.Equal(t, uint8(0), pct)
	assert.Equal(t, stateEmpty, state)
}

func TestGetBatteryStats_NotCharging(t *testing.T) {
	_, addBattery := setupFakeSysfs(t)
	addBattery("BAT0", "80", "Not charging")

	pct, state, err := GetBatteryStats()
	assert.NoError(t, err)
	assert.Equal(t, uint8(80), pct)
	assert.Equal(t, stateIdle, state)
}

func TestGetBatteryStats_NoBatteries(t *testing.T) {
	setupFakeSysfs(t) // empty directory, no batteries

	_, _, err := GetBatteryStats()
	assert.Error(t, err)
}

func TestGetBatteryStats_NonBatterySupplyIgnored(t *testing.T) {
	tmp, addBattery := setupFakeSysfs(t)

	// Add a real battery
	addBattery("BAT0", "55", "Charging")

	// Add an AC adapter (type != Battery) - should be ignored
	acDir := filepath.Join(tmp, "AC0")
	if err := os.MkdirAll(acDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(acDir, "type"), []byte("Mains"), 0o644); err != nil {
		t.Fatal(err)
	}

	pct, state, err := GetBatteryStats()
	assert.NoError(t, err)
	assert.Equal(t, uint8(55), pct)
	assert.Equal(t, stateCharging, state)
}

func TestGetBatteryStats_InvalidCapacitySkipped(t *testing.T) {
	tmp, addBattery := setupFakeSysfs(t)

	// One battery with valid capacity
	addBattery("BAT0", "90", "Discharging")

	// Another with invalid capacity text
	badDir := filepath.Join(tmp, "BAT1")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badDir, "type"), []byte("Battery"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badDir, "capacity"), []byte("not-a-number"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badDir, "status"), []byte("Discharging"), 0o644); err != nil {
		t.Fatal(err)
	}

	pct, _, err := GetBatteryStats()
	assert.NoError(t, err)
	// Only BAT0 counted
	assert.Equal(t, uint8(90), pct)
}

func TestGetBatteryStats_UnknownStatusOnly(t *testing.T) {
	_, addBattery := setupFakeSysfs(t)
	addBattery("BAT0", "50", "SomethingWeird")

	_, _, err := GetBatteryStats()
	assert.Error(t, err)
}

func TestHasReadableBattery_True(t *testing.T) {
	_, addBattery := setupFakeSysfs(t)
	addBattery("BAT0", "50", "Charging")

	assert.True(t, HasReadableBattery())
}

func TestHasReadableBattery_False(t *testing.T) {
	setupFakeSysfs(t) // no batteries

	assert.False(t, HasReadableBattery())
}

func TestHasReadableBattery_NoCapacityFile(t *testing.T) {
	tmp, _ := setupFakeSysfs(t)

	// Battery dir with type file but no capacity file
	batDir := filepath.Join(tmp, "BAT0")
	err := os.MkdirAll(batDir, 0o755)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(batDir, "type"), []byte("Battery"), 0o644)
	assert.NoError(t, err)

	assert.False(t, HasReadableBattery())
}
