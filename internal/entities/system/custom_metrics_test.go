//go:build testing

package system

import (
	"encoding/json"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func customMetricsPayload(values map[string]float64, meta map[string]CustomMetricMeta) CombinedData {
	return CombinedData{
		Stats: Stats{Cpu: 12.5, CustomMetrics: values},
		Info:  Info{AgentVersion: "0.22.0", CustomMetricsMeta: meta},
	}
}

func TestCustomMetricsCBORRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]float64
		meta   map[string]CustomMetricMeta
	}{
		{name: "nil maps"},
		{name: "empty maps", values: map[string]float64{}, meta: map[string]CustomMetricMeta{}},
		{
			name:   "populated",
			values: map[string]float64{"pi_power_board_watts": 1.84, "myapp_ratio": 0.0012},
			meta: map[string]CustomMetricMeta{
				"pi_power_board_watts": {
					Unit: "watts", Help: "Board power.", DisplayName: "Board power",
					Chart: "Power consumption", ChartDescription: "Measured and estimated",
				},
				"myapp_ratio": {Unit: "ratio"},
				"stale_only":  {DisplayName: "stale_only"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := cbor.Marshal(customMetricsPayload(tt.values, tt.meta))
			require.NoError(t, err)

			var decoded CombinedData
			require.NoError(t, cbor.Unmarshal(encoded, &decoded))
			if len(tt.values) == 0 {
				assert.Nil(t, decoded.Stats.CustomMetrics)
			} else {
				assert.Equal(t, tt.values, decoded.Stats.CustomMetrics)
			}
			if len(tt.meta) == 0 {
				assert.Nil(t, decoded.Info.CustomMetricsMeta)
			} else {
				assert.Equal(t, tt.meta, decoded.Info.CustomMetricsMeta)
			}
		})
	}
}

// An unconfigured agent must send the same bytes as before the feature existed.
func TestCustomMetricsOmittedFromCBORWhenEmpty(t *testing.T) {
	for _, tt := range []struct {
		name   string
		values map[string]float64
		meta   map[string]CustomMetricMeta
	}{
		{name: "nil maps"},
		{name: "empty maps", values: map[string]float64{}, meta: map[string]CustomMetricMeta{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := cbor.Marshal(customMetricsPayload(tt.values, tt.meta))
			require.NoError(t, err)
			baseline, err := cbor.Marshal(customMetricsPayload(nil, nil))
			require.NoError(t, err)
			assert.Equal(t, baseline, encoded)

			var raw map[any]any
			require.NoError(t, cbor.Unmarshal(encoded, &raw))
			stats := raw[uint64(0)].(map[any]any)
			info := raw[uint64(1)].(map[any]any)
			assert.NotContains(t, stats, uint64(48))
			assert.NotContains(t, info, uint64(30))
		})
	}

	encoded, err := cbor.Marshal(customMetricsPayload(map[string]float64{"a": 1}, map[string]CustomMetricMeta{"a": {}}))
	require.NoError(t, err)
	var raw map[any]any
	require.NoError(t, cbor.Unmarshal(encoded, &raw))
	assert.Contains(t, raw[uint64(0)].(map[any]any), uint64(48))
	assert.Contains(t, raw[uint64(1)].(map[any]any), uint64(30))
}

func TestCustomMetricsJSONKeys(t *testing.T) {
	data := customMetricsPayload(
		map[string]float64{"pi_power_board_watts": 1.84},
		map[string]CustomMetricMeta{"pi_power_board_watts": {Unit: "watts", Help: "Board power.", DisplayName: "Board power", Chart: "Power"}},
	)

	encoded, err := json.Marshal(data.Stats)
	require.NoError(t, err)
	var stats map[string]any
	require.NoError(t, json.Unmarshal(encoded, &stats))
	assert.Equal(t, map[string]any{"pi_power_board_watts": 1.84}, stats["cm"])

	encoded, err = json.Marshal(data.Info)
	require.NoError(t, err)
	var info map[string]any
	require.NoError(t, json.Unmarshal(encoded, &info))
	assert.Equal(t, map[string]any{
		"pi_power_board_watts": map[string]any{"u": "watts", "h": "Board power.", "l": "Board power", "c": "Power"},
	}, info["cmm"])

	encoded, err = json.Marshal(customMetricsPayload(nil, nil))
	require.NoError(t, err)
	var empty map[string]map[string]any
	require.NoError(t, json.Unmarshal(encoded, &empty))
	assert.NotContains(t, empty["stats"], "cm")
	assert.NotContains(t, empty["info"], "cmm")
}

// A hub built before the feature decodes into structs without the new fields.
func TestCustomMetricsIgnoredByOldHub(t *testing.T) {
	type oldStats struct {
		Cpu  float64         `cbor:"0,keyasint"`
		WiFi map[string]int8 `cbor:"40,keyasint,omitempty"`
	}
	type oldInfo struct {
		AgentVersion string `cbor:"10,keyasint"`
		SystemdLogs  bool   `cbor:"27,keyasint,omitempty"`
	}
	type oldCombinedData struct {
		Stats oldStats `cbor:"0,keyasint"`
		Info  oldInfo  `cbor:"1,keyasint"`
	}

	data := customMetricsPayload(
		map[string]float64{"pi_power_board_watts": 1.84},
		map[string]CustomMetricMeta{"pi_power_board_watts": {Unit: "watts"}},
	)
	data.Stats.WiFi = map[string]int8{"wlan0": -55}
	encoded, err := cbor.Marshal(data)
	require.NoError(t, err)

	var decoded oldCombinedData
	require.NoError(t, cbor.Unmarshal(encoded, &decoded))
	assert.Equal(t, 12.5, decoded.Stats.Cpu)
	assert.Equal(t, map[string]int8{"wlan0": -55}, decoded.Stats.WiFi)
	assert.Equal(t, "0.22.0", decoded.Info.AgentVersion)
}
