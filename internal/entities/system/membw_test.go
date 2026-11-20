package system

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemBandwidthCBORRoundTrip(t *testing.T) {
	in := Stats{
		MemBandwidth:    [2]float64{12.34, 5.67},
		MaxMemBandwidth: [2]float64{99, 99},
		GPUData: map[string]GPUData{"0": {
			Name: "RTX PRO 6000", MemBandwidth: 812.5, MemBandwidthPeak: 1792.13,
			MemBandwidthApprox: true, MemBandwidthCount: 7,
		}},
	}
	data, err := cbor.Marshal(in)
	require.NoError(t, err)

	var keys map[int]cbor.RawMessage
	require.NoError(t, cbor.Unmarshal(data, &keys))
	assert.Contains(t, keys, 43)
	assert.NotContains(t, keys, 41, "key 41 is upstream MemBytes")

	gpu := decodeGPUKeys(t, keys)["0"]
	assert.Equal(t, 812.5, gpu[7])
	assert.Equal(t, 1792.13, gpu[8])
	assert.Equal(t, true, gpu[9])
	assert.Len(t, gpu, 4, "only name + 3 bandwidth keys; the agent-side count is not sent")

	var out Stats
	require.NoError(t, cbor.Unmarshal(data, &out))
	assert.Equal(t, in.MemBandwidth, out.MemBandwidth)
	assert.Equal(t, [2]float64{}, out.MaxMemBandwidth, "max is hub-side only")
	assert.Equal(t, 812.5, out.GPUData["0"].MemBandwidth)
	assert.Equal(t, 1792.13, out.GPUData["0"].MemBandwidthPeak)
	assert.True(t, out.GPUData["0"].MemBandwidthApprox)
	assert.Zero(t, out.GPUData["0"].MemBandwidthCount)
	assert.NotContains(t, keys, 44, "total-only flag omitted for read/write sources")

	data, err = cbor.Marshal(Stats{MemBandwidth: [2]float64{80, 0}, MemBandwidthTotalOnly: true})
	require.NoError(t, err)
	keys = nil
	require.NoError(t, cbor.Unmarshal(data, &keys))
	assert.Contains(t, keys, 44)
	assert.NotContains(t, keys, 42, "key 42 is upstream SwapBytes")
	out = Stats{}
	require.NoError(t, cbor.Unmarshal(data, &out))
	assert.True(t, out.MemBandwidthTotalOnly)
	b, err := json.Marshal(out)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"mbwt":true`)
}

func TestMemBandwidthOmittedWhenMissing(t *testing.T) {
	in := Stats{GPUData: map[string]GPUData{"0": {Name: "GPU"}}}
	data, err := cbor.Marshal(in)
	require.NoError(t, err)
	var keys map[int]cbor.RawMessage
	require.NoError(t, cbor.Unmarshal(data, &keys))
	assert.NotContains(t, keys, 41)

	gpuKeys := decodeGPUKeys(t, keys)
	for _, k := range []int{7, 8, 9} {
		assert.NotContains(t, gpuKeys["0"], k)
	}

	for name, marshal := range map[string]func(any) ([]byte, error){
		"json":   json.Marshal,
		"jsonv2": func(v any) ([]byte, error) { return jsonv2.Marshal(v) },
	} {
		t.Run(name, func(t *testing.T) {
			b, err := marshal(in)
			require.NoError(t, err)
			var m map[string]any
			require.NoError(t, json.Unmarshal(b, &m))
			assert.NotContains(t, m, "mbw")
			assert.NotContains(t, m, "mbwm")
			g := m["g"].(map[string]any)["0"].(map[string]any)
			for _, k := range []string{"bw", "bwp", "bwa"} {
				assert.NotContains(t, g, k)
			}

			b, err = marshal(Stats{
				MemBandwidth: [2]float64{1.5, 0.5}, MaxMemBandwidth: [2]float64{3, 1},
				GPUData: map[string]GPUData{"0": {Name: "GPU", MemBandwidth: 10, MemBandwidthPeak: 448.03, MemBandwidthCount: 2}},
			})
			require.NoError(t, err)
			m = nil
			require.NoError(t, json.Unmarshal(b, &m))
			assert.Equal(t, []any{1.5, 0.5}, m["mbw"])
			assert.Equal(t, []any{3.0, 1.0}, m["mbwm"])
			g = m["g"].(map[string]any)["0"].(map[string]any)
			assert.Equal(t, 10.0, g["bw"])
			assert.Equal(t, 448.03, g["bwp"])
			assert.NotContains(t, g, "bwa")
			assert.NotContains(t, g, "MemBandwidthCount")
		})
	}
}

func TestMemBandwidthWireKeys(t *testing.T) {
	t.Run("bandwidth keys sit beside memory bytes", func(t *testing.T) {
		in := Stats{
			Cpu:                   7,
			MemBytes:              [4]uint64{64 << 30, 8 << 30, 2 << 30, 0},
			SwapBytes:             [2]uint64{4 << 30, 1 << 30},
			MemBandwidth:          [2]float64{9.5, 3.25},
			MemBandwidthTotalOnly: true,
		}
		wire, err := cbor.Marshal(in)
		require.NoError(t, err)
		var keys map[int]cbor.RawMessage
		require.NoError(t, cbor.Unmarshal(wire, &keys))
		assert.Contains(t, keys, 43)
		assert.Contains(t, keys, 44)
		var out Stats
		require.NoError(t, cbor.Unmarshal(wire, &out))
		assert.Equal(t, in.MemBytes, out.MemBytes)
		assert.Equal(t, in.SwapBytes, out.SwapBytes)
		assert.Equal(t, in.MemBandwidth, out.MemBandwidth)
		assert.True(t, out.MemBandwidthTotalOnly)
	})

	t.Run("integral float values stay floats", func(t *testing.T) {
		wire, err := cbor.Marshal(Stats{MemBandwidth: [2]float64{2, 1}})
		require.NoError(t, err)
		var out Stats
		require.NoError(t, cbor.Unmarshal(wire, &out))
		assert.Equal(t, [2]float64{2, 1}, out.MemBandwidth)
	})

	t.Run("older hubs ignore the new keys", func(t *testing.T) {
		type olderHubStats struct {
			Cpu float64 `cbor:"0,keyasint"`
		}
		wire, err := cbor.Marshal(Stats{Cpu: 12, MemBandwidth: [2]float64{80, 0}, MemBandwidthTotalOnly: true})
		require.NoError(t, err)
		var older olderHubStats
		require.NoError(t, cbor.Unmarshal(wire, &older))
		assert.Equal(t, 12.0, older.Cpu)
	})
}

func decodeGPUKeys(t *testing.T, stats map[int]cbor.RawMessage) map[string]map[int]any {
	t.Helper()
	var gpus map[string]map[int]any
	require.NoError(t, cbor.Unmarshal(stats[22], &gpus))
	return gpus
}
