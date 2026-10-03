//go:build testing && linux

package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupMemoryCgroup(t *testing.T, v2 bool) string {
	t.Helper()
	swapCpuContainerSeams(t)
	root, mountinfo, selfCgroup := memoryCgroupRoot, memoryCgroupMountinfo, memoryProcSelfCgroup
	t.Cleanup(func() {
		memoryCgroupRoot, memoryCgroupMountinfo, memoryProcSelfCgroup = root, mountinfo, selfCgroup
	})
	tmp := t.TempDir()
	memoryCgroupRoot = filepath.Join(tmp, "cgroup")
	memoryCgroupMountinfo = filepath.Join(tmp, "mountinfo")
	memoryProcSelfCgroup = filepath.Join(tmp, "self-cgroup")
	if v2 {
		writeCpuFixture(t, memoryProcSelfCgroup, "0::/system.slice/agent.service\n")
		writeCpuFixture(t, memoryCgroupMountinfo, "")
		return memoryCgroupRoot
	}
	dir := filepath.Join(tmp, "memory")
	writeCpuFixture(t, memoryProcSelfCgroup, "2:memory:/system.slice/agent.service\n")
	writeCpuFixture(t, memoryCgroupMountinfo,
		"30 25 0:26 / "+dir+" rw - cgroup cgroup rw,memory\n")
	return dir
}

func writeMemoryFixture(t *testing.T, dir string, v2 bool, usage, limit, stat string) {
	t.Helper()
	usageName, limitName := "memory.usage_in_bytes", "memory.limit_in_bytes"
	if v2 {
		usageName, limitName = "memory.current", "memory.max"
	}
	writeCpuFixture(t, filepath.Join(dir, usageName), usage)
	writeCpuFixture(t, filepath.Join(dir, limitName), limit)
	writeCpuFixture(t, filepath.Join(dir, "memory.stat"), stat)
}

func TestContainerMemoryMetrics(t *testing.T) {
	for _, v2 := range []bool{true, false} {
		t.Run(strconv.FormatBool(v2), func(t *testing.T) {
			dir := setupMemoryCgroup(t, v2)
			key, shmemKey := "total_cache", "total_shmem"
			if v2 {
				key, shmemKey = "file", "shmem"
			}
			stat := func(cache, shmem string) string {
				return key + " " + cache + "\n" + shmemKey + " " + shmem + "\n"
			}
			for _, tt := range []struct {
				name, usage, limit, stat string
				host                     uint64
				want                     memoryMetrics
				ok                       bool
			}{
				{"file cache excluded", "600", "1000", stat("200", "0"), 2000, memoryMetrics{1000, 400, 200}, true},
				{"shared memory retained", "600", "1000", stat("200", "50"), 2000, memoryMetrics{1000, 450, 150}, true},
				{"shared memory larger than cache", "600", "1000", stat("200", "300"), 2000, memoryMetrics{1000, 600, 0}, true},
				{"zero usage", "0", "1000", stat("0", "0"), 2000, memoryMetrics{1000, 0, 0}, true},
				{"cache larger than charge", "100", "1000", stat("200", "0"), 2000, memoryMetrics{1000, 0, 100}, true},
				{"host caps limit", "600", "3000", stat("200", "0"), 2000, memoryMetrics{2000, 400, 200}, true},
				{"finite without host", "600", "1000", stat("200", "0"), 0, memoryMetrics{1000, 400, 200}, true},
				{"missing cache key", "600", "1000", shmemKey + " 0\n", 2000, memoryMetrics{}, false},
				{"missing shared memory key", "600", "1000", key + " 200\n", 2000, memoryMetrics{}, false},
				{"invalid cache", "600", "1000", stat("-1", "0"), 2000, memoryMetrics{}, false},
				{"invalid shared memory", "600", "1000", stat("200", "-1"), 2000, memoryMetrics{}, false},
				{"invalid usage", "bad", "1000", stat("200", "0"), 2000, memoryMetrics{}, false},
				{"invalid limit", "600", "bad", stat("200", "0"), 2000, memoryMetrics{}, false},
				{"zero limit", "600", "0", stat("200", "0"), 2000, memoryMetrics{}, false},
			} {
				t.Run(tt.name, func(t *testing.T) {
					writeMemoryFixture(t, dir, v2, tt.usage, tt.limit, tt.stat)
					metrics, ok := containerMemoryMetrics(tt.host, true)
					assert.Equal(t, tt.ok, ok)
					assert.Equal(t, tt.want, metrics)
				})
			}
			limit := "9223372036854771712"
			if v2 {
				limit = "max"
			}
			writeMemoryFixture(t, dir, v2, "600", limit, stat("200", "0"))
			writeMemoryFixture(t, filepath.Join(dir, "system.slice/agent.service"), v2, "1", "100", stat("0", "0"))
			metrics, ok := containerMemoryMetrics(2000, true)
			require.True(t, ok)
			assert.Equal(t, memoryMetrics{2000, 400, 200}, metrics)
			_, ok = containerMemoryMetrics(0, true)
			assert.False(t, ok)
			if !v2 {
				writeMemoryFixture(t, dir, false, "600", "2147479552", stat("200", "0"))
				metrics, ok = containerMemoryMetrics(2000, true)
				require.True(t, ok)
				assert.EqualValues(t, 2000, metrics.Total)
			}
		})
	}
}

func TestContainerMemoryScopeAndEnablement(t *testing.T) {
	dir := setupMemoryCgroup(t, true)
	// A discovered non-default mount must win over the default root.
	mount := filepath.Join(t.TempDir(), "unified")
	writeCpuFixture(t, memoryCgroupMountinfo,
		"30 25 0:26 /guest "+mount+" rw - cgroup2 cgroup2 rw\n")
	writeMemoryFixture(t, dir, true, "1", "1000", "file 0\nshmem 0\n")
	writeMemoryFixture(t, mount, true, "600", "1000", "file 200\nshmem 0\n")
	writeMemoryFixture(t, filepath.Join(mount, "system.slice/agent.service"), true, "10", "100", "file 0\nshmem 0\n")
	_, ok := containerMemoryMetrics(2000, false)
	assert.False(t, ok) // Docker/host agents keep the existing path by default.
	markLxc(t)
	inLxc = sync.OnceValue(detectLxc) // Simulate a fresh process in LXC.
	metrics, ok := containerMemoryMetrics(2000, false)
	require.True(t, ok)
	assert.Equal(t, memoryMetrics{1000, 400, 200}, metrics)
	require.NoError(t, os.Remove(filepath.Join(mount, "memory.stat")))
	_, ok = containerMemoryMetrics(2000, true)
	assert.False(t, ok)
}

func TestUpdateMemoryStatsCgroupAndFallback(t *testing.T) {
	dir := setupMemoryCgroup(t, true)
	const gib = uint64(1 << 30)
	writeMemoryFixture(t, dir, true, strconv.FormatUint(6*gib, 10), strconv.FormatUint(10*gib, 10),
		"file "+strconv.FormatUint(2*gib, 10)+"\nshmem 0\n")
	original := memoryVirtualMemory
	t.Cleanup(func() { memoryVirtualMemory = original })
	memoryVirtualMemory = func() (*mem.VirtualMemoryStat, error) {
		return &mem.VirtualMemoryStat{Total: 20 * gib, Used: 12 * gib, Free: 4 * gib,
			Cached: 4 * gib, SwapTotal: 2 * gib, SwapFree: gib}, nil
	}
	a := &Agent{forceUseCgroup: true, memCalc: "htop", zfs: true}
	var stats system.Stats
	a.updateMemoryStats(&stats)
	assert.Equal(t, float64(10), stats.Mem)
	assert.Equal(t, float64(4), stats.MemUsed)
	assert.Equal(t, float64(2), stats.MemBuffCache)
	assert.Equal(t, float64(40), stats.MemPct)
	assert.Zero(t, stats.MemZfsArc)
	assert.Equal(t, float64(2), stats.Swap)
	assert.Equal(t, float64(1), stats.SwapUsed)
	assert.Equal(t, 10*gib, a.systemDetails.MemoryTotal)
	assert.True(t, a.detailsDirty)

	a.detailsDirty = false
	a.updateMemoryStats(&system.Stats{})
	assert.False(t, a.detailsDirty)
	writeCpuFixture(t, filepath.Join(dir, "memory.max"), strconv.FormatUint(8*gib, 10))
	a.updateMemoryStats(&system.Stats{})
	assert.Equal(t, 8*gib, a.systemDetails.MemoryTotal)
	assert.True(t, a.detailsDirty)

	// A missing required field falls back to the original htop calculation.
	writeCpuFixture(t, filepath.Join(dir, "memory.stat"), "file 123\n")
	a.zfs = false
	stats = system.Stats{}
	a.updateMemoryStats(&stats)
	assert.Equal(t, float64(20), stats.Mem)
	assert.Equal(t, float64(12), stats.MemUsed)
	assert.Equal(t, float64(4), stats.MemBuffCache)
	assert.Equal(t, float64(60), stats.MemPct)
	assert.Equal(t, 20*gib, a.systemDetails.MemoryTotal)

	// Finite cgroup limits still work if /proc/meminfo cannot be read.
	memoryVirtualMemory = func() (*mem.VirtualMemoryStat, error) { return nil, errors.New("unreadable") }
	writeCpuFixture(t, filepath.Join(dir, "memory.stat"), "file 0\nshmem 0\n")
	stats = system.Stats{}
	a.updateMemoryStats(&stats)
	assert.Equal(t, float64(8), stats.Mem)
	assert.Equal(t, float64(6), stats.MemUsed)
	assert.Equal(t, float64(75), stats.MemPct)
	assert.Zero(t, stats.Swap)
}

func TestMemoryDetailsInitiallyUseCgroupLimit(t *testing.T) {
	dir := setupMemoryCgroup(t, true)
	writeMemoryFixture(t, dir, true, "600", "1000", "file 200\nshmem 0\n")
	original := memoryVirtualMemory
	t.Cleanup(func() { memoryVirtualMemory = original })
	memoryVirtualMemory = func() (*mem.VirtualMemoryStat, error) {
		return &mem.VirtualMemoryStat{Total: 2000}, nil
	}
	a := &Agent{forceUseCgroup: true}
	a.refreshSystemDetails()
	assert.EqualValues(t, 1000, a.systemDetails.MemoryTotal)
}

// Captured from this container: active file cache made the working-set
// calculation report ~80 GiB despite only ~17 GiB of anonymous RSS.
func TestMemoryStatsExcludeActiveFileCache(t *testing.T) {
	dir := setupMemoryCgroup(t, false)
	const usage = uint64(135881601024)
	const cache = uint64(116823166976)
	const shmem = uint64(77180928)
	writeMemoryFixture(t, dir, false, strconv.FormatUint(usage, 10), "137438953472",
		"total_cache 116823166976\ntotal_shmem 77180928\n"+
			"total_inactive_file 49902608384\ntotal_active_file 67684671488\n")
	original := memoryVirtualMemory
	t.Cleanup(func() { memoryVirtualMemory = original })
	memoryVirtualMemory = func() (*mem.VirtualMemoryStat, error) {
		return &mem.VirtualMemoryStat{Total: 1580621720 * 1024}, nil
	}
	metrics, ok := containerMemoryMetrics(1580621720*1024, true)
	require.True(t, ok)
	assert.Equal(t, usage-(cache-shmem), metrics.Used)
	assert.Equal(t, cache-shmem, metrics.BuffCache)
	assert.Equal(t, usage, metrics.Used+metrics.BuffCache)
	var stats system.Stats
	a := &Agent{forceUseCgroup: true}
	a.updateMemoryStats(&stats)
	assert.Equal(t, float64(17.82), stats.MemUsed)
	assert.Equal(t, float64(108.73), stats.MemBuffCache)
	assert.Equal(t, float64(13.92), stats.MemPct)
}
