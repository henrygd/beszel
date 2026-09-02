package agent

import (
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/henrygd/beszel/agent/zfs"
	"github.com/henrygd/beszel/internal/entities/system"
	zfsentity "github.com/henrygd/beszel/internal/entities/zfs"
)

// zfsDatasetUsage holds usage values for a ZFS dataset mountpoint.
type zfsDatasetUsage struct {
	used  uint64
	avail uint64
}

// datasetUsageRefreshInterval controls how often `zfs list` is re-run for the
// mountpoint usage map. Dataset inventory changes rarely.
const datasetUsageRefreshInterval = 5 * time.Minute

// poolStatsRefreshInterval controls how often `zpool list` is re-run for pool
// capacity. Health and I/O are read from procfs on Linux, so the utility only
// needs to refresh slow-moving space accounting.
const poolStatsRefreshInterval = time.Minute

type poolKernelSample struct {
	nread  uint64
	nwrite uint64
	at     time.Time
}

// ZfsManager collects ZFS pool and dataset statistics. Collection functions
// are fields so unit tests can substitute them (same pattern as
// diskDiscovery.usageFn). It is safe for concurrent use by a single goroutine
// only; callers must hold the agent lock like updateDiskUsage does.
type ZfsManager struct {
	poolStatsFn    func() ([]zfs.PoolStat, error)       // capacity/health source
	datasetsFn     func() ([]zfs.Dataset, error)        // dataset inventory source
	kernelStatsFn  func() ([]zfs.PoolKernelStat, error) // procfs pool state/I/O source
	poolStatusesFn func() ([]zfs.PoolStatus, error)     // scrub/vdev detail source

	poolData      []zfs.PoolStat // cached pool inventory (TTL below)
	lastPoolStats time.Time
	kernelSamples map[string]poolKernelSample

	datasetUsage     map[string]zfsDatasetUsage // mountpoint -> usage
	lastUsageRefresh time.Time

	// Detail data (pools, vdevs, scrub, datasets) is cached and refreshed on
	// an interval. Accessed from handler goroutines, so it is mutex-protected.
	detailMu          sync.Mutex
	detail            *zfsentity.ZfsData
	lastDetailRefresh time.Time
	detailInterval    time.Duration
}

// newZfsManager creates a ZfsManager wired to the system's ZFS utilities.
func newZfsManager() *ZfsManager {
	return &ZfsManager{
		poolStatsFn:    zfs.PoolStats,
		datasetsFn:     zfs.Datasets,
		kernelStatsFn:  zfs.PoolKernelStats,
		poolStatusesFn: zfs.PoolStatuses,
		detailInterval: time.Hour,
	}
}

// Update refreshes systemStats.ZfsPools with the latest pool data. I/O
// throughput and health come from inexpensive kernel kstats on Linux. Pool
// capacity and dataset usage come from separately cached utility calls. It is
// a no-op when ZFS is absent.
func (zm *ZfsManager) Update(systemStats *system.Stats) {
	pools := zm.poolStats()
	if len(pools) == 0 {
		return
	}

	kernelStats, ioRates := zm.kernelStats()

	if systemStats.ZfsPools == nil {
		systemStats.ZfsPools = make(map[string]*system.ZfsPool, len(pools))
	}
	for i := range pools {
		pool := &pools[i]
		// Full precision, matching the dataset values below; the frontend
		// formats any magnitude.
		stats := &system.ZfsPool{
			Total:  float64(pool.Size) / (1024 * 1024 * 1024),
			Used:   float64(pool.Alloc) / (1024 * 1024 * 1024),
			Health: pool.Health,
		}
		if kernel, exists := kernelStats[pool.Name]; exists && kernel.Health != "" {
			stats.Health = kernel.Health
		}
		if io, exists := ioRates[pool.Name]; exists {
			stats.ReadBytes = io.NRead
			stats.WriteBytes = io.NWrite
		}
		slog.Debug("ZFS pool sample", "pool", pool.Name, "health", stats.Health, "used_gb", stats.Used, "read_bps", stats.ReadBytes, "write_bps", stats.WriteBytes)
		systemStats.ZfsPools[pool.Name] = stats
	}

}

// poolStats returns the cached pool inventory, re-running `zpool list` at most
// every poolStatsRefreshInterval. On failure the previous inventory is
// retained and the refresh is retried on the next cadence.
func (zm *ZfsManager) poolStats() []zfs.PoolStat {
	if zm.lastPoolStats.IsZero() || time.Since(zm.lastPoolStats) >= poolStatsRefreshInterval {
		pools, err := zm.poolStatsFn()
		if err != nil {
			slog.Debug("ZFS pool stats unavailable", "err", err)
		} else {
			zm.poolData = pools
		}
		zm.lastPoolStats = time.Now()
	}
	return zm.poolData
}

// kernelStats reads cumulative pool counters and converts them to per-second
// rates. Counter decreases indicate a pool export/import and reset the
// baseline instead of producing an underflow spike.
func (zm *ZfsManager) kernelStats() (map[string]zfs.PoolKernelStat, map[string]zfs.PoolIoStats) {
	if zm.kernelStatsFn == nil {
		return nil, nil
	}
	stats, err := zm.kernelStatsFn()
	if err != nil {
		slog.Debug("ZFS kernel stats unavailable", "err", err)
		return nil, nil
	}
	now := time.Now()
	byName := make(map[string]zfs.PoolKernelStat, len(stats))
	rates := make(map[string]zfs.PoolIoStats, len(stats))
	nextSamples := make(map[string]poolKernelSample, len(stats))
	for _, stat := range stats {
		byName[stat.Name] = stat
		if previous, ok := zm.kernelSamples[stat.Name]; ok && now.After(previous.at) &&
			stat.NRead >= previous.nread && stat.NWrite >= previous.nwrite {
			seconds := now.Sub(previous.at).Seconds()
			rates[stat.Name] = zfs.PoolIoStats{
				NRead:  uint64(float64(stat.NRead-previous.nread) / seconds),
				NWrite: uint64(float64(stat.NWrite-previous.nwrite) / seconds),
			}
		}
		nextSamples[stat.Name] = poolKernelSample{nread: stat.NRead, nwrite: stat.NWrite, at: now}
	}
	zm.kernelSamples = nextSamples
	return byName, rates
}

// refreshDatasetUsage re-runs `zfs list` when the refresh window has elapsed
// and rebuilds the mountpoint-keyed usage map.
func (zm *ZfsManager) refreshDatasetUsage() {
	if !zm.lastUsageRefresh.IsZero() && time.Since(zm.lastUsageRefresh) < datasetUsageRefreshInterval {
		return
	}
	datasets, err := zm.datasetsFn()
	if err != nil {
		slog.Debug("ZFS dataset usage unavailable", "err", err)
	} else {
		usage := make(map[string]zfsDatasetUsage, len(datasets))
		for _, ds := range datasets {
			if ds.Mountpoint != "" && ds.Mountpoint != "-" {
				usage[ds.Mountpoint] = zfsDatasetUsage{used: ds.Used, avail: ds.Avail}
			}
		}
		zm.datasetUsage = usage
	}
	zm.lastUsageRefresh = time.Now()
}

// DatasetUsage returns ZFS dataset usage keyed by mountpoint, refreshed at
// most every datasetUsageRefreshInterval. On failure the previous map is
// retained and a debug log is emitted.
func (zm *ZfsManager) DatasetUsage() map[string]zfsDatasetUsage {
	zm.refreshDatasetUsage()
	return zm.datasetUsage
}

// GetDetail returns ZFS detail data (pool health, scrub, vdevs, datasets).
// Scheduled requests use the cached snapshot until stale; manual requests can
// force collection. On failure the previous snapshot is retained.
func (zm *ZfsManager) GetDetail(force bool) *zfsentity.ZfsData {
	zm.detailMu.Lock()
	defer zm.detailMu.Unlock()

	if force || zm.detail == nil || time.Since(zm.lastDetailRefresh) >= zm.detailInterval {
		if data, err := zm.collectDetail(zm.detail); err != nil {
			slog.Debug("ZFS detail collection failed", "err", err)
			if zm.detail == nil {
				return &zfsentity.ZfsData{}
			}
			return &zfsentity.ZfsData{Pools: zm.detail.Pools}
		} else {
			zm.detail = data
			zm.lastDetailRefresh = time.Now()
		}
	}
	if zm.detail == nil {
		return &zfsentity.ZfsData{}
	}
	return zm.detail
}

// collectDetail builds a ZfsData payload from the current system state.
func (zm *ZfsManager) collectDetail(previous *zfsentity.ZfsData) (*zfsentity.ZfsData, error) {
	pools, err := zm.poolStatsFn()
	if err != nil {
		return nil, err
	}
	if len(pools) == 0 {
		return &zfsentity.ZfsData{Pools: []*zfsentity.PoolDetail{}, Complete: true}, nil
	}

	statuses, statusErr := zm.poolStatusesFn()
	if statusErr != nil {
		slog.Debug("ZFS pool status unavailable", "err", statusErr)
	}
	datasets, datasetsErr := zm.datasetsFn()
	if datasetsErr != nil {
		slog.Debug("ZFS datasets unavailable", "err", datasetsErr)
	}

	statusByPool := make(map[string]zfs.PoolStatus, len(statuses))
	for _, st := range statuses {
		statusByPool[st.Name] = st
	}

	previousByPool := make(map[string]*zfsentity.PoolDetail)
	if previous != nil {
		for _, pool := range previous.Pools {
			if pool != nil {
				previousByPool[pool.Name] = pool
			}
		}
	}

	data := &zfsentity.ZfsData{Pools: make([]*zfsentity.PoolDetail, 0, len(pools)), Complete: true}
	for i := range pools {
		p := &pools[i]
		detail := &zfsentity.PoolDetail{
			Name:   p.Name,
			Health: p.Health,
			Size:   p.Size,
			Alloc:  p.Alloc,
			Free:   p.Free,
		}
		if st, ok := statusByPool[p.Name]; statusErr == nil && ok {
			if st.Scrub.State != "" && st.Scrub.State != "NONE" {
				detail.Scrub = &zfsentity.Scrub{
					State:    st.Scrub.State,
					Progress: st.Scrub.Progress,
					Errors:   st.Scrub.Errors,
				}
			}
			for _, v := range st.Vdevs {
				detail.Vdevs = append(detail.Vdevs, &zfsentity.Vdev{
					Name:         v.Name,
					State:        v.State,
					ReadErrs:     v.ReadErrs,
					WriteErrs:    v.WriteErrs,
					ChecksumErrs: v.ChecksumErrs,
				})
			}
		} else {
			if cached := previousByPool[p.Name]; cached != nil {
				detail.Scrub = cached.Scrub
				detail.Vdevs = cached.Vdevs
			}
		}
		if datasetsErr == nil {
			foundDataset := false
			for _, ds := range datasets {
				if poolOfDataset(ds.Name) == p.Name {
					foundDataset = true
					detail.Datasets = append(detail.Datasets, &zfsentity.Dataset{
						Name:       ds.Name,
						Used:       ds.Used,
						Avail:      ds.Avail,
						Mountpoint: ds.Mountpoint,
					})
				}
			}
			if !foundDataset {
				if cached := previousByPool[p.Name]; cached != nil {
					detail.Datasets = cached.Datasets
				}
			}
		} else if cached := previousByPool[p.Name]; cached != nil {
			detail.Datasets = cached.Datasets
		}
		data.Pools = append(data.Pools, detail)
	}
	return data, nil
}

// poolOfDataset returns the pool name for a dataset name (everything before
// the first '/'). Datasets without a separator belong to a pool of the same
// name.
func poolOfDataset(name string) string {
	if idx := strings.IndexByte(name, '/'); idx >= 0 {
		return name[:idx]
	}
	return name
}

// ZfsMountpoints returns the set of mountpoints backed by ZFS datasets.
func (zm *ZfsManager) ZfsMountpoints() map[string]bool {
	usage := zm.DatasetUsage()
	mountpoints := make(map[string]bool, len(usage))
	for mountpoint := range usage {
		mountpoints[mountpoint] = true
	}
	return mountpoints
}
