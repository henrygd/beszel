//go:build testing

package agent

import (
	"testing"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Clock values are nvidia-smi "Max Clocks: Memory" (NVML_CLOCK_MEM) as
// reported for each card; expected bandwidth is the vendor spec.
func TestNvmlPeakMemBandwidth(t *testing.T) {
	tests := []struct {
		name     string
		clockMHz uint32
		busWidth uint32
		specGBps float64
	}{
		{"RTX PRO 6000 Blackwell (GDDR7 28 Gbps, 512-bit)", 14001, 512, 1792},
		{"RTX 5090 (GDDR7 28 Gbps, 512-bit)", 14001, 512, 1792},
		{"RTX 5060 Ti (GDDR7 28 Gbps, 128-bit)", 14001, 128, 448},
		{"RTX 3090 (GDDR6X 19.5 Gbps, 384-bit)", 9751, 384, 936},
		{"RTX 4090 (GDDR6X 21 Gbps, 384-bit)", 10501, 384, 1008},
		{"A100 80GB SXM (HBM2e, 5120-bit)", 1593, 5120, 2039},
		{"H100 SXM (HBM3, 5120-bit)", 2619, 5120, 3352},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nvmlPeakMemBandwidthGBps(tt.clockMHz, tt.busWidth)
			// within 0.1% of spec (NVML clocks are rounded to whole MHz)
			assert.InEpsilon(t, tt.specGBps, got, 0.001)
		})
	}
	assert.Equal(t, 0.0, nvmlPeakMemBandwidthGBps(0, 384))
	assert.Equal(t, 0.0, nvmlPeakMemBandwidthGBps(9751, 0))
}

func TestGpuMemBandwidthFromUtil(t *testing.T) {
	assert.Equal(t, 896.0, gpuMemBandwidthFromUtil(50, 1792))
	assert.Equal(t, 1792.0, gpuMemBandwidthFromUtil(130, 1792))
	assert.Equal(t, 0.0, gpuMemBandwidthFromUtil(-1, 1792))
	assert.Equal(t, 0.0, gpuMemBandwidthFromUtil(50, 0))
}

func TestGPUManagerAveragesMemBandwidth(t *testing.T) {
	gm := &GPUManager{GpuDataMap: map[string]*system.GPUData{
		"0": {Name: "RTX PRO 6000", Temperature: 50, Usage: 30, Count: 3,
			MemBandwidth: 600, MemBandwidthCount: 2, MemBandwidthPeak: 1792.13},
		"1": {Name: "RTX 3090", Temperature: 40, Usage: 10, Count: 1,
			MemBandwidth: 93.6, MemBandwidthCount: 1, MemBandwidthPeak: 936.05, MemBandwidthApprox: true},
		"2": {Name: "Other", Temperature: 40, Usage: 10, Count: 1},
	}}

	data := gm.GetCurrentData(5000)
	assert.Equal(t, 300.0, data["0"].MemBandwidth)
	assert.Equal(t, 1792.13, data["0"].MemBandwidthPeak)
	assert.False(t, data["0"].MemBandwidthApprox)
	assert.Equal(t, 0.0, data["0"].MemBandwidthCount)
	assert.Equal(t, 93.6, data["1"].MemBandwidth)
	assert.True(t, data["1"].MemBandwidthApprox)
	assert.Equal(t, system.GPUData{Name: "Other", Temperature: 40, Usage: 10, Count: 1}, data["2"])

	// new usage samples but no bandwidth samples: bandwidth fields are cleared
	gpu := gm.GpuDataMap["0"]
	gpu.Usage += 20
	gpu.Count++
	data = gm.GetCurrentData(5000)
	assert.Equal(t, 20.0, data["0"].Usage)
	assert.Zero(t, data["0"].MemBandwidth)
	assert.Zero(t, data["0"].MemBandwidthPeak)

	// one zero-valued sample: value 0 (omitted on the wire) with peak present
	gpu.Count++
	gpu.MemBandwidthCount++
	data = gm.GetCurrentData(5000)
	assert.Zero(t, data["0"].MemBandwidth)
	assert.Equal(t, 1792.13, data["0"].MemBandwidthPeak)

	// an independent cache key averages from its own start
	data = gm.GetCurrentData(60000)
	require.Contains(t, data, "0")
	assert.Equal(t, 200.0, data["0"].MemBandwidth)
}
