package records

import (
	"testing"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
)

func TestAverageSystemStatsSliceMemBandwidth(t *testing.T) {
	input := []system.Stats{
		{Cpu: 1, MemBandwidth: [2]float64{10, 4}, MaxMemBandwidth: [2]float64{30, 5}},
		{Cpu: 1}, // agent without counters, or first tick: must not drag the average down
		{Cpu: 1, MemBandwidth: [2]float64{20, 8}},
	}
	result := AverageSystemStatsSlice(input)
	assert.Equal(t, [2]float64{15, 6}, result.MemBandwidth)
	assert.Equal(t, [2]float64{30, 8}, result.MaxMemBandwidth)

	assert.Equal(t, [2]float64{}, AverageSystemStatsSlice([]system.Stats{{Cpu: 1}}).MemBandwidth)
	assert.False(t, result.MemBandwidthTotalOnly)

	// total-only sources (Jetson) keep the flag through averaging
	totalOnly := AverageSystemStatsSlice([]system.Stats{
		{Cpu: 1, MemBandwidth: [2]float64{40, 0}, MemBandwidthTotalOnly: true},
		{Cpu: 1, MemBandwidth: [2]float64{60, 0}, MemBandwidthTotalOnly: true},
	})
	assert.Equal(t, [2]float64{50, 0}, totalOnly.MemBandwidth)
	assert.True(t, totalOnly.MemBandwidthTotalOnly)

	// A source change within a rollup preserves both contributions to total.
	mixed := AverageSystemStatsSlice([]system.Stats{
		{MemBandwidth: [2]float64{40, 0}, MemBandwidthTotalOnly: true},
		{MemBandwidth: [2]float64{10, 4}},
	})
	assert.True(t, mixed.MemBandwidthTotalOnly)
	assert.Equal(t, [2]float64{25, 2}, mixed.MemBandwidth)
	assert.Equal(t, 27.0, mixed.MemBandwidth[0]+mixed.MemBandwidth[1])
	assert.Equal(t, [2]float64{40, 4}, mixed.MaxMemBandwidth, "independent peaks are not a total peak")
}

func TestAverageSystemStatsSliceGPUMemBandwidth(t *testing.T) {
	input := []system.Stats{
		{GPUData: map[string]system.GPUData{
			"0": {Name: "A", MemBandwidth: 900, MemBandwidthPeak: 1792.13},
			"1": {Name: "B", MemBandwidth: 100, MemBandwidthPeak: 936.05, MemBandwidthApprox: true},
		}},
		// value 0 with peak present is a real zero sample
		{GPUData: map[string]system.GPUData{"0": {Name: "A", MemBandwidthPeak: 1792.13}, "1": {Name: "B"}}},
		{GPUData: map[string]system.GPUData{"0": {Name: "A", MemBandwidth: 300, MemBandwidthPeak: 1792.13}}},
	}
	result := AverageSystemStatsSlice(input)
	a, b := result.GPUData["0"], result.GPUData["1"]
	assert.Equal(t, 400.0, a.MemBandwidth)
	assert.Equal(t, 1792.13, a.MemBandwidthPeak)
	assert.False(t, a.MemBandwidthApprox)
	// GPU B had one sample; the record without a peak is not a zero sample
	assert.Equal(t, 100.0, b.MemBandwidth)
	assert.Equal(t, 936.05, b.MemBandwidthPeak)
	assert.True(t, b.MemBandwidthApprox)
}
