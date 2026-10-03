package agent

import (
	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/agent/zfs"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/shirou/gopsutil/v4/mem"
)

// memoryMetrics holds byte counters. For cgroups, Used is the working set and
// BuffCache is the inactive file memory subtracted from the raw charge.
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
		stats.Swap = utils.BytesToGigabytes(v.SwapTotal)
		stats.SwapUsed = utils.BytesToGigabytes(saturatingSub(v.SwapTotal, v.SwapFree))
	}

	metrics, fromCgroup := containerMemoryMetrics(hostTotal, a.forceUseCgroup)
	if !fromCgroup {
		if err != nil {
			return
		}
		used, cacheBuff, _ := calculateHostMemoryUsage(v, a.memCalc == "htop")
		metrics = memoryMetrics{Total: v.Total, Used: used, BuffCache: cacheBuff}
		// Host ARC must not be subtracted from container-scoped accounting.
		if a.zfs {
			if arcSize, _ := zfs.ARCSize(); arcSize > 0 && arcSize < metrics.Used {
				metrics.Used -= arcSize
				stats.MemZfsArc = utils.BytesToGigabytes(arcSize)
			}
		}
	}

	stats.Mem = utils.BytesToGigabytes(metrics.Total)
	stats.MemUsed = utils.BytesToGigabytes(metrics.Used)
	stats.MemBuffCache = utils.BytesToGigabytes(metrics.BuffCache)
	if metrics.Total > 0 {
		stats.MemPct = utils.TwoDecimals(float64(metrics.Used) / float64(metrics.Total) * 100)
	}
	if a.systemDetails.MemoryTotal != metrics.Total {
		a.updateSystemDetails(func(details *system.Details) {
			details.MemoryTotal = metrics.Total
		})
	}
}
