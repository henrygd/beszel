package agent

import (
	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/agent/zfs"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/shirou/gopsutil/v4/mem"
)

// memoryMetrics holds byte counters. For cgroups, Used excludes file cache,
// while retaining shared memory/tmpfs. BuffCache is the subtracted file cache.
type memoryMetrics struct {
	Total, Used, BuffCache uint64
}

var memoryVirtualMemory = mem.VirtualMemory

func (a *Agent) updateMemoryStats(stats *system.Stats) {
	v, err := memoryVirtualMemory()
	var hostTotal uint64
	if err == nil {
		hostTotal = v.Total
		// Swap retains its existing /proc/meminfo source in both memory paths.
		stats.SwapBytes = [2]uint64{v.SwapTotal, saturatingSub(v.SwapTotal, v.SwapFree)}
		// TODO: stop populating deprecated GB fields once hubs < 0.21.1 are no longer supported
		stats.Swap, stats.SwapUsed, stats.SwapPct = calculateSwapUsage(stats.SwapBytes[0], stats.SwapBytes[1])
	}

	var arcSize uint64
	metrics, fromCgroup := containerMemoryMetrics(hostTotal, a.forceUseCgroup)
	if !fromCgroup {
		if err != nil {
			return
		}
		used, cacheBuff, _ := calculateHostMemoryUsage(v, a.memCalc == "htop")
		metrics = memoryMetrics{Total: v.Total, Used: used, BuffCache: cacheBuff}
		// Host ARC must not be subtracted from container-scoped accounting.
		if a.zfs {
			if size, _ := zfs.ARCSize(); size > 0 && size < metrics.Used {
				arcSize = size
				metrics.Used -= arcSize
			}
		}
	}

	stats.MemBytes = [4]uint64{metrics.Total, metrics.Used, metrics.BuffCache, arcSize}
	// TODO: stop populating deprecated GB fields once hubs < 0.21.1 are no longer supported
	stats.Mem = utils.BytesToGigabytes(metrics.Total)
	stats.MemUsed = utils.BytesToGigabytes(metrics.Used)
	stats.MemBuffCache = utils.BytesToGigabytes(metrics.BuffCache)
	stats.MemZfsArc = utils.BytesToGigabytes(arcSize)
	if metrics.Total > 0 {
		stats.MemPct = utils.TwoDecimals(float64(metrics.Used) / float64(metrics.Total) * 100)
	}
	if a.systemDetails.MemoryTotal != metrics.Total {
		a.updateSystemDetails(func(details *system.Details) {
			details.MemoryTotal = metrics.Total
		})
	}
}
