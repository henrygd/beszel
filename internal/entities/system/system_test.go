package system

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/henrygd/beszel/internal/entities/container"
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

func TestInfoSwapAndPackageUpdatesTransport(t *testing.T) {
	signal := -55.0
	info := Info{
		SwapPct:        42.5,
		PackageUpdates: []uint16{12, 3},
		WiFi:           map[string]WiFi{"wlan0": {SSID: "home", Signal: &signal}},
		SystemdLogs:    true,
	}

	data, err := cbor.Marshal(info)
	require.NoError(t, err)
	var payload map[uint64]cbor.RawMessage
	require.NoError(t, cbor.Unmarshal(data, &payload))
	var updates []uint16
	require.NoError(t, cbor.Unmarshal(payload[25], &updates))
	assert.Equal(t, info.PackageUpdates, updates)
	var swap float64
	require.NoError(t, cbor.Unmarshal(payload[28], &swap))
	assert.Equal(t, info.SwapPct, swap)
	var wifi map[string]WiFi
	require.NoError(t, cbor.Unmarshal(payload[26], &wifi))
	assert.Equal(t, info.WiFi, wifi)
	var logs bool
	require.NoError(t, cbor.Unmarshal(payload[27], &logs))
	assert.True(t, logs)

	var decoded Info
	require.NoError(t, cbor.Unmarshal(data, &decoded))
	assert.Equal(t, info.PackageUpdates, decoded.PackageUpdates)
	assert.Equal(t, info.SwapPct, decoded.SwapPct)
	assert.Equal(t, info.WiFi, decoded.WiFi)
	assert.True(t, decoded.SystemdLogs)

	legacyData, err := cbor.Marshal(map[uint64]any{25: info.PackageUpdates, 26: info.WiFi, 27: true})
	require.NoError(t, err)
	var legacy Info
	require.NoError(t, cbor.Unmarshal(legacyData, &legacy))
	assert.Equal(t, info.PackageUpdates, legacy.PackageUpdates)
	assert.Equal(t, info.WiFi, legacy.WiFi)
	assert.True(t, legacy.SystemdLogs)
	assert.Zero(t, legacy.SwapPct)
}

func TestStatsSwapAndWiFiTransport(t *testing.T) {
	stats := Stats{SwapPct: 42.5, WiFi: map[string]int8{"wlan0": -55}}
	data, err := cbor.Marshal(stats)
	require.NoError(t, err)
	var payload map[uint64]cbor.RawMessage
	require.NoError(t, cbor.Unmarshal(data, &payload))
	var wifi map[string]int8
	require.NoError(t, cbor.Unmarshal(payload[40], &wifi))
	assert.Equal(t, stats.WiFi, wifi)
	var swap float64
	require.NoError(t, cbor.Unmarshal(payload[41], &swap))
	assert.Equal(t, stats.SwapPct, swap)
	var decoded Stats
	require.NoError(t, cbor.Unmarshal(data, &decoded))
	assert.Equal(t, stats.WiFi, decoded.WiFi)
	assert.Equal(t, stats.SwapPct, decoded.SwapPct)

	legacyData, err := cbor.Marshal(map[uint64]any{40: stats.WiFi})
	require.NoError(t, err)
	var legacy Stats
	require.NoError(t, cbor.Unmarshal(legacyData, &legacy))
	assert.Equal(t, stats.WiFi, legacy.WiFi)
	assert.Zero(t, legacy.SwapPct)
}

func TestSwapJSONTransport(t *testing.T) {
	for name, marshal := range map[string]func(any) ([]byte, error){
		"json_v1": json.Marshal,
		"json_v2": func(value any) ([]byte, error) { return jsonv2.Marshal(value) },
	} {
		t.Run(name, func(t *testing.T) {
			for _, swap := range []float64{0, 42.5, 100} {
				data, err := marshal(CombinedData{Info: Info{SwapPct: swap}, Stats: Stats{SwapPct: swap}})
				require.NoError(t, err)
				var payload struct {
					Info  map[string]any `json:"info"`
					Stats map[string]any `json:"stats"`
				}
				require.NoError(t, json.Unmarshal(data, &payload))
				assert.Equal(t, swap, payload.Info["sp"])
				assert.Equal(t, swap, payload.Stats["sp"])
				var decoded CombinedData
				require.NoError(t, json.Unmarshal(data, &decoded))
				assert.Equal(t, swap, decoded.Info.SwapPct)
				assert.Equal(t, swap, decoded.Stats.SwapPct)
			}
		})
	}
	var legacy CombinedData
	require.NoError(t, json.Unmarshal([]byte(`{"info":{"pu":[12,3],"wf":{"wlan0":{"s":"home","r":-55}}},"stats":{"s":2,"su":1,"wf":{"wlan0":-55}}}`), &legacy))
	assert.Zero(t, legacy.Info.SwapPct)
	assert.Zero(t, legacy.Stats.SwapPct)
	assert.Equal(t, []uint16{12, 3}, legacy.Info.PackageUpdates)
	assert.Equal(t, int8(-55), legacy.Stats.WiFi["wlan0"])
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

func TestCombinedDataSystemdUpdateMarkerTransport(t *testing.T) {
	data := CombinedData{SystemdServicesUpdated: true}

	jsonData, err := json.Marshal(data)
	require.NoError(t, err)
	var decodedJSON CombinedData
	require.NoError(t, json.Unmarshal(jsonData, &decodedJSON))
	assert.True(t, decodedJSON.SystemdServicesUpdated)
	assert.Empty(t, decodedJSON.SystemdServices)

	cborData, err := cbor.Marshal(data)
	require.NoError(t, err)
	var decodedCBOR CombinedData
	require.NoError(t, cbor.Unmarshal(cborData, &decodedCBOR))
	assert.True(t, decodedCBOR.SystemdServicesUpdated)
	assert.Empty(t, decodedCBOR.SystemdServices)

	var legacy CombinedData
	require.NoError(t, json.Unmarshal([]byte(`{"stats":{},"info":{},"container":[]}`), &legacy))
	assert.False(t, legacy.SystemdServicesUpdated)
}

func TestCombinedDataContainerValidityTransport(t *testing.T) {
	validEmpty := CombinedData{Containers: []*container.Stats{}}

	jsonData, err := json.Marshal(validEmpty)
	require.NoError(t, err)
	var decodedJSON CombinedData
	require.NoError(t, json.Unmarshal(jsonData, &decodedJSON))
	assert.NotNil(t, decodedJSON.Containers)
	assert.Empty(t, decodedJSON.Containers)

	jsonV2Data, err := jsonv2.Marshal(validEmpty)
	require.NoError(t, err)
	var decodedJSONV2 CombinedData
	require.NoError(t, jsonv2.Unmarshal(jsonV2Data, &decodedJSONV2))
	assert.NotNil(t, decodedJSONV2.Containers)
	assert.Empty(t, decodedJSONV2.Containers)

	cborData, err := cbor.Marshal(validEmpty)
	require.NoError(t, err)
	var decodedCBOR CombinedData
	require.NoError(t, cbor.Unmarshal(cborData, &decodedCBOR))
	assert.NotNil(t, decodedCBOR.Containers)
	assert.Empty(t, decodedCBOR.Containers)

	invalidData, err := cbor.Marshal(CombinedData{})
	require.NoError(t, err)
	var decodedInvalid CombinedData
	require.NoError(t, cbor.Unmarshal(invalidData, &decodedInvalid))
	assert.Nil(t, decodedInvalid.Containers)
}
