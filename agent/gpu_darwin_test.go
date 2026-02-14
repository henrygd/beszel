//go:build darwin

package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePowermetricsData(t *testing.T) {
	input := `
Machine model: Mac14,10
OS version: 25D125

*** Sampled system activity (Sat Feb 14 00:42:06 2026 -0500) (503.05ms elapsed) ***

**** GPU usage ****

GPU HW active frequency: 444 MHz
GPU HW active residency:   0.97% (444 MHz: .97% 612 MHz:   0% 808 MHz:   0% 968 MHz:   0% 1110 MHz:   0% 1236 MHz:   0% 1338 MHz:   0% 1398 MHz:   0%)
GPU SW requested state: (P1 : 100% P2 :   0% P3 :   0% P4 :   0% P5 :   0% P6 :   0% P7 :   0% P8 :   0%)
GPU idle residency:  99.03%
GPU Power: 4 mW
`
	gm := &GPUManager{
		GpuDataMap: make(map[string]*system.GPUData),
	}
	valid := gm.parsePowermetricsData([]byte(input))
	require.True(t, valid)

	g0, ok := gm.GpuDataMap["0"]
	require.True(t, ok)
	assert.Equal(t, "Apple GPU", g0.Name)
	// Usage = 100 - 99.03 = 0.97
	assert.InDelta(t, 0.97, g0.Usage, 0.01)
	// 4 mW -> 0.004 W
	assert.InDelta(t, 0.004, g0.Power, 0.0001)
	assert.Equal(t, 1.0, g0.Count)
}

func TestParsePowermetricsDataPartial(t *testing.T) {
	// Only power line (e.g. older macOS or different sampler output)
	input := `
**** GPU usage ****
GPU Power: 120 mW
`
	gm := &GPUManager{
		GpuDataMap: make(map[string]*system.GPUData),
	}
	valid := gm.parsePowermetricsData([]byte(input))
	require.True(t, valid)

	g0, ok := gm.GpuDataMap["0"]
	require.True(t, ok)
	assert.Equal(t, "Apple GPU", g0.Name)
	assert.InDelta(t, 0.12, g0.Power, 0.001)
	assert.Equal(t, 1.0, g0.Count)
}
