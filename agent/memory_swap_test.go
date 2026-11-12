package agent

import (
	"testing"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/stretchr/testify/assert"
)

func TestUpdateMemoryStatsDarwinSwap(t *testing.T) {
	origVM, origSwap, origOS := memoryVirtualMemory, memorySwapMemory, memoryGOOS
	t.Cleanup(func() { memoryVirtualMemory, memorySwapMemory, memoryGOOS = origVM, origSwap, origOS })
	const gib = uint64(1 << 30)
	memoryVirtualMemory = func() (*mem.VirtualMemoryStat, error) {
		return &mem.VirtualMemoryStat{Total: 16 * gib, Used: 8 * gib}, nil
	}
	memorySwapMemory = func() (*mem.SwapMemoryStat, error) {
		return &mem.SwapMemoryStat{Total: 4 * gib, Free: gib}, nil
	}

	memoryGOOS = "darwin"
	var stats system.Stats
	(&Agent{}).updateMemoryStats(&stats)
	assert.Equal(t, [2]uint64{4 * gib, 3 * gib}, stats.SwapBytes)
	assert.Equal(t, float64(4), stats.Swap)
	assert.Equal(t, float64(3), stats.SwapUsed)

	// Other platforms keep the VirtualMemory values.
	memoryGOOS = "freebsd"
	stats = system.Stats{}
	(&Agent{}).updateMemoryStats(&stats)
	assert.Zero(t, stats.SwapBytes)
}
