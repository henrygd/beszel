//go:build testing

package agent

import (
	"os"
	"path/filepath"
	"testing"

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
