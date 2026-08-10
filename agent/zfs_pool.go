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
// capacity and health. The values change slowly, so a short TTL avoids an exec
// on every realtime poll.
const poolStatsRefreshInterval = 10 * time.Second

// watcherRetryInterval is the backoff between attempts to (re)start the
// streaming `zpool iostat` watcher after a failure.
const watcherRetryInterval = 30 * time.Second

// poolIoSource is the streaming I/O rate source (see zfs.PoolIoWatcher).
type poolIoSource interface {
	Latest() (map[string]zfs.PoolIoStats, bool)
}

// ZfsManager collects ZFS pool and dataset statistics. Collection functions
// are fields so unit tests can substitute them (same pattern as
// diskDiscovery.usageFn). It is safe for concurrent use by a single goroutine
// only; callers must hold the agent lock like updateDiskUsage does.
type ZfsManager struct {
	poolStatsFn func() ([]zfs.PoolStat, error) // capacity/health source
	datasetsFn  func() ([]zfs.Dataset, error)  // dataset inventory source
	watcherFn   func() (poolIoSource, error)   // streaming I/O rate source

	poolData      []zfs.PoolStat // cached pool inventory (TTL below)
	lastPoolStats time.Time
	ioWatcher     poolIoSource // streaming I/O rate source
	watcherSince  time.Time    // last watcher (re)start attempt

	datasetUsage     map[string]zfsDatasetUsage // mountpoint -> usage
	datasetStats     map[string]zfsDatasetUsage // dataset name -> usage
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
		watcherFn:      func() (poolIoSource, error) { return zfs.NewPoolIoWatcher() },
		detailInterval: time.Hour,
	}
}

// Update refreshes systemStats.ZfsPools with the latest pool data. I/O
// throughput comes from the streaming `zpool iostat` watcher (per-second
// rates, works on every OpenZFS version), so the request path never blocks on
// ZFS collection (the realtime worker polls every second). It is a no-op when
// ZFS is absent.
func (zm *ZfsManager) Update(systemStats *system.Stats) {
	pools := zm.poolStats()
	if len(pools) == 0 {
		return
	}

	ioCounters := zm.watcherIo()

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
		if io, exists := ioCounters[pool.Name]; exists {
			stats.ReadBytes = io.NRead
			stats.WriteBytes = io.NWrite
		}
		slog.Debug("ZFS pool sample", "pool", pool.Name, "health", stats.Health, "used_gb", stats.Used, "read_bps", stats.ReadBytes, "write_bps", stats.WriteBytes)
		systemStats.ZfsPools[pool.Name] = stats
	}

	// Per-dataset usage (refreshed at most every datasetUsageRefreshInterval).
	// Values are not rounded to 2 decimal GB so small datasets (EFI disks,
	// small volumes) stay visible; the frontend formats any magnitude.
	datasetStats := zm.DatasetStats()
	if len(datasetStats) > 0 {
		if systemStats.ZfsDatasets == nil {
			systemStats.ZfsDatasets = make(map[string]*system.ZfsDataset, len(datasetStats))
		}
		for name, ds := range datasetStats {
			systemStats.ZfsDatasets[name] = &system.ZfsDataset{
				Used: float64(ds.used) / (1024 * 1024 * 1024),
			}
		}
	}
}

// poolStats returns the cached pool inventory, re-running `zpool list` at most
// every poolStatsRefreshInterval. On failure the previous inventory is
// retained and the refresh is retried on the next cadence.
func (zm *ZfsManager) poolStats() []zfs.PoolStat {
	if zm.poolData == nil || time.Since(zm.lastPoolStats) >= poolStatsRefreshInterval {
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

// watcherIo returns the latest rates from the streaming `zpool iostat`
// watcher, starting it on first use with a retry backoff.
func (zm *ZfsManager) watcherIo() map[string]zfs.PoolIoStats {
	if zm.ioWatcher == nil {
		if time.Since(zm.watcherSince) < watcherRetryInterval || zm.watcherFn == nil {
			return nil
		}
		zm.watcherSince = time.Now()
		watcher, err := zm.watcherFn()
		if err != nil {
			slog.Debug("ZFS pool I/O watcher unavailable", "err", err)
			return nil
		}
		zm.ioWatcher = watcher
	}
	latest, ok := zm.ioWatcher.Latest()
	if !ok {
		return nil
	}
	return latest
}

// refreshDatasetUsage re-runs `zfs list` when the refresh window has elapsed
// and rebuilds both the mountpoint-keyed and name-keyed usage maps.
func (zm *ZfsManager) refreshDatasetUsage() {
	if !zm.lastUsageRefresh.IsZero() && time.Since(zm.lastUsageRefresh) < datasetUsageRefreshInterval {
		return
	}
	datasets, err := zm.datasetsFn()
	if err != nil {
		slog.Debug("ZFS dataset usage unavailable", "err", err)
	} else {
		usage := make(map[string]zfsDatasetUsage, len(datasets))
		stats := make(map[string]zfsDatasetUsage, len(datasets))
		for _, ds := range datasets {
			if ds.Mountpoint != "" && ds.Mountpoint != "-" {
				usage[ds.Mountpoint] = zfsDatasetUsage{used: ds.Used, avail: ds.Avail}
			}
			stats[ds.Name] = zfsDatasetUsage{used: ds.Used, avail: ds.Avail}
		}
		zm.datasetUsage = usage
		zm.datasetStats = stats
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

// DatasetStats returns ZFS dataset usage keyed by dataset name (for per-dataset
// charts), refreshed on the same cadence as DatasetUsage.
func (zm *ZfsManager) DatasetStats() map[string]zfsDatasetUsage {
	zm.refreshDatasetUsage()
	return zm.datasetStats
}

// GetDetail returns cached ZFS detail data (pool health, scrub, vdevs,
// datasets), refreshing it when stale. On failure the previous snapshot is
// retained; an empty payload is returned when nothing has been collected.
func (zm *ZfsManager) GetDetail() *zfsentity.ZfsData {
	zm.detailMu.Lock()
	defer zm.detailMu.Unlock()

	if zm.detail == nil || time.Since(zm.lastDetailRefresh) >= zm.detailInterval {
		if data, err := zm.collectDetail(); err != nil {
			slog.Debug("ZFS detail collection failed", "err", err)
		} else {
			zm.detail = data
		}
		zm.lastDetailRefresh = time.Now()
	}
	if zm.detail == nil {
		return &zfsentity.ZfsData{}
	}
	return zm.detail
}

// collectDetail builds a ZfsData payload from the current system state.
func (zm *ZfsManager) collectDetail() (*zfsentity.ZfsData, error) {
	pools, err := zm.poolStatsFn()
	if err != nil {
		return nil, err
	}
	if len(pools) == 0 {
		return nil, zfs.ErrNoZfs
	}

	statuses, err := zfs.PoolStatuses()
	if err != nil {
		slog.Debug("ZFS pool status unavailable", "err", err)
	}
	datasets, err := zm.datasetsFn()
	if err != nil {
		slog.Debug("ZFS datasets unavailable", "err", err)
	}

	statusByPool := make(map[string]zfs.PoolStatus, len(statuses))
	for _, st := range statuses {
		statusByPool[st.Name] = st
	}

	data := &zfsentity.ZfsData{Pools: make([]*zfsentity.PoolDetail, 0, len(pools))}
	for i := range pools {
		p := &pools[i]
		detail := &zfsentity.PoolDetail{
			Name:   p.Name,
			Health: p.Health,
			Size:   p.Size,
			Alloc:  p.Alloc,
			Free:   p.Free,
		}
		if st, ok := statusByPool[p.Name]; ok {
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
		}
		for _, ds := range datasets {
			if poolOfDataset(ds.Name) == p.Name {
				detail.Datasets = append(detail.Datasets, &zfsentity.Dataset{
					Name:       ds.Name,
					Used:       ds.Used,
					Avail:      ds.Avail,
					Mountpoint: ds.Mountpoint,
				})
			}
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
