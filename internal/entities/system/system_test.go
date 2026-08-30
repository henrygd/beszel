package system

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatsBatteryTransport(t *testing.T) {
	stats := Stats{Battery: [2]uint8{0, 1}, Batteries: map[string]uint8{"Primary": 0, "Mouse": 75}}

	for name, marshal := range map[string]func(any) ([]byte, error){
		"json_v1": json.Marshal,
		"json_v2": func(value any) ([]byte, error) { return jsonv2.Marshal(value) },
	} {
		t.Run(name, func(t *testing.T) {
			jsonData, err := marshal(stats)
			require.NoError(t, err)
			var jsonPayload map[string]any
			require.NoError(t, json.Unmarshal(jsonData, &jsonPayload))
			assert.Equal(t, []any{float64(0), float64(1)}, jsonPayload["bat"])
			assert.Equal(t, map[string]any{"Primary": float64(0), "Mouse": float64(75)}, jsonPayload["bats"])
		})
	}

	cborData, err := cbor.Marshal(stats)
	require.NoError(t, err)
	var decoded Stats
	require.NoError(t, cbor.Unmarshal(cborData, &decoded))
	assert.Equal(t, stats.Battery, decoded.Battery)
	assert.Equal(t, stats.Batteries, decoded.Batteries)
}

func TestStatsDiskIOTotalAndFansTransport(t *testing.T) {
	stats := Stats{
		DiskIOTotal: [2]uint64{437348527104, 331522465792},
		Fans:        map[string]uint16{"cpu": 1200},
	}

	cborData, err := cbor.Marshal(stats)
	require.NoError(t, err)
	var decoded Stats
	require.NoError(t, cbor.Unmarshal(cborData, &decoded))
	assert.Equal(t, stats.DiskIOTotal, decoded.DiskIOTotal)
	assert.Equal(t, stats.Fans, decoded.Fans)
}

func TestStatsBatteryNumericArrayUnmarshal(t *testing.T) {
	var stats Stats
	require.NoError(t, json.Unmarshal([]byte(`{"bat":[50,4]}`), &stats))
	assert.Equal(t, Battery{50, 4}, stats.Battery)
}

func TestStatsLegacyBatteryPayload(t *testing.T) {
	data, err := json.Marshal(Stats{Battery: [2]uint8{50, 4}})
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(data, &payload))
	assert.Contains(t, payload, "bat")
	assert.NotContains(t, payload, "bats")
}
