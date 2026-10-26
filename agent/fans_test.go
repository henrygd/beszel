//go:build testing

package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFile creates path with parents and writes contents.
func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
}

// TestReadHwmonFans verifies the /sys/class/hwmon walker:
//   - picks up fan*_input from every chip,
//   - keys entries by chip name + sensor label (or fan idx if no label),
//   - retains 0 RPM for stopped fans,
//   - tolerates chips with no fan files at all.
func TestReadHwmonFans(t *testing.T) {
	root := t.TempDir()

	// hwmon0: Raspberry Pi 5 active cooler — one fan, no label.
	writeFile(t, filepath.Join(root, "hwmon0", "name"), "pwmfan\n")
	writeFile(t, filepath.Join(root, "hwmon0", "fan1_input"), "6500\n")

	// hwmon1: a thermal-only chip, no fan files. Must not error.
	writeFile(t, filepath.Join(root, "hwmon1", "name"), "cpu_thermal\n")
	writeFile(t, filepath.Join(root, "hwmon1", "temp1_input"), "55000\n")

	// hwmon2: two fans — one stopped (0 RPM) and one labeled "chassis".
	writeFile(t, filepath.Join(root, "hwmon2", "name"), "nct6798\n")
	writeFile(t, filepath.Join(root, "hwmon2", "fan1_input"), "0\n")
	writeFile(t, filepath.Join(root, "hwmon2", "fan2_input"), "1200\n")
	writeFile(t, filepath.Join(root, "hwmon2", "fan2_label"), "chassis\n")

	fans, err := readHwmonFans(root)
	require.NoError(t, err)

	assert.Equal(t, map[string]uint16{
		"pwmfan_fan1":     6500,
		"nct6798_fan1":    0,
		"nct6798_chassis": 1200,
	}, fans)
}

// TestReadHwmonFansLegacyParent verifies legacy hwmon layouts such as applesmc,
// where the hwmon class node exists but fan attributes live on hwmonN/device.
func TestReadHwmonFansLegacyParent(t *testing.T) {
	root := t.TempDir()
	deviceDir := filepath.Join(root, "devices", "applesmc.768")
	writeFile(t, filepath.Join(deviceDir, "name"), "applesmc\n")
	writeFile(t, filepath.Join(deviceDir, "fan1_input"), "1202\n")
	writeFile(t, filepath.Join(deviceDir, "fan1_label"), "Exhaust\n")

	chipDir := filepath.Join(root, "hwmon1")
	require.NoError(t, os.MkdirAll(chipDir, 0o755))
	require.NoError(t, os.Symlink(deviceDir, filepath.Join(chipDir, "device")))

	fans, err := readHwmonFans(root)
	require.NoError(t, err)
	assert.Equal(t, map[string]uint16{"applesmc_Exhaust": 1202}, fans)
}

// TestReadHwmonFansMissingRoot returns an error rather than panicking when the
// hwmon root doesn't exist (e.g. running on a kernel without hwmon support).
func TestReadHwmonFansMissingRoot(t *testing.T) {
	_, err := readHwmonFans(filepath.Join(t.TempDir(), "does-not-exist"))
	assert.Error(t, err)
}

// TestReadHwmonFansEmpty returns an empty map (not nil error) when the root
// exists but contains no chips at all.
func TestReadHwmonFansEmpty(t *testing.T) {
	root := t.TempDir()
	fans, err := readHwmonFans(root)
	require.NoError(t, err)
	assert.Empty(t, fans)
}

func TestFanDiscoveryCache(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "hwmon0", "fan1_input")
	writeFile(t, filepath.Join(root, "hwmon0", "name"), "chip\n")
	writeFile(t, input, "1000\n")

	getSensors := newFanSensorCache(root)
	sensors, err := getSensors()
	require.NoError(t, err)
	fans := readFanSensors(sensors)
	assert.Equal(t, uint16(1000), fans["chip_fan1"])

	writeFile(t, input, "1200\n")
	writeFile(t, filepath.Join(root, "hwmon0", "fan1_label"), "case\n")
	sensors, err = getSensors()
	require.NoError(t, err)
	fans = readFanSensors(sensors)
	assert.Equal(t, map[string]uint16{"chip_fan1": 1200}, fans)
}

func TestFilterGpuFans(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "hwmon0", "name"), "xe\n")
	writeFile(t, filepath.Join(root, "hwmon0", "fan1_input"), "1200\n")
	writeFile(t, filepath.Join(root, "hwmon1", "name"), "nct6798\n")
	writeFile(t, filepath.Join(root, "hwmon1", "fan1_input"), "800\n")

	discovered, err := discoverHwmonFans(root)
	require.NoError(t, err)
	require.Len(t, discovered, 2)

	filtered := filterGpuFans(discovered)
	require.Len(t, filtered, 1)
	assert.Equal(t, "nct6798_fan1", filtered[0].key)
	assert.Len(t, discovered, 2)
}

func TestReadFansWithTimeout(t *testing.T) {
	agent := &Agent{sensorConfig: &SensorConfig{timeout: 10 * time.Millisecond}}
	release := make(chan struct{})
	blocked := func() ([]fanSensor, error) {
		<-release
		return nil, nil
	}

	fans, err := agent.readFansWithTimeout(blocked)
	assert.ErrorIs(t, err, errFanFetchTimeout)
	assert.Nil(t, fans)

	// a read still blocked in the kernel isn't retried on later polls
	calls := 0
	fans, err = agent.readFansWithTimeout(func() ([]fanSensor, error) {
		calls++
		return nil, nil
	})
	assert.ErrorIs(t, err, errSensorReadInFlight)
	assert.Nil(t, fans)
	assert.Zero(t, calls)

	// once the blocked read returns, collection resumes
	close(release)
	require.Eventually(t, func() bool { return !agent.fansReading.Load() }, time.Second, time.Millisecond)

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "hwmon0", "name"), "chip\n")
	writeFile(t, filepath.Join(root, "hwmon0", "fan1_input"), "1000\n")
	fans, err = agent.readFansWithTimeout(newFanSensorCache(root))
	require.NoError(t, err)
	assert.Equal(t, map[string]uint16{"chip_fan1": 1000}, fans)
}

func TestFilterFanSensors(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "hwmon0", "name"), "nct6791\n")
	writeFile(t, filepath.Join(root, "hwmon0", "fan1_input"), "0\n")
	writeFile(t, filepath.Join(root, "hwmon0", "fan2_input"), "900\n")
	writeFile(t, filepath.Join(root, "hwmon0", "fan3_input"), "0\n")
	writeFile(t, filepath.Join(root, "hwmon0", "fan4_input"), "1100\n")
	writeFile(t, filepath.Join(root, "hwmon0", "fan4_label"), "chassis\n")
	writeFile(t, filepath.Join(root, "hwmon1", "name"), "gpio_fan\n")
	writeFile(t, filepath.Join(root, "hwmon1", "fan1_input"), "0\n")

	discovered, err := discoverHwmonFans(root)
	require.NoError(t, err)
	require.Len(t, discovered, 5)

	tests := []struct {
		name       string
		fanSensors string
		expected   []string
	}{
		{"empty list keeps all", "", []string{"nct6791_fan1", "nct6791_fan2", "nct6791_fan3", "nct6791_chassis", "gpio_fan_fan1"}},
		{"whitelist", "nct6791_fan2,nct6791_chassis", []string{"nct6791_fan2", "nct6791_chassis"}},
		{"whitelist matches label not fan index", "nct6791_fan4", []string{}},
		{"blacklist", "-gpio_fan_fan1", []string{"nct6791_fan1", "nct6791_fan2", "nct6791_fan3", "nct6791_chassis"}},
		{"whitelist wildcard", "nct6791_*", []string{"nct6791_fan1", "nct6791_fan2", "nct6791_fan3", "nct6791_chassis"}},
		{"blacklist wildcard and exact", "-gpio_*, nct6791_fan1 ,nct6791_fan3", []string{"nct6791_fan2", "nct6791_chassis"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := filterFanSensors(discovered, parseSensorFilter(tt.fanSensors))
			keys := make([]string, 0, len(filtered))
			for _, sensor := range filtered {
				keys = append(keys, sensor.key)
			}
			assert.ElementsMatch(t, tt.expected, keys)
			assert.Len(t, discovered, 5)
		})
	}
}

func TestReadFansWithTimeoutFanSensors(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "hwmon0", "name"), "nct6791\n")
	writeFile(t, filepath.Join(root, "hwmon0", "fan1_input"), "0\n")
	writeFile(t, filepath.Join(root, "hwmon0", "fan2_input"), "900\n")

	agent := &Agent{sensorConfig: &SensorConfig{timeout: time.Second, fanSensors: parseSensorFilter("-nct6791_fan1")}}
	getSensors := newFanSensorCache(root)
	fans, err := agent.readFansWithTimeout(getSensors)
	require.NoError(t, err)
	assert.Equal(t, map[string]uint16{"nct6791_fan2": 900}, fans)

	// the cached discovery still holds every fan
	sensors, err := getSensors()
	require.NoError(t, err)
	assert.Len(t, sensors, 2)
}
