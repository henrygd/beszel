package agent

import (
	"strings"
	"testing"

	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatherStatsDoesNotAttachDetailsToCachedRequests(t *testing.T) {
	agent := &Agent{
		cache:         NewSystemDataCache(),
		systemDetails: system.Details{Hostname: "updated-host", Podman: true},
		detailsDirty:  true,
	}
	cached := &system.CombinedData{
		Info: system.Info{Hostname: "cached-host"},
	}
	agent.cache.Set(cached, defaultDataCacheTimeMs)

	response := agent.gatherStats(common.DataRequestOptions{CacheTimeMs: defaultDataCacheTimeMs})

	assert.Same(t, cached, response)
	assert.Nil(t, response.Details)
	assert.True(t, agent.detailsDirty)
	assert.Equal(t, "cached-host", response.Info.Hostname)
	assert.Nil(t, cached.Details)

	secondResponse := agent.gatherStats(common.DataRequestOptions{CacheTimeMs: defaultDataCacheTimeMs})
	assert.Same(t, cached, secondResponse)
	assert.Nil(t, secondResponse.Details)
}

func TestCalculateHostMemoryUsage(t *testing.T) {
	tests := []struct {
		name                      string
		memory                    mem.VirtualMemoryStat
		htop                      bool
		used, cacheBuff, swapUsed uint64
	}{
		{
			name:      "normal",
			memory:    mem.VirtualMemoryStat{Total: 100, Available: 40, Used: 60, Free: 20, Cached: 25, Buffers: 10, Shared: 5, SwapTotal: 20, SwapFree: 8, SwapCached: 2},
			used:      60,
			cacheBuff: 30,
			swapUsed:  12,
		},
		{
			name:      "inconsistent counters saturate",
			memory:    mem.VirtualMemoryStat{Total: 100, Available: 110, Used: ^uint64(0) - 9, Free: 90, Cached: 5, Buffers: 10, Shared: 20, SwapTotal: 10, SwapFree: 9, SwapCached: 2},
			used:      0,
			cacheBuff: 0,
			swapUsed:  1,
		},
		{
			name:      "htop subtraction saturates",
			memory:    mem.VirtualMemoryStat{Total: 100, Available: 20, Used: 80, Free: 90, Cached: 20, Buffers: 5, SwapTotal: 30, SwapFree: 10, SwapCached: 5},
			htop:      true,
			used:      0,
			cacheBuff: 25,
			swapUsed:  20,
		},
		{
			name:      "zero cache from shared cancellation does not fall back",
			memory:    mem.VirtualMemoryStat{Total: 100, Used: 60, Free: 10, Cached: 20, Buffers: 10, Shared: 30},
			used:      60,
			cacheBuff: 0,
		},
		{
			name:      "absent cache counters use fallback",
			memory:    mem.VirtualMemoryStat{Total: 100, Used: 60, Free: 10},
			used:      60,
			cacheBuff: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			used, cacheBuff, swapUsed := calculateHostMemoryUsage(&tt.memory, tt.htop)
			assert.Equal(t, tt.used, used)
			assert.Equal(t, tt.cacheBuff, cacheBuff)
			assert.Equal(t, tt.swapUsed, swapUsed)
		})
	}
}

func TestUpdateSystemDetailsMarksDetailsDirty(t *testing.T) {
	agent := &Agent{}

	agent.updateSystemDetails(func(details *system.Details) {
		details.Hostname = "updated-host"
		details.Podman = true
	})

	assert.True(t, agent.detailsDirty)
	assert.Equal(t, "updated-host", agent.systemDetails.Hostname)
	assert.True(t, agent.systemDetails.Podman)

	original := &system.CombinedData{}
	realTimeResponse := agent.attachSystemDetails(original, 1000, true)
	assert.Same(t, original, realTimeResponse)
	assert.Nil(t, realTimeResponse.Details)
	assert.True(t, agent.detailsDirty)

	response := agent.attachSystemDetails(original, defaultDataCacheTimeMs, false)
	require.NotNil(t, response.Details)
	assert.NotSame(t, original, response)
	assert.Equal(t, "updated-host", response.Details.Hostname)
	assert.True(t, response.Details.Podman)
	assert.False(t, agent.detailsDirty)
	assert.Nil(t, original.Details)
}

func TestParseCpuModel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "MIPS with both cpu model and system type",
			input: `system type             : MediaTek MT7621 ver:1 eco:3
machine                 : ASUS RT-AX53U
processor               : 0
cpu model               : MIPS 1004Kc V2.15
BogoMIPS                : 586.13
wait instruction        : yes`,
			expected: "MIPS 1004Kc V2.15 / MediaTek MT7621 ver:1 eco:3",
		},
		{
			name: "MIPS with different SoC",
			input: `system type             : Atheros AR7161 rev 2
machine                 : NETGEAR WNDR3700
processor               : 0
cpu model               : MIPS 24Kc V7.4
BogoMIPS                : 452.19`,
			expected: "MIPS 24Kc V7.4 / Atheros AR7161 rev 2",
		},
		{
			name: "only system type when cpu model missing",
			input: `system type             : Broadcom BCM47xx
processor               : 0
BogoMIPS                : 296.11`,
			expected: "Broadcom BCM47xx",
		},
		{
			name: "only cpu model when system type missing",
			input: `processor               : 0
cpu model               : MIPS 34Kc V2.15
BogoMIPS                : 300.00`,
			expected: "MIPS 34Kc V2.15",
		},
		{
			name: "x86 cpuinfo returns empty",
			input: `processor	: 0
vendor_id	: GenuineIntel
cpu family	: 6
model		: 142
model name	: Intel(R) Core(TM) i5-8250U CPU @ 1.60GHz
stepping	: 10`,
			expected: "",
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name: "cpu model with extra whitespace",
			input: `processor               : 0
cpu model               :   MIPS 34Kc V2.15
BogoMIPS                : 300.00`,
			expected: "MIPS 34Kc V2.15",
		},
		{
			name: "cpu model without value",
			input: `processor               : 0
cpu model               :
BogoMIPS                : 300.00`,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCpuModel(strings.NewReader(tt.input))
			assert.Equal(t, tt.expected, result)
		})
	}
}
