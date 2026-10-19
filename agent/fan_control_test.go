//go:build testing

package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFanControlDiscoveryWithoutTachometers(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "hwmon0", "name"), "pwmfan\n")
	for name, value := range map[string]string{
		"pwm1": "102", "pwm2": "153", "pwm3": "204", "pwm4": "255", "pwm5": "0", "pwm6": "128",
		"pwm1_enable": "0", "pwm1_mode": "1", "pwm1_freq": "25000", "pwm0": "255", "pwm01": "255",
	} {
		writeFile(t, filepath.Join(root, "hwmon0", name), value)
	}
	writeFile(t, filepath.Join(root, "hwmon0", "fan1_label"), "not-the-control-label")
	sensors, err := discoverHwmonFanControls(root)
	require.NoError(t, err)
	assert.Equal(t, map[string]float64{
		"pwmfan_pwm1": 40, "pwmfan_pwm2": 60, "pwmfan_pwm3": 80,
		"pwmfan_pwm4": 100, "pwmfan_pwm5": 0, "pwmfan_pwm6": 50.2,
	}, readFanControls(sensors))
	rpm, err := readHwmonFans(root)
	require.NoError(t, err)
	assert.Empty(t, rpm)
}

func TestFanControlGPIOTransitions(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "hwmon0", "pwm1")
	writeFile(t, filepath.Join(root, "hwmon0", "name"), "gpio_fan")
	writeFile(t, filepath.Join(root, "hwmon0", "fan1_input"), "5000") // configured, not measured
	writeFile(t, input, "0")
	sensors, err := discoverHwmonFanControls(root)
	require.NoError(t, err)
	assert.Equal(t, map[string]float64{"gpio_fan_pwm1": 0}, readFanControls(sensors))
	writeFile(t, input, "255")
	assert.Equal(t, map[string]float64{"gpio_fan_pwm1": 100}, readFanControls(sensors))
	writeFile(t, input, "0")
	assert.Equal(t, map[string]float64{"gpio_fan_pwm1": 0}, readFanControls(sensors))

	writeFile(t, filepath.Join(root, "hwmon1", "name"), "tachometer")
	writeFile(t, filepath.Join(root, "hwmon1", "fan1_input"), "1200")
	rpm, err := readHwmonFans(root)
	require.NoError(t, err)
	assert.Equal(t, map[string]uint16{"tachometer_fan1": 1200}, rpm)
}

func TestFanControlLegacyAndDuplicateChips(t *testing.T) {
	root := t.TempDir()
	device := t.TempDir()
	writeFile(t, filepath.Join(device, "name"), "pwmfan")
	writeFile(t, filepath.Join(device, "pwm1"), "255")
	writeFile(t, filepath.Join(root, "hwmon0", "name"), "pwmfan")
	// A direct RPM file must not prevent independent control discovery on the parent.
	writeFile(t, filepath.Join(root, "hwmon0", "fan1_input"), "1200")
	require.NoError(t, os.Symlink(device, filepath.Join(root, "hwmon0", "device")))
	writeFile(t, filepath.Join(root, "hwmon1", "name"), "pwmfan")
	writeFile(t, filepath.Join(root, "hwmon1", "pwm1"), "102")
	sensors, err := discoverHwmonFanControls(root)
	require.NoError(t, err)
	assert.Equal(t, map[string]float64{"pwmfan_pwm1_hwmon0": 100, "pwmfan_pwm1_hwmon1": 40}, readFanControls(sensors))
}

func TestFanControlInvalidReadings(t *testing.T) {
	root := t.TempDir()
	for _, value := range []string{"-1", "256", "not-a-number", "", "1.5", "18446744073709551616"} {
		t.Run(value, func(t *testing.T) {
			path := filepath.Join(root, "pwm1")
			writeFile(t, path, value)
			assert.Nil(t, readFanControls([]fanSensor{{key: "fan", path: path}}))
		})
	}
	assert.Nil(t, readFanControls([]fanSensor{
		{key: "missing", path: filepath.Join(root, "missing")},
		{key: "unreadable", path: root}, // ReadFile fails deterministically, including as root.
	}))
	sensors, err := discoverHwmonFanControls(filepath.Join(root, "missing"))
	assert.Error(t, err)
	assert.Empty(t, sensors)
	sensors, err = discoverHwmonFanControls(t.TempDir())
	require.NoError(t, err)
	assert.Empty(t, sensors)
}

func TestUpdateFanControlsSkipsGPU(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "hwmon0", "name"), "amdgpu")
	writeFile(t, filepath.Join(root, "hwmon0", "pwm1"), "255")
	writeFile(t, filepath.Join(root, "hwmon1", "name"), "pwmfan")
	writeFile(t, filepath.Join(root, "hwmon1", "pwm1"), "0")
	sensors, err := discoverHwmonFanControls(root)
	require.NoError(t, err)
	original := getFanControlSensors
	t.Cleanup(func() { getFanControlSensors = original })
	getFanControlSensors = func() ([]fanSensor, error) { return sensors, nil }
	a := &Agent{sensorConfig: &SensorConfig{skipGPU: true}}
	var stats system.Stats
	a.updateFanControls(&stats)
	assert.Equal(t, map[string]float64{"pwmfan_pwm1": 0}, stats.FanControl)
	assert.Len(t, sensors, 2)
	a.sensorConfig.skipGPU = false
	a.updateFanControls(&stats)
	assert.Equal(t, map[string]float64{"pwmfan_pwm1": 0, "amdgpu_pwm1": 100}, stats.FanControl)
}
