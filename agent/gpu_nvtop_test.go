package agent

import (
	"testing"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNvtopEncodeDecode(t *testing.T) {
	gm := &GPUManager{GpuDataMap: make(map[string]*system.GPUData)}
	// The device-level value from #2740 must be used, not the process value.
	require.True(t, gm.parseNvtopData([]byte(`[{"device_name":"Arrow Lake-S (Intel Graphics)","gpu_util":"0%","encode_decode":"100%","processes":[{"encode_decode":"106%"}]}]`)))
	first := gm.GetCurrentData(1)["n0"]
	assert.Equal(t, 100.0, first.Usage)
	assert.Equal(t, map[string]float64{"ENC/DEC": 100}, first.Engines)

	// Average new samples per cache key, keeping 3D usage when it is busier.
	require.True(t, gm.parseNvtopData([]byte(`[{"device_name":"Arrow Lake-S (Intel Graphics)","gpu_util":"80%","encode_decode":"20%"}]`)))
	require.True(t, gm.parseNvtopData([]byte(`[{"device_name":"Arrow Lake-S (Intel Graphics)","gpu_util":"40%","encode_decode":"0%"}]`)))
	next := gm.GetCurrentData(1)["n0"]
	assert.Equal(t, 60.0, next.Usage)
	assert.Equal(t, map[string]float64{"ENC/DEC": 10}, next.Engines)
	assert.Equal(t, 100.0, first.Engines["ENC/DEC"], "averaging must not mutate earlier results")
	other := gm.GetCurrentData(2)["n0"]
	assert.Equal(t, 40.0, other.Usage)
	assert.Equal(t, 40.0, other.Engines["ENC/DEC"])

	require.True(t, gm.parseNvtopData([]byte(`[{"device_name":"Arrow Lake-S (Intel Graphics)","gpu_util":"0%","encode_decode":"0%"}]`)))
	idle := gm.GetCurrentData(1)["n0"]
	assert.Zero(t, idle.Usage)
	assert.Zero(t, idle.Engines["ENC/DEC"])
}

func TestNvtopWithoutEncodeDecode(t *testing.T) {
	for _, payload := range []string{
		`[{"device_name":"GPU","gpu_util":"30%"}]`,
		`[{"device_name":"GPU","gpu_util":"30%","encode_decode":null}]`,
	} {
		gm := &GPUManager{GpuDataMap: make(map[string]*system.GPUData)}
		require.True(t, gm.parseNvtopData([]byte(payload)))
		gpu := gm.GetCurrentData(1)["n0"]
		assert.Equal(t, 30.0, gpu.Usage)
		assert.Nil(t, gpu.Engines)

		// A later sample may start reporting the media engine.
		require.True(t, gm.parseNvtopData([]byte(`[{"device_name":"GPU","gpu_util":"10%","encode_decode":"80%"}]`)))
		gpu = gm.GetCurrentData(1)["n0"]
		assert.Equal(t, 80.0, gpu.Usage)
		assert.Equal(t, 80.0, gpu.Engines["ENC/DEC"])

		// Missing or null readings must not reuse the previous media load.
		require.True(t, gm.parseNvtopData([]byte(payload)))
		gpu = gm.GetCurrentData(1)["n0"]
		assert.Equal(t, 30.0, gpu.Usage)
		assert.Zero(t, gpu.Engines["ENC/DEC"])
	}
}

func TestNvtopSeparateEncodeDecode(t *testing.T) {
	gm := &GPUManager{GpuDataMap: make(map[string]*system.GPUData)}
	// nvtop emits separate encode/decode fields when the device's media engines are not shared.
	require.True(t, gm.parseNvtopData([]byte(`[{"device_name":"NVIDIA GeForce RTX 3050","gpu_util":"5%","encode":"30%","decode":"70%"}]`)))
	gpu := gm.GetCurrentData(1)["n0"]
	assert.Equal(t, 70.0, gpu.Usage)
	assert.Equal(t, map[string]float64{"ENC": 30, "DEC": 70}, gpu.Engines)

	require.True(t, gm.parseNvtopData([]byte(`[{"device_name":"NVIDIA GeForce RTX 3050","gpu_util":"50%","encode":"10%","decode":null}]`)))
	gpu = gm.GetCurrentData(1)["n0"]
	assert.Equal(t, 50.0, gpu.Usage)
	assert.Equal(t, map[string]float64{"ENC": 10, "DEC": 0}, gpu.Engines)
}
