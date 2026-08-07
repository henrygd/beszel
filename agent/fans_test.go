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
//   - skips 0 RPM (unpopulated headers),
//   - tolerates chips with no fan files at all.
func TestReadHwmonFans(t *testing.T) {
	root := t.TempDir()

	// hwmon0: Raspberry Pi 5 active cooler — one fan, no label.
	writeFile(t, filepath.Join(root, "hwmon0", "name"), "pwmfan\n")
	writeFile(t, filepath.Join(root, "hwmon0", "fan1_input"), "6500\n")

	// hwmon1: a thermal-only chip, no fan files. Must not error.
	writeFile(t, filepath.Join(root, "hwmon1", "name"), "cpu_thermal\n")
	writeFile(t, filepath.Join(root, "hwmon1", "temp1_input"), "55000\n")

	// hwmon2: two fans — one unpopulated (0 RPM, must be skipped) and one
	// labeled "chassis".
	writeFile(t, filepath.Join(root, "hwmon2", "name"), "nct6798\n")
	writeFile(t, filepath.Join(root, "hwmon2", "fan1_input"), "0\n")
	writeFile(t, filepath.Join(root, "hwmon2", "fan2_input"), "1200\n")
	writeFile(t, filepath.Join(root, "hwmon2", "fan2_label"), "chassis\n")

	fans, err := readHwmonFans(root)
	require.NoError(t, err)

	assert.Equal(t, map[string]float64{
		"pwmfan_fan1":      6500,
		"nct6798_chassis":  1200,
	}, fans)
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
