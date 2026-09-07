//go:build testing

package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errFakeFreeBSDSysctlNotFound = errors.New("sysctl not found")

type fakeFreeBSDSysctls struct {
	values map[string]uint32
	errs   map[string]error
}

func (f fakeFreeBSDSysctls) read(name string) (uint32, error) {
	if err, ok := f.errs[name]; ok {
		return 0, err
	}
	if value, ok := f.values[name]; ok {
		return value, nil
	}
	return 0, errFakeFreeBSDSysctlNotFound
}

func TestFreeBSDDeciKelvinToCelsius(t *testing.T) {
	tests := []struct {
		name     string
		value    uint32
		expected float64
		ok       bool
	}{
		{
			name:     "45 Celsius",
			value:    3181,
			expected: 45,
			ok:       true,
		},
		{
			name:     "fractional Celsius",
			value:    3186,
			expected: 45.5,
			ok:       true,
		},
		{
			name:  "zero deci-Kelvin",
			value: 0,
			ok:    false,
		},
		{
			name:  "zero Celsius",
			value: freebsdZeroCelsiusDeciKelvin,
			ok:    false,
		},
		{
			name:  "below zero Celsius",
			value: freebsdZeroCelsiusDeciKelvin - 1,
			ok:    false,
		},
		{
			name:  "invalid signed integer",
			value: 1<<32 - 1,
			ok:    false,
		},
		{
			name:  "unreasonably high Celsius",
			value: freebsdZeroCelsiusDeciKelvin + 2000,
			ok:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := freebsdDeciKelvinToCelsius(tt.value)
			assert.Equal(t, tt.ok, ok)
			assert.InDelta(t, tt.expected, result, 0.001)
		})
	}
}

func TestGetFreeBSDSensorTemps(t *testing.T) {
	reader := fakeFreeBSDSysctls{
		values: map[string]uint32{
			"hw.ncpu":                         4,
			"dev.cpu.0.temperature":           3231,
			"dev.cpu.1.temperature":           3242,
			"dev.cpu.3.temperature":           freebsdZeroCelsiusDeciKelvin,
			"hw.acpi.thermal.tz0.temperature": 3101,
			"hw.acpi.thermal.tz2.temperature": 3116,
			"hw.acpi.thermal.tz3.temperature": freebsdZeroCelsiusDeciKelvin,
			"unrelated.sensor.value":          9999,
			"dev.cpu.99.temperature":          9999,
			"dev.amdtemp.0.core0.foo":         9999,
		},
	}

	temps, err := getFreeBSDSensorTemps(context.Background(), reader.read)

	require.NoError(t, err)
	require.Len(t, temps, 4)
	assert.Equal(t, "cpu.0", temps[0].SensorKey)
	assert.InDelta(t, 50.0, temps[0].Temperature, 0.001)
	assert.Equal(t, "cpu.1", temps[1].SensorKey)
	assert.InDelta(t, 51.1, temps[1].Temperature, 0.001)
	assert.Equal(t, "acpi.thermal.tz0", temps[2].SensorKey)
	assert.InDelta(t, 37.0, temps[2].Temperature, 0.001)
	assert.Equal(t, "acpi.thermal.tz2", temps[3].SensorKey)
	assert.InDelta(t, 38.5, temps[3].Temperature, 0.001)
}

func TestGetFreeBSDSensorTempsCpuCountError(t *testing.T) {
	reader := fakeFreeBSDSysctls{
		errs: map[string]error{
			"hw.ncpu": errors.New("permission denied"),
		},
	}

	temps, err := getFreeBSDSensorTemps(context.Background(), reader.read)

	assert.Nil(t, temps)
	assert.EqualError(t, err, "permission denied")
}

func TestGetFreeBSDSensorTempsNoTemperatureSysctls(t *testing.T) {
	reader := fakeFreeBSDSysctls{
		values: map[string]uint32{"hw.ncpu": 2},
	}

	temps, err := getFreeBSDSensorTemps(context.Background(), reader.read)

	require.NoError(t, err)
	assert.Empty(t, temps)
}

func TestGetFreeBSDSensorTempsAcpiOnly(t *testing.T) {
	reader := fakeFreeBSDSysctls{
		values: map[string]uint32{
			"hw.ncpu":                         0,
			"hw.acpi.thermal.tz0.temperature": 3081,
		},
	}

	temps, err := getFreeBSDSensorTemps(context.Background(), reader.read)

	require.NoError(t, err)
	require.Len(t, temps, 1)
	assert.Equal(t, "acpi.thermal.tz0", temps[0].SensorKey)
	assert.InDelta(t, 35.0, temps[0].Temperature, 0.001)
}

func TestGetFreeBSDSensorTempsContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reader := fakeFreeBSDSysctls{
		values: map[string]uint32{"hw.ncpu": 2},
	}

	temps, err := getFreeBSDSensorTemps(ctx, reader.read)

	assert.Empty(t, temps)
	assert.ErrorIs(t, err, context.Canceled)
}
