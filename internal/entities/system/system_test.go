package system

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/henrygd/beszel/internal/entities/smart"
	"github.com/henrygd/beszel/internal/entities/zfs"
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

// GPUs without a package power reading (e.g. NVIDIA) must not get a "pp" key,
// otherwise the UI shows an empty package power series (PocketBase uses json/v2)
func TestGPUDataPowerPkgOmittedWhenZero(t *testing.T) {
	gpus := map[string]GPUData{
		"0": {Name: "GeForce RTX 4060", Power: 91.21},
		"1": {Name: "Arc A380", Power: 10, PowerPkg: 20},
	}

	for name, marshal := range map[string]func(any) ([]byte, error){
		"json_v1": json.Marshal,
		"json_v2": func(value any) ([]byte, error) { return jsonv2.Marshal(value) },
	} {
		t.Run(name, func(t *testing.T) {
			jsonData, err := marshal(gpus)
			require.NoError(t, err)
			var jsonPayload map[string]map[string]any
			require.NoError(t, json.Unmarshal(jsonData, &jsonPayload))
			assert.NotContains(t, jsonPayload["0"], "pp")
			assert.Equal(t, float64(20), jsonPayload["1"]["pp"])
		})
	}
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

// PocketBase marshals record JSON fields with encoding/json/v2, where omitempty
// no longer omits zero numbers or false. Scalar fields must use omitzero so
// stored records keep the same shape as encoding/json (v1).
func TestZeroValueJSONMatchesV1(t *testing.T) {
	for name, value := range map[string]any{
		"stats": Stats{
			ExtraFs:  map[string]*FsStats{"sda": {}},
			GPUData:  map[string]GPUData{"0": {}},
			ZfsPools: map[string]*ZfsPool{"tank": {}},
		},
		"info":            Info{WiFi: map[string]WiFi{"wlan0": {}}},
		"package_updates": PackageUpdates{Packages: []PackageUpdate{{}}},
		"zfs": zfs.ZfsData{Pools: []*zfs.PoolDetail{{
			Scrub:    &zfs.Scrub{State: "NONE"}, // a scrub encoding to {} is dropped by v2 omitempty
			Vdevs:    []*zfs.Vdev{{}},
			Datasets: []*zfs.Dataset{{}},
		}}},
		"smart": smart.SmartData{Attributes: []*smart.SmartAttribute{{}}},
	} {
		t.Run(name, func(t *testing.T) {
			v1Data, err := json.Marshal(value)
			require.NoError(t, err)
			v2Data, err := jsonv2.Marshal(value)
			require.NoError(t, err)
			assert.JSONEq(t, string(v1Data), string(v2Data))
		})
	}
}

func TestTrimGPUClients(t *testing.T) {
	clients := map[string]float64{"c:a": 5, "c:b": 50, "s:c": 20, "p:d": 20}
	assert.Equal(t, clients, TrimGPUClients(clients, 4))
	assert.Equal(t, map[string]float64{"c:b": 50, "p:d": 20}, TrimGPUClients(clients, 2))
}

func TestGPUDataClientsTransport(t *testing.T) {
	gpus := map[string]GPUData{
		"card0": {Name: "Intel", Clients: map[string]float64{"c:jellyfin": 30.5}},
		"card1": {Name: "AMD"},
	}

	jsonData, err := jsonv2.Marshal(gpus)
	require.NoError(t, err)
	var jsonPayload map[string]map[string]any
	require.NoError(t, json.Unmarshal(jsonData, &jsonPayload))
	assert.Equal(t, map[string]any{"c:jellyfin": 30.5}, jsonPayload["card0"]["c"])
	assert.NotContains(t, jsonPayload["card1"], "c")

	cborData, err := cbor.Marshal(gpus)
	require.NoError(t, err)
	var decoded map[string]GPUData
	require.NoError(t, cbor.Unmarshal(cborData, &decoded))
	assert.Equal(t, gpus["card0"].Clients, decoded["card0"].Clients)
	assert.Nil(t, decoded["card1"].Clients)
}
