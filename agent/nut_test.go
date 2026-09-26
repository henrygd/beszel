//go:build testing

package agent

import (
	"testing"

	"github.com/henrygd/beszel/internal/entities/nut"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseNutDataBasic(t *testing.T) {
	vars := map[string]string{
		"ups.model":          "APC SMT 1500",
		"ups.manufacturer":   "APC",
		"ups.serial":         "ABC123",
		"ups.firmware":       "v2.9",
		"driver":             "apcupsd",
		"ups.status":         "OL",
		"battery.charge":     "100",
		"battery.voltage":    "27.2",
		"battery.runtime":    "1800",
		"input.voltage":      "120.5",
		"output.voltage":     "121.0",
		"input.voltage.nominal": "120",
		"ups.load":           "45",
		"output.current":     "3.2",
		"output.power":       "388",
	}

	data := parseNutData("ups1", vars)

	assert.Equal(t, "APC SMT 1500", data.Model)
	assert.Equal(t, "APC", data.Manufacturer)
	assert.Equal(t, "ABC123", data.Serial)
	assert.Equal(t, "v2.9", data.Firmware)
	assert.Equal(t, "apcupsd", data.Driver)
	assert.Equal(t, "OL", data.Status)
	assert.Equal(t, nut.HealthOnline, data.Health)
	assert.Equal(t, 100.0, data.BatteryCharge)
	assert.Equal(t, 27.2, data.BatteryVoltage)
	assert.Equal(t, int64(1800), data.BatteryRuntime)
	assert.Equal(t, 120.5, data.InputVoltage)
	assert.Equal(t, 121.0, data.OutputVoltage)
	assert.Equal(t, 120.0, data.InputNominal)
	assert.Equal(t, 45.0, data.Load)
	assert.Equal(t, 3.2, data.OutputCurrent)
	assert.Equal(t, 388.0, data.OutputPower)
	assert.Equal(t, nut.DeviceTypeUPS, data.DeviceType)
	assert.Nil(t, data.Outlets)
}

func TestParseNutDataEmptyModelFallsBackToName(t *testing.T) {
	vars := map[string]string{
		"ups.status": "OL",
	}

	data := parseNutData("myups", vars)
	assert.Equal(t, "myups", data.Model)
}

func TestParseNutDataMissingFields(t *testing.T) {
	vars := map[string]string{
		"ups.status": "OL",
	}

	data := parseNutData("ups1", vars)

	// Model falls back to device name when ups.model is absent
	assert.Equal(t, "ups1", data.Model)
	assert.Equal(t, 0.0, data.BatteryCharge)
	assert.Equal(t, int64(0), data.BatteryRuntime)
	assert.Equal(t, nut.DeviceTypeOther, data.DeviceType)
	assert.Nil(t, data.Outlets)
}

func TestDeriveHealth(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"OL", nut.HealthOnline},
		{"OB", nut.HealthOnBattery},
		{"OB LB", nut.HealthLowBattery},
		{"OL OVER", nut.HealthOverload},
		{"SD", nut.HealthFault},
		{"OFF", nut.HealthFault},
		{"OFFB", nut.HealthFault},
		{"COMM", nut.HealthFault},
		{"OL OB", nut.HealthOnBattery},
		{"", nut.HealthUnknown},
		{"OL OB LB", nut.HealthLowBattery},
		{"OL COMM", nut.HealthFault},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			assert.Equal(t, tt.want, deriveHealth(tt.status))
		})
	}
}

func TestDetectDeviceType(t *testing.T) {
	tests := []struct {
		name string
		vars map[string]string
		want string
	}{
		{
			name: "ups with battery",
			vars: map[string]string{"battery.charge": "100"},
			want: nut.DeviceTypeUPS,
		},
		{
			name: "ups with battery voltage",
			vars: map[string]string{"battery.voltage": "27.0"},
			want: nut.DeviceTypeUPS,
		},
		{
			name: "pdu with outlets",
			vars: map[string]string{"outlet.1.status": "on"},
			want: nut.DeviceTypePDU,
		},
		{
			name: "other device",
			vars: map[string]string{"ups.status": "OL"},
			want: nut.DeviceTypeOther,
		},
		{
			name: "ups takes precedence over pdu",
			vars: map[string]string{"battery.charge": "50", "outlet.1.status": "on"},
			want: nut.DeviceTypeUPS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, detectDeviceType(tt.vars))
		})
	}
}

func TestParseOutlets(t *testing.T) {
	vars := map[string]string{
		"outlet.1.status":  "on",
		"outlet.1.desc":    "Server 1",
		"outlet.1.current": "2.5",
		"outlet.1.power":   "300",
		"outlet.1.energy":  "15.5",
		"outlet.1.voltage": "120.1",
		"outlet.2.status":  "off",
		"outlet.2.desc":    "Server 2",
		"outlet.3.status":  "1",
	}

	outlets := parseOutlets(vars)
	require.Len(t, outlets, 3)

	byID := make(map[string]*nut.NutOutlet)
	for _, o := range outlets {
		byID[o.ID] = o
	}

	require.Contains(t, byID, "1")
	assert.Equal(t, "Server 1", byID["1"].Desc)
	assert.Equal(t, "on", byID["1"].Status)
	assert.Equal(t, 2.5, byID["1"].Current)
	assert.Equal(t, 300.0, byID["1"].Power)
	assert.Equal(t, 15.5, byID["1"].Energy)
	assert.Equal(t, 120.1, byID["1"].Voltage)

	require.Contains(t, byID, "2")
	assert.Equal(t, "off", byID["2"].Status)

	require.Contains(t, byID, "3")
	assert.Equal(t, "on", byID["3"].Status)
}

func TestParseOutletsNoOutlets(t *testing.T) {
	vars := map[string]string{
		"ups.status": "OL",
	}
	assert.Nil(t, parseOutlets(vars))
}

func TestNormalizeOutletStatus(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"on", "on"},
		{"ON", "on"},
		{"1", "on"},
		{"off", "off"},
		{"OFF", "off"},
		{"0", "off"},
		{"", "unknown"},
		{"weird", "unknown"},
		{" on ", "on"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeOutletStatus(tt.input))
		})
	}
}

func TestUpscArgsNoAuth(t *testing.T) {
	nm := &NutManager{}
	assert.Equal(t, []string{"-L", "-j", "localhost"}, nm.upscArgs("-L", "-j", "localhost"))
}

func TestUpscArgsWithAuth(t *testing.T) {
	nm := &NutManager{authFile: "/etc/nut/upscmd.conf"}
	assert.Equal(t, []string{"-A", "/etc/nut/upscmd.conf", "-j", "ups1@localhost"}, nm.upscArgs("-j", "ups1@localhost"))
}

func TestParseConfiguredDevices(t *testing.T) {
	t.Setenv("NUT_DEVICES", "ups1, ups2 ,ups3")
	nm := &NutManager{}
	nm.parseConfiguredDevices()
	assert.Equal(t, []string{"ups1", "ups2", "ups3"}, nm.configuredDevices)
}

func TestParseConfiguredDevicesWithSeparator(t *testing.T) {
	t.Setenv("NUT_DEVICES", "ups1|ups2|ups3")
	t.Setenv("NUT_DEVICES_SEPARATOR", "|")
	nm := &NutManager{}
	nm.parseConfiguredDevices()
	assert.Equal(t, []string{"ups1", "ups2", "ups3"}, nm.configuredDevices)
}

func TestParseConfiguredDevicesEmpty(t *testing.T) {
	t.Setenv("NUT_DEVICES", "  ,  ")
	nm := &NutManager{}
	nm.parseConfiguredDevices()
	assert.Empty(t, nm.configuredDevices)
}

func TestNutRefreshExcludedDevices(t *testing.T) {
	t.Setenv("EXCLUDE_NUT", "ups1, ups2 ,ups3")
	nm := &NutManager{}
	nm.refreshExcludedDevices()
	assert.Equal(t, map[string]struct{}{"ups1": {}, "ups2": {}, "ups3": {}}, nm.excludedDevices)
}

func TestNutRefreshExcludedDevicesEmpty(t *testing.T) {
	t.Setenv("EXCLUDE_NUT", "")
	nm := &NutManager{}
	nm.refreshExcludedDevices()
	assert.Empty(t, nm.excludedDevices)
}

func TestIsExcluded(t *testing.T) {
	nm := &NutManager{
		excludedDevices: map[string]struct{}{"ups1": {}},
	}
	assert.True(t, nm.isExcluded("ups1"))
	assert.False(t, nm.isExcluded("ups2"))
}

func TestGetCurrentDataReturnsCopy(t *testing.T) {
	nm := &NutManager{
		NutDataMap: map[string]*nut.NutData{
			"ups1": {Model: "APC", Health: nut.HealthOnline},
			"ups2": nil,
		},
	}

	data := nm.GetCurrentData()
	require.Len(t, data, 1)
	assert.Equal(t, "APC", data["ups1"].Model)
	assert.NotContains(t, data, "ups2")

	// Mutating the original should not affect the returned copy
	nm.NutDataMap["ups1"].Model = "Changed"
	assert.Equal(t, "APC", data["ups1"].Model)
}

func TestHasOutletStatus(t *testing.T) {
	assert.True(t, hasOutletStatus(map[string]string{"outlet.1.status": "on"}))
	assert.True(t, hasOutletStatus(map[string]string{"outlet.abc.status": "off"}))
	assert.False(t, hasOutletStatus(map[string]string{"outlet.1.desc": "x"}))
	assert.False(t, hasOutletStatus(map[string]string{"ups.status": "OL"}))
	assert.False(t, hasOutletStatus(map[string]string{}))
}

func TestParseFloat(t *testing.T) {
	assert.Equal(t, 0.0, parseFloat(""))
	assert.Equal(t, 3.14, parseFloat("3.14"))
	assert.Equal(t, 0.0, parseFloat("abc"))
	assert.Equal(t, 42.0, parseFloat(" 42 "))
}

func TestParseInt64(t *testing.T) {
	assert.Equal(t, int64(0), parseInt64(""))
	assert.Equal(t, int64(1800), parseInt64("1800"))
	assert.Equal(t, int64(0), parseInt64("abc"))
	assert.Equal(t, int64(42), parseInt64(" 42 "))
}