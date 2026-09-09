package agent

import (
	"errors"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/henrygd/beszel/agent/btrfs"
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

// btrfsFilesystems is the btrfs source; overridable in tests.
var btrfsFilesystems = btrfs.Filesystems

type poolKernelSample struct {
	nread  uint64
	nwrite uint64
	at     time.Time
}

// StoragePoolManager combines independent backend inventories. Metrics and
// dataset usage require the agent lock; GetDetail is safe for concurrent calls.
type StoragePoolManager struct {
	backends       []*poolBackend
	detailInterval time.Duration
}

// poolBackend owns one backend's collectors and caches. Collector functions
// are immutable after construction and may run concurrently for metrics/details.
type poolBackend struct {
	name           string
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
	detailFailed      bool
}

func newStoragePoolManager() *StoragePoolManager {
	return &StoragePoolManager{
		backends:       []*poolBackend{newZfsBackend(), newBtrfsBackend()},
		detailInterval: time.Hour,
	}
}

func newZfsBackend() *poolBackend {
	return &poolBackend{
		name:           "zfs",
		poolStatsFn:    optionalPoolSource(zfs.PoolStats),
		datasetsFn:     zfs.Datasets,
		kernelStatsFn:  optionalPoolSource(zfs.PoolKernelStats),
		poolStatusesFn: optionalPoolSource(zfs.PoolStatuses),
	}
}

func newBtrfsBackend() *poolBackend {
	return &poolBackend{
		name:           "btrfs",
		poolStatsFn:    btrfsSource(btrfsPoolStats),
		kernelStatsFn:  btrfsSource(btrfsKernelStats),
		poolStatusesFn: btrfsSource(btrfsPoolStatuses),
	}
}

// datasets is optional: only backends that expose datasets provide a collector.
func (b *poolBackend) datasets() ([]zfs.Dataset, error) {
	if b.datasetsFn == nil {
		return nil, nil
	}
	return b.datasetsFn()
}

// A missing utility/interface is a successfully observed absent backend.
func optionalPoolSource[T any](source func() ([]T, error)) func() ([]T, error) {
	return func() ([]T, error) {
		items, err := source()
		if errors.Is(err, zfs.ErrNoZfs) || errors.Is(err, exec.ErrNotFound) || errors.Is(err, errors.ErrUnsupported) {
			return nil, nil
		}
		return items, err
	}
}

func btrfsSource[T any](convert func(btrfs.Filesystem) T) func() ([]T, error) {
	return func() ([]T, error) {
		filesystems, err := optionalPoolSource(btrfsFilesystems)()
		if err != nil {
			return nil, err
		}
		items := make([]T, 0, len(filesystems))
		for _, fs := range filesystems {
			items = append(items, convert(fs))
		}
		return items, nil
	}
}

// Update refreshes systemStats.ZfsPools with the latest pool data. I/O
// throughput and health come from inexpensive kernel kstats on Linux. Pool
// capacity and dataset usage come from separately cached utility calls. The
// pool map is empty when both backends are absent.
func (m *StoragePoolManager) Update(systemStats *system.Stats) {
	// Rebuild the combined map so successful pool removals clear old samples.
	systemStats.ZfsPools = nil
	for _, backend := range m.backends {
		backend.updateBackendStats(systemStats)
	}
}

func (b *poolBackend) updateBackendStats(systemStats *system.Stats) {
	pools := b.poolStats()
	if len(pools) == 0 {
		b.kernelSamples = nil
		return
	}

	kernelStats, ioRates := b.kernelStats()

	if systemStats.ZfsPools == nil {
		systemStats.ZfsPools = make(map[string]*system.ZfsPool, len(pools))
	}
	for i := range pools {
		pool := &pools[i]
		// Full precision, matching the dataset values below; the frontend
		// formats any magnitude.
		stats := &system.ZfsPool{
			DisplayName: pool.DisplayName,
			Raw:         pool.Raw,
			Total:       float64(pool.Size) / (1024 * 1024 * 1024),
			Used:        float64(pool.Alloc) / (1024 * 1024 * 1024),
			Health:      pool.Health,
		}
		if kernel, exists := kernelStats[pool.Name]; exists && kernel.Health != "" {
			stats.Health = kernel.Health
		}
		if io, exists := ioRates[pool.Name]; exists {
			stats.ReadBytes = io.NRead
			stats.WriteBytes = io.NWrite
		}
		slog.Debug("Storage pool sample", "backend", b.name, "pool", pool.Name, "health", stats.Health, "used_gb", stats.Used, "read_bps", stats.ReadBytes, "write_bps", stats.WriteBytes)
		systemStats.ZfsPools[pool.Name] = stats
	}

}

// poolStats returns the cached pool inventory, calling its collector at most
// every poolStatsRefreshInterval. On failure the previous inventory is
// retained and the refresh is retried on the next cadence.
func (b *poolBackend) poolStats() []zfs.PoolStat {
	if b.lastPoolStats.IsZero() || time.Since(b.lastPoolStats) >= poolStatsRefreshInterval {
		pools, err := b.poolStatsFn()
		if err != nil {
			slog.Debug("Storage pool stats unavailable", "backend", b.name, "err", err)
		} else {
			b.poolData = pools
		}
		b.lastPoolStats = time.Now()
	}
	return b.poolData
}

// kernelStats reads cumulative pool counters and converts them to per-second
// rates. Counter decreases indicate a pool export/import and reset the
// baseline instead of producing an underflow spike.
func (b *poolBackend) kernelStats() (map[string]zfs.PoolKernelStat, map[string]zfs.PoolIoStats) {
	if b.kernelStatsFn == nil {
		return nil, nil
	}
	stats, err := b.kernelStatsFn()
	if err != nil {
		slog.Debug("Storage pool kernel stats unavailable", "backend", b.name, "err", err)
		return nil, nil
	}
	now := time.Now()
	byName := make(map[string]zfs.PoolKernelStat, len(stats))
	rates := make(map[string]zfs.PoolIoStats, len(stats))
	nextSamples := make(map[string]poolKernelSample, len(stats))
	for _, stat := range stats {
		byName[stat.Name] = stat
		if previous, ok := b.kernelSamples[stat.Name]; ok && now.After(previous.at) &&
			stat.NRead >= previous.nread && stat.NWrite >= previous.nwrite {
			seconds := now.Sub(previous.at).Seconds()
			rates[stat.Name] = zfs.PoolIoStats{
				NRead:  uint64(float64(stat.NRead-previous.nread) / seconds),
				NWrite: uint64(float64(stat.NWrite-previous.nwrite) / seconds),
			}
		}
		nextSamples[stat.Name] = poolKernelSample{nread: stat.NRead, nwrite: stat.NWrite, at: now}
	}
	b.kernelSamples = nextSamples
	return byName, rates
}

// refreshDatasetUsage re-runs `zfs list` when the refresh window has elapsed
// and rebuilds the mountpoint-keyed usage map.
func (b *poolBackend) refreshDatasetUsage() {
	if !b.lastUsageRefresh.IsZero() && time.Since(b.lastUsageRefresh) < datasetUsageRefreshInterval {
		return
	}
	datasets, err := b.datasets()
	if err != nil {
		slog.Debug("Storage pool dataset usage unavailable", "backend", b.name, "err", err)
	} else {
		usage := make(map[string]zfsDatasetUsage, len(datasets))
		for _, ds := range datasets {
			if ds.Mountpoint != "" && ds.Mountpoint != "-" {
				usage[ds.Mountpoint] = zfsDatasetUsage{used: ds.Used, avail: ds.Avail}
			}
		}
		b.datasetUsage = usage
	}
	b.lastUsageRefresh = time.Now()
}

// DatasetUsage returns ZFS dataset usage keyed by mountpoint, refreshed at
// most every datasetUsageRefreshInterval. On failure the previous map is
// retained and a debug log is emitted.
func (m *StoragePoolManager) DatasetUsage() map[string]zfsDatasetUsage {
	for _, backend := range m.backends {
		if backend.name == "zfs" {
			backend.refreshDatasetUsage()
			return backend.datasetUsage
		}
	}
	return nil
}

// GetDetail combines backend snapshots, identifying successful inventories so
// the hub can accept partial updates without deleting failed backend records.
func (m *StoragePoolManager) GetDetail(force bool) *zfsentity.ZfsData {
	data := &zfsentity.ZfsData{Complete: true}
	for _, backend := range m.backends {
		snapshot := backend.getBackendDetail(force, m.detailInterval)
		data.Pools = append(data.Pools, snapshot.Pools...)
		if snapshot.Complete {
			data.CompleteBackends = append(data.CompleteBackends, backend.name)
		} else {
			data.Complete = false
		}
	}
	return data
}

func (b *poolBackend) getBackendDetail(force bool, interval time.Duration) *zfsentity.ZfsData {
	b.detailMu.Lock()
	defer b.detailMu.Unlock()

	if force || b.detailFailed || b.detail == nil || time.Since(b.lastDetailRefresh) >= interval {
		if data, err := b.collectDetail(b.detail); err != nil {
			b.detailFailed = true
			slog.Debug("Storage pool detail collection failed", "backend", b.name, "err", err)
			if b.detail == nil {
				return &zfsentity.ZfsData{}
			}
			return &zfsentity.ZfsData{Pools: b.detail.Pools}
		} else {
			b.detailFailed = false
			b.detail = data
			b.lastDetailRefresh = time.Now()
		}
	}
	if b.detail == nil {
		return &zfsentity.ZfsData{}
	}
	return b.detail
}

// collectDetail builds a ZfsData payload from the current system state.
func (b *poolBackend) collectDetail(previous *zfsentity.ZfsData) (*zfsentity.ZfsData, error) {
	pools, err := b.poolStatsFn()
	if err != nil {
		return nil, err
	}
	if len(pools) == 0 {
		return &zfsentity.ZfsData{Pools: []*zfsentity.PoolDetail{}, Complete: true}, nil
	}

	statuses, statusErr := b.poolStatusesFn()
	if statusErr != nil {
		slog.Debug("Storage pool status unavailable", "backend", b.name, "err", statusErr)
	}
	datasets, datasetsErr := b.datasets()
	if datasetsErr != nil {
		slog.Debug("Storage pool datasets unavailable", "backend", b.name, "err", datasetsErr)
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
			DisplayName: p.DisplayName,
			Raw:         p.Raw,
			Name:        p.Name,
			Health:      p.Health,
			Size:        p.Size,
			Alloc:       p.Alloc,
			Free:        p.Free,
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
func (m *StoragePoolManager) ZfsMountpoints() map[string]bool {
	usage := m.DatasetUsage()
	mountpoints := make(map[string]bool, len(usage))
	for mountpoint := range usage {
		mountpoints[mountpoint] = true
	}
	return mountpoints
}

func btrfsPoolStats(fs btrfs.Filesystem) zfs.PoolStat {
	return zfs.PoolStat{MountID: fs.MountID, IODevice: fs.IODevice, Raw: fs.Raw, DisplayName: fs.Name, Name: "b:" + fs.UUID, Size: fs.Size, Alloc: fs.Alloc, Free: fs.Size - min(fs.Alloc, fs.Size), Health: fs.Health}
}

func btrfsKernelStats(fs btrfs.Filesystem) zfs.PoolKernelStat {
	return zfs.PoolKernelStat{Name: "b:" + fs.UUID, Health: fs.Health, NRead: fs.NRead, NWrite: fs.NWrite}
}

func btrfsPoolStatuses(fs btrfs.Filesystem) zfs.PoolStatus {
	status := zfs.PoolStatus{Name: "b:" + fs.UUID, State: fs.Health, Scrub: zfs.ScrubStatus{State: "NONE"}}
	for _, dev := range fs.Devices {
		status.Vdevs = append(status.Vdevs, zfs.VdevStatus{
			Name: dev.Name, State: dev.State,
			ReadErrs: dev.ReadErrs, WriteErrs: dev.WriteErrs, ChecksumErrs: dev.CorruptionErrs,
		})
	}
	return status
}

// markDuplicateCharts leaves pool telemetry and detail intact, but tells the
// hub which charts already have a filesystem equivalent. Only exact kernel
// filesystem and I/O-device matches qualify; labels are never used.
func (m *StoragePoolManager) markDuplicateCharts(stats *system.Stats, filesystems map[string]*system.FsStats, mountID func(string) string) {
	identities := make(map[string]string, len(filesystems))
	for device, fs := range filesystems {
		if fs.DiskTotal > 0 {
			identities[device] = mountID(fs.Mountpoint)
		}
	}
	for _, backend := range m.backends {
		for _, pool := range backend.poolData {
			sample := stats.ZfsPools[pool.Name]
			if sample == nil || pool.MountID == "" {
				continue
			}
			for device, identity := range identities {
				if identity != pool.MountID {
					continue
				}
				// Raw physical usage is not equivalent to a filesystem usage chart.
				sample.HideUsage = !pool.Raw
				if pool.IODevice != "" && pool.IODevice == device {
					sample.HideIO = true
				}
			}
		}
	}
}
