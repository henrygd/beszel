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

func TestApplyContainerMemoryLimit(t *testing.T) {
	const hostTotal = uint64(128) << 30 // 128 GiB, as /proc/meminfo reports it from inside an LXC

	t.Run("cgroup usage overrides host figures when runtime total corroborates", func(t *testing.T) {
		withCgroupRoots(t, map[string]string{
			"memory.max":     "268435456", // 256 MiB
			"memory.current": "200000000",
			"memory.stat":    "file 80000000\n",
		}, nil)
		a := &Agent{systemDetails: system.Details{MemoryTotal: 268435456}}
		v := &mem.VirtualMemoryStat{Total: hostTotal}
		used, cacheBuff := uint64(40)<<30, uint64(70)<<30

		a.applyContainerMemoryLimit(v, &used, &cacheBuff)

		assert.Equal(t, uint64(268435456), v.Total)
		assert.Equal(t, uint64(120000000), used) // memory.current - memory.stat:file
		assert.Equal(t, uint64(80000000), cacheBuff)
		assert.Equal(t, uint64(268435456-120000000), v.Available)
	})

	t.Run("tighter cgroup limit wins over runtime total", func(t *testing.T) {
		withCgroupRoots(t, map[string]string{
			"memory.max":     "268435456", // 256 MiB cgroup cap
			"memory.current": "100000000",
			"memory.stat":    "file 10000000\n",
		}, nil)
		a := &Agent{systemDetails: system.Details{MemoryTotal: 536870912}} // runtime says 512 MiB
		v := &mem.VirtualMemoryStat{Total: hostTotal}
		used, cacheBuff := uint64(40)<<30, uint64(70)<<30

		a.applyContainerMemoryLimit(v, &used, &cacheBuff)

		assert.Equal(t, uint64(268435456), v.Total)
		assert.Equal(t, uint64(90000000), used)
	})

	t.Run("falls back to runtime total and scales usage without cgroup accounting", func(t *testing.T) {
		withCgroupRoots(t, nil, nil) // Docker in an unprivileged LXC: no cgroup memory controller
		a := &Agent{systemDetails: system.Details{MemoryTotal: 268435456}}
		v := &mem.VirtualMemoryStat{Total: hostTotal}
		used, cacheBuff := hostTotal/2, hostTotal/4

		a.applyContainerMemoryLimit(v, &used, &cacheBuff)

		assert.Equal(t, uint64(268435456), v.Total)
		// scaled by limit/hostTotal, so still ~50% used / ~25% cache of the new total
		assert.InEpsilon(t, float64(v.Total)/2, float64(used), 0.01)
		assert.InEpsilon(t, float64(v.Total)/4, float64(cacheBuff), 0.01)
		assert.LessOrEqual(t, used, v.Total)
	})

	t.Run("no runtime total: host figures left untouched", func(t *testing.T) {
		withCgroupRoots(t, map[string]string{
			"memory.max":     "268435456",
			"memory.current": "100000000",
			"memory.stat":    "file 0\n",
		}, nil)
		a := &Agent{} // no Docker/Podman -> systemDetails.MemoryTotal == 0
		v := &mem.VirtualMemoryStat{Total: hostTotal}
		used, cacheBuff := uint64(1000), uint64(2000)

		a.applyContainerMemoryLimit(v, &used, &cacheBuff)

		assert.Equal(t, hostTotal, v.Total)
		assert.Equal(t, uint64(1000), used)
		assert.Equal(t, uint64(2000), cacheBuff)
	})

	t.Run("memory-limited agent on a normal host is left untouched", func(t *testing.T) {
		// /proc/meminfo is the real host total, so the runtime reports the same;
		// the cgroup cap is the agent's own --memory limit and must be ignored.
		withCgroupRoots(t, map[string]string{
			"memory.max":     "268435456",
			"memory.current": "100000000",
			"memory.stat":    "file 0\n",
		}, nil)
		a := &Agent{systemDetails: system.Details{MemoryTotal: hostTotal}}
		v := &mem.VirtualMemoryStat{Total: hostTotal}
		used, cacheBuff := uint64(1000), uint64(2000)

		a.applyContainerMemoryLimit(v, &used, &cacheBuff)

		assert.Equal(t, hostTotal, v.Total)
		assert.Equal(t, uint64(1000), used)
	})

	t.Run("runtime total within 1% of host total is ignored", func(t *testing.T) {
		withCgroupRoots(t, nil, nil)
		a := &Agent{systemDetails: system.Details{MemoryTotal: 137000000000}} // ~99.7% of 128 GiB
		v := &mem.VirtualMemoryStat{Total: hostTotal}
		used, cacheBuff := uint64(1000), uint64(2000)

		a.applyContainerMemoryLimit(v, &used, &cacheBuff)

		assert.Equal(t, hostTotal, v.Total)
		assert.Equal(t, uint64(1000), used)
	})
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
