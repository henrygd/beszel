//go:build testing

package agent

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/henrygd/beszel/agent/btrfs"
	"github.com/henrygd/beszel/agent/zfs"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOptionalPoolSource(t *testing.T) {
	failure := errors.New("timeout")
	for _, err := range []error{nil, zfs.ErrNoZfs, fmt.Errorf("zpool: %w", exec.ErrNotFound), errors.ErrUnsupported, failure} {
		_, got := optionalPoolSource(func() ([]zfs.PoolStat, error) { return nil, err })()
		if err == failure {
			assert.ErrorIs(t, got, failure)
		} else {
			assert.NoError(t, got)
		}
	}
}

type poolTestBackend struct {
	name  string
	err   error
	alloc uint64
	read  uint64
	empty bool
}

func (state *poolTestBackend) backend() *poolBackend {
	name := "zfs"
	if strings.HasPrefix(state.name, "b:") {
		name = "btrfs"
	}
	return &poolBackend{
		name: name,
		poolStatsFn: func() ([]zfs.PoolStat, error) {
			if state.empty {
				return nil, state.err
			}
			return []zfs.PoolStat{{Name: state.name, Size: 100, Alloc: state.alloc}}, state.err
		},
		kernelStatsFn: func() ([]zfs.PoolKernelStat, error) {
			return []zfs.PoolKernelStat{{Name: state.name, NRead: state.read}}, state.err
		},
		poolStatusesFn: func() ([]zfs.PoolStatus, error) { return nil, nil },
		datasetsFn:     func() ([]zfs.Dataset, error) { return nil, nil },
	}
}

func TestIndependentPoolBackendCaches(t *testing.T) {
	for _, failed := range []int{0, 1} {
		t.Run([]string{"zfs", "btrfs"}[failed], func(t *testing.T) {
			states := []*poolTestBackend{{name: "tank", alloc: 10}, {name: "b:uuid", alloc: 10}}
			managers := []*poolBackend{states[0].backend(), states[1].backend()}
			zm := &StoragePoolManager{backends: managers, detailInterval: time.Hour}
			var stats system.Stats
			zm.Update(&stats)
			require.Len(t, stats.ZfsPools, 2)
			require.True(t, zm.GetDetail(true).Complete)
			baseline := poolKernelSample{at: time.Now().Add(-time.Second)}
			for i, m := range managers {
				m.lastPoolStats = time.Time{}
				m.kernelSamples[states[i].name] = baseline
				states[i].alloc = 20
				states[i].read = 100
			}
			states[failed].err = errors.New("collection failed")
			zm.Update(&stats)
			healthy := 1 - failed
			assert.Equal(t, uint64(10), managers[failed].poolData[0].Alloc)
			assert.Equal(t, uint64(20), managers[healthy].poolData[0].Alloc)
			assert.Equal(t, baseline, managers[failed].kernelSamples[states[failed].name])
			assert.Zero(t, stats.ZfsPools[states[failed].name].ReadBytes)
			assert.Positive(t, stats.ZfsPools[states[healthy].name].ReadBytes)
			partial := zm.GetDetail(true)
			assert.False(t, partial.Complete)
			assert.False(t, partial.CanRefreshPool(states[failed].name))
			assert.True(t, partial.CanRefreshPool(states[healthy].name))
			assert.Equal(t, uint64(10), partial.Pools[failed].Alloc)
			assert.Equal(t, uint64(20), partial.Pools[healthy].Alloc)
			assert.False(t, zm.GetDetail(false).Complete, "a failed forced refresh must not become complete from cache")

			// Successful empty inventory removes only the healthy backend's pool.
			states[healthy].empty = true
			managers[healthy].lastPoolStats = time.Time{}
			zm.Update(&stats)
			require.Len(t, stats.ZfsPools, 1)
			assert.Contains(t, stats.ZfsPools, states[failed].name)
			partial = zm.GetDetail(true)
			require.Len(t, partial.Pools, 1)
			assert.True(t, partial.CanRefreshPool(states[healthy].name))

			// Recovery uses the retained I/O baseline, then normal removal works.
			states[failed].err = nil
			managers[failed].lastPoolStats = time.Time{}
			zm.Update(&stats)
			assert.Positive(t, stats.ZfsPools[states[failed].name].ReadBytes)
			assert.True(t, zm.GetDetail(true).Complete)
			states[failed].empty = true
			managers[failed].lastPoolStats = time.Time{}
			zm.Update(&stats)
			assert.Empty(t, stats.ZfsPools)
			assert.Empty(t, zm.GetDetail(true).Pools)
		})
	}
}

func TestIndependentBackendsWithoutCache(t *testing.T) {
	z := &poolTestBackend{name: "tank", err: errors.New("ZFS failure")}
	b := &poolTestBackend{name: "b:uuid", alloc: 20}
	zm := &StoragePoolManager{backends: []*poolBackend{z.backend(), b.backend()}, detailInterval: time.Hour}
	var stats system.Stats
	zm.Update(&stats)
	require.Len(t, stats.ZfsPools, 1)
	assert.Contains(t, stats.ZfsPools, "b:uuid")
	detail := zm.GetDetail(true)
	require.Len(t, detail.Pools, 1)
	assert.False(t, detail.Complete)
	assert.Equal(t, []string{"btrfs"}, detail.CompleteBackends)
}

func TestConcurrentBackendDetailsAndMetrics(t *testing.T) {
	zm := &StoragePoolManager{backends: []*poolBackend{(&poolTestBackend{name: "tank"}).backend(), (&poolTestBackend{name: "b:uuid"}).backend()}, detailInterval: time.Hour}
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(metrics bool) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				if metrics {
					zm.Update(&system.Stats{})
				} else {
					zm.GetDetail(true)
				}
			}
		}(i == 0)
	}
	wg.Wait()
}

func TestStoragePoolBackendOrder(t *testing.T) {
	z := (&poolTestBackend{name: "tank"}).backend()
	b := (&poolTestBackend{name: "b:uuid"}).backend()
	b.datasetsFn = nil // Btrfs does not expose datasets.
	z.datasetsFn = func() ([]zfs.Dataset, error) {
		return []zfs.Dataset{{Name: "tank/data", Mountpoint: "/tank", Used: 10}}, nil
	}
	m := &StoragePoolManager{backends: []*poolBackend{b, z}, detailInterval: time.Hour}
	var stats system.Stats
	m.Update(&stats)
	require.Len(t, stats.ZfsPools, 2)
	detail := m.GetDetail(true)
	require.True(t, detail.Complete)
	assert.Equal(t, []string{"btrfs", "zfs"}, detail.CompleteBackends)
	assert.Empty(t, detail.Pools[0].Datasets)
	assert.Len(t, detail.Pools[1].Datasets, 1)
	assert.Equal(t, uint64(10), m.DatasetUsage()["/tank"].used)

	b.poolData[0].MountID = "uuid"
	b.poolData[0].IODevice = "sda"
	calls := 0
	m.markDuplicateCharts(&stats, map[string]*system.FsStats{
		"sda": {Mountpoint: "/", DiskTotal: 100},
	}, func(string) string { calls++; return "uuid" })
	assert.Equal(t, 1, calls, "resolve each filesystem once across all backends")
	assert.True(t, stats.ZfsPools["b:uuid"].HideUsage)
	assert.True(t, stats.ZfsPools["b:uuid"].HideIO)
}

func TestUpdatePopulatesZfsPools(t *testing.T) {
	zm := &StoragePoolManager{detailInterval: time.Hour, backends: []*poolBackend{{name: "zfs"}}}
	zm.backends[0].poolStatsFn = func() ([]zfs.PoolStat, error) {
		return []zfs.PoolStat{{Name: "tank", Size: 23999000000000, Alloc: 12000000000000, Free: 11999000000000, Health: "DEGRADED"}}, nil
	}
	zm.backends[0].datasetsFn = func() ([]zfs.Dataset, error) {
		return []zfs.Dataset{
			{Name: "tank/apps", Used: 5000000000000, Avail: 11999000000000, Mountpoint: "/tank/apps"},
			{Name: "tank/backup", Used: 6000000000000, Avail: 11999000000000, Mountpoint: "/tank/backup"},
			// Small zvol (Proxmox VM EFI disk): must not round to zero.
			{Name: "rpool/vm-100-disk-2", Used: 4194304, Avail: 0, Mountpoint: "-"},
		}, nil
	}
	var kernelCalls int
	zm.backends[0].kernelStatsFn = func() ([]zfs.PoolKernelStat, error) {
		kernelCalls++
		return []zfs.PoolKernelStat{{
			Name: "tank", Health: "ONLINE",
			NRead: uint64(kernelCalls-1) * 1250, NWrite: uint64(kernelCalls-1) * 5120,
		}}, nil
	}

	var stats system.Stats
	// The first kernel sample establishes the cumulative-counter baseline.
	zm.Update(&stats)
	zm.backends[0].kernelSamples["tank"] = poolKernelSample{at: time.Now().Add(-time.Second)}
	zm.Update(&stats)
	require.NotNil(t, stats.ZfsPools)
	require.Contains(t, stats.ZfsPools, "tank")
	assert.InDelta(t, 22350.8105, stats.ZfsPools["tank"].Total, 0.0001) // Size in GiB
	assert.InDelta(t, 11175.8709, stats.ZfsPools["tank"].Used, 0.0001)  // Alloc in GiB
	assert.Equal(t, "ONLINE", stats.ZfsPools["tank"].Health)
	assert.InDelta(t, 1250, stats.ZfsPools["tank"].ReadBytes, 5)
	assert.InDelta(t, 5120, stats.ZfsPools["tank"].WriteBytes, 5)

}

// TestUpdateKernelStatsMissing verifies pools without a kernel sample report zero
// I/O instead of erroring.
func TestUpdateKernelStatsMissing(t *testing.T) {
	zm := &StoragePoolManager{detailInterval: time.Hour, backends: []*poolBackend{{name: "zfs"}}}
	zm.backends[0].poolStatsFn = func() ([]zfs.PoolStat, error) {
		return []zfs.PoolStat{{Name: "tank", Size: 1, Alloc: 1, Health: "ONLINE"}}, nil
	}
	zm.backends[0].datasetsFn = func() ([]zfs.Dataset, error) { return nil, nil }
	zm.backends[0].kernelStatsFn = func() ([]zfs.PoolKernelStat, error) {
		return nil, zfs.ErrNoZfs
	}

	var stats system.Stats
	zm.Update(&stats)
	require.NotNil(t, stats.ZfsPools)
	assert.Equal(t, uint64(0), stats.ZfsPools["tank"].ReadBytes)
	assert.Equal(t, uint64(0), stats.ZfsPools["tank"].WriteBytes)
}

func TestUpdateKernelCounterReset(t *testing.T) {
	zm := &StoragePoolManager{detailInterval: time.Hour, backends: []*poolBackend{{name: "zfs"}}}
	zm.backends[0].poolStatsFn = func() ([]zfs.PoolStat, error) {
		return []zfs.PoolStat{{Name: "tank", Health: "ONLINE"}}, nil
	}
	zm.backends[0].datasetsFn = func() ([]zfs.Dataset, error) { return nil, nil }
	zm.backends[0].kernelSamples = map[string]poolKernelSample{
		"tank": {nread: 100, nwrite: 200, at: time.Now().Add(-time.Second)},
	}
	zm.backends[0].kernelStatsFn = func() ([]zfs.PoolKernelStat, error) {
		return []zfs.PoolKernelStat{{Name: "tank", Health: "ONLINE", NRead: 10, NWrite: 20}}, nil
	}

	var stats system.Stats
	zm.Update(&stats)
	assert.Equal(t, uint64(0), stats.ZfsPools["tank"].ReadBytes)
	assert.Equal(t, uint64(0), stats.ZfsPools["tank"].WriteBytes)
}

func TestUpdateNoZfs(t *testing.T) {
	zm := &StoragePoolManager{detailInterval: time.Hour, backends: []*poolBackend{{name: "zfs"}}}
	calls := 0
	zm.backends[0].poolStatsFn = func() ([]zfs.PoolStat, error) {
		calls++
		return nil, zfs.ErrNoZfs
	}

	var stats system.Stats
	zm.Update(&stats)
	zm.Update(&stats)
	assert.Nil(t, stats.ZfsPools)
	assert.Equal(t, 1, calls, "failed pool discovery should be cached until the next refresh interval")
}

func TestUpdateEmptyPools(t *testing.T) {
	zm := &StoragePoolManager{detailInterval: time.Hour, backends: []*poolBackend{{name: "zfs"}}}
	calls := 0
	zm.backends[0].poolStatsFn = func() ([]zfs.PoolStat, error) {
		calls++
		return nil, nil
	}

	var stats system.Stats
	zm.Update(&stats)
	zm.Update(&stats)
	assert.Nil(t, stats.ZfsPools)
	assert.Equal(t, 1, calls, "an empty pool inventory should be cached until the next refresh interval")
}

func TestDatasetUsage(t *testing.T) {
	zm := &StoragePoolManager{detailInterval: time.Hour, backends: []*poolBackend{{name: "zfs"}}}
	calls := 0
	zm.backends[0].datasetsFn = func() ([]zfs.Dataset, error) {
		calls++
		return []zfs.Dataset{
			{Name: "tank", Used: 12000000000000, Avail: 11999000000000, Mountpoint: "/tank"},
			{Name: "tank/apps", Used: 1000000000000, Avail: 11999000000000, Mountpoint: "/tank/apps"},
			{Name: "rpool", Used: 900000000000, Avail: 300000000000, Mountpoint: "-"}, // zvol/unmounted: excluded
		}, nil
	}

	usage := zm.DatasetUsage()
	require.Len(t, usage, 2)
	assert.Equal(t, zfsDatasetUsage{used: 12000000000000, avail: 11999000000000}, usage["/tank"])
	assert.Equal(t, zfsDatasetUsage{used: 1000000000000, avail: 11999000000000}, usage["/tank/apps"])
	assert.Equal(t, 1, calls)

	// Second call within the refresh window must not re-run the collector.
	zm.DatasetUsage()
	assert.Equal(t, 1, calls)
}

func TestDatasetUsageRefreshOnErrorKeepsPrevious(t *testing.T) {
	zm := &StoragePoolManager{detailInterval: time.Hour, backends: []*poolBackend{{name: "zfs"}}}
	zm.backends[0].datasetsFn = func() ([]zfs.Dataset, error) {
		return []zfs.Dataset{{Name: "tank", Used: 1, Avail: 1, Mountpoint: "/tank"}}, nil
	}
	assert.Len(t, zm.DatasetUsage(), 1)

	// Force refresh window expiry, then a failing collector.
	zm.backends[0].lastUsageRefresh = time.Now().Add(-10 * time.Minute)
	zm.backends[0].datasetsFn = func() ([]zfs.Dataset, error) {
		return nil, zfs.ErrNoZfs
	}
	usage := zm.DatasetUsage()
	assert.Len(t, usage, 1, "previous usage should be retained on error")
}

func TestGetDetailForceRefresh(t *testing.T) {
	zm := &StoragePoolManager{detailInterval: time.Hour, backends: []*poolBackend{{name: "zfs"}}}
	poolCalls := 0
	zm.backends[0].poolStatsFn = func() ([]zfs.PoolStat, error) {
		poolCalls++
		return []zfs.PoolStat{{Name: "tank", Alloc: uint64(poolCalls)}}, nil
	}
	zm.backends[0].poolStatusesFn = func() ([]zfs.PoolStatus, error) { return nil, nil }
	zm.backends[0].datasetsFn = func() ([]zfs.Dataset, error) { return nil, nil }

	first := zm.GetDetail(false)
	assert.True(t, first.Complete)
	require.Len(t, first.Pools, 1)
	assert.Equal(t, uint64(1), first.Pools[0].Alloc)

	cached := zm.GetDetail(false)
	require.Len(t, cached.Pools, 1)
	assert.Equal(t, uint64(1), cached.Pools[0].Alloc)
	assert.Equal(t, 1, poolCalls)

	refreshed := zm.GetDetail(true)
	assert.True(t, refreshed.Complete)
	require.Len(t, refreshed.Pools, 1)
	assert.Equal(t, uint64(2), refreshed.Pools[0].Alloc)
	assert.Equal(t, 2, poolCalls)
}

func TestGetDetailSuccessfulEmptyInventoryClearsCache(t *testing.T) {
	zm := &StoragePoolManager{detailInterval: time.Hour, backends: []*poolBackend{{name: "zfs"}}}
	zm.backends[0].poolStatsFn = func() ([]zfs.PoolStat, error) {
		return []zfs.PoolStat{{Name: "tank"}}, nil
	}
	zm.backends[0].poolStatusesFn = func() ([]zfs.PoolStatus, error) { return nil, nil }
	zm.backends[0].datasetsFn = func() ([]zfs.Dataset, error) { return nil, nil }

	require.Len(t, zm.GetDetail(false).Pools, 1)
	zm.backends[0].poolStatsFn = func() ([]zfs.PoolStat, error) { return nil, nil }
	empty := zm.GetDetail(true)
	assert.True(t, empty.Complete)
	assert.Empty(t, empty.Pools)
}

func TestGetDetailFailureReturnsIncompleteCachedInventory(t *testing.T) {
	zm := &StoragePoolManager{detailInterval: time.Hour, backends: []*poolBackend{{name: "zfs"}}}
	zm.backends[0].poolStatsFn = func() ([]zfs.PoolStat, error) {
		return []zfs.PoolStat{{Name: "tank"}}, nil
	}
	zm.backends[0].poolStatusesFn = func() ([]zfs.PoolStatus, error) {
		return []zfs.PoolStatus{{Name: "tank", Vdevs: []zfs.VdevStatus{{Name: "mirror-0"}}}}, nil
	}
	zm.backends[0].datasetsFn = func() ([]zfs.Dataset, error) {
		return []zfs.Dataset{{Name: "tank/data"}}, nil
	}
	first := zm.GetDetail(false)
	require.True(t, first.Complete)
	require.Len(t, first.Pools[0].Vdevs, 1)
	require.Len(t, first.Pools[0].Datasets, 1)

	zm.backends[0].poolStatusesFn = func() ([]zfs.PoolStatus, error) { return nil, zfs.ErrNoZfs }
	zm.backends[0].datasetsFn = func() ([]zfs.Dataset, error) { return nil, zfs.ErrNoZfs }
	partial := zm.GetDetail(true)
	require.True(t, partial.Complete)
	require.Len(t, partial.Pools[0].Vdevs, 1)
	require.Len(t, partial.Pools[0].Datasets, 1)

	zm.backends[0].poolStatsFn = func() ([]zfs.PoolStat, error) { return nil, zfs.ErrNoZfs }
	lastSuccessfulRefresh := zm.backends[0].lastDetailRefresh
	failed := zm.GetDetail(true)
	assert.False(t, failed.Complete)
	require.Len(t, failed.Pools, 1)
	assert.Equal(t, "tank", failed.Pools[0].Name)
	assert.Equal(t, lastSuccessfulRefresh, zm.backends[0].lastDetailRefresh)
}

func TestZfsMountpoints(t *testing.T) {
	zm := &StoragePoolManager{detailInterval: time.Hour, backends: []*poolBackend{{name: "zfs"}}}
	zm.backends[0].datasetsFn = func() ([]zfs.Dataset, error) {
		return []zfs.Dataset{
			{Name: "tank", Mountpoint: "/tank"},
			{Name: "rpool/ROOT/pve-1", Mountpoint: "/"},
		}, nil
	}
	mountpoints := zm.ZfsMountpoints()
	assert.Len(t, mountpoints, 2)
	assert.True(t, mountpoints["/tank"])
	assert.True(t, mountpoints["/"])
}

func TestBtrfsRawCapacityPropagates(t *testing.T) {
	zm := &StoragePoolManager{detailInterval: time.Hour, backends: []*poolBackend{{name: "zfs", poolStatsFn: func() ([]zfs.PoolStat, error) {
		return []zfs.PoolStat{btrfsPoolStats(btrfs.Filesystem{UUID: "raw", Name: "raw", Size: 200, Alloc: 100, Raw: true})}, nil
	},
		poolStatusesFn: func() ([]zfs.PoolStatus, error) { return nil, nil },
		datasetsFn:     func() ([]zfs.Dataset, error) { return nil, nil }}}}
	var stats system.Stats
	zm.Update(&stats)
	require.True(t, stats.ZfsPools["b:raw"].Raw)
	detail := zm.GetDetail(true)
	require.True(t, detail.Complete)
	require.Len(t, detail.Pools, 1)
	assert.True(t, detail.Pools[0].Raw)
}

func TestMarkDuplicatePoolCharts(t *testing.T) {
	for _, tc := range []struct {
		name, poolID, device string
		raw                  bool
		diskTotal            float64
		wantUsage, wantIO    bool
	}{
		{"single device root", "fs1", "dm-0", false, 100, true, true},
		{"multi device", "fs1", "", false, 100, true, false},
		{"different IO device", "fs1", "nvme0n1", false, 100, true, false},
		{"different filesystem", "fs2", "dm-0", false, 100, false, false},
		{"unknown identity", "", "dm-0", false, 100, false, false},
		{"raw usage", "fs1", "dm-0", true, 100, false, true},
		{"failed disk collection", "fs1", "dm-0", false, 0, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			zm := &StoragePoolManager{detailInterval: time.Hour, backends: []*poolBackend{{name: "zfs", poolData: []zfs.PoolStat{{Name: "arbitrary label", MountID: tc.poolID, IODevice: tc.device, Raw: tc.raw}}}}}
			stats := &system.Stats{ZfsPools: map[string]*system.ZfsPool{"arbitrary label": {}}}
			fs := map[string]*system.FsStats{"dm-0": {Root: true, Mountpoint: "/", DiskTotal: tc.diskTotal}}
			zm.markDuplicateCharts(stats, fs, func(string) string { return "fs1" })
			assert.Equal(t, tc.wantUsage, stats.ZfsPools["arbitrary label"].HideUsage)
			assert.Equal(t, tc.wantIO, stats.ZfsPools["arbitrary label"].HideIO)
			// Bind mounts and custom extra-filesystem names have the same identity.
			fs["dm-0"].Root = false
			fs["dm-0"].Mountpoint = "/extra-filesystems/storage"
			fs["dm-0"].Name = "custom name"
			stats.ZfsPools["arbitrary label"] = &system.ZfsPool{}
			zm.markDuplicateCharts(stats, fs, func(string) string { return "fs1" })
			assert.Equal(t, tc.wantUsage, stats.ZfsPools["arbitrary label"].HideUsage)
			assert.Equal(t, tc.wantIO, stats.ZfsPools["arbitrary label"].HideIO)
		})
	}
}

func TestBtrfsPoolIdentities(t *testing.T) {
	old := btrfsFilesystems
	t.Cleanup(func() { btrfsFilesystems = old })
	label := "tank"
	btrfsFilesystems = func() ([]btrfs.Filesystem, error) {
		return []btrfs.Filesystem{
			{UUID: "11111111-1111-4111-8111-111111111111", Name: label, Size: 100, Health: "ONLINE", NRead: 100, Devices: []btrfs.Device{{Name: "first"}}},
			{UUID: "22222222-2222-4222-8222-222222222222", Name: "tank", Size: 200, Health: "DEGRADED", NRead: 200, Devices: []btrfs.Device{{Name: "second"}}},
		}, nil
	}
	zm := &StoragePoolManager{detailInterval: time.Hour, backends: []*poolBackend{{name: "zfs", poolStatsFn: func() ([]zfs.PoolStat, error) { return []zfs.PoolStat{{Name: "tank", Size: 300}}, nil },
		kernelStatsFn: func() ([]zfs.PoolKernelStat, error) { return []zfs.PoolKernelStat{{Name: "tank", NRead: 300}}, nil },
		poolStatusesFn: func() ([]zfs.PoolStatus, error) {
			return []zfs.PoolStatus{{Name: "tank", Vdevs: []zfs.VdevStatus{{Name: "zfs-device"}}}}, nil
		},

		datasetsFn: func() ([]zfs.Dataset, error) { return []zfs.Dataset{{Name: "tank/data"}}, nil }}, newBtrfsBackend()}}
	first := "b:11111111-1111-4111-8111-111111111111"
	second := "b:22222222-2222-4222-8222-222222222222"
	var stats system.Stats
	zm.Update(&stats)
	require.Len(t, stats.ZfsPools, 3)
	assert.Contains(t, stats.ZfsPools, "tank")
	assert.Equal(t, "ONLINE", stats.ZfsPools[first].Health)
	assert.Equal(t, "DEGRADED", stats.ZfsPools[second].Health)
	assert.Equal(t, uint64(100), zm.backends[1].kernelSamples[first].nread)
	assert.Equal(t, uint64(200), zm.backends[1].kernelSamples[second].nread)
	detail := zm.GetDetail(true)
	require.Len(t, detail.Pools, 3)
	assert.Equal(t, "zfs-device", detail.Pools[0].Vdevs[0].Name)
	assert.Len(t, detail.Pools[0].Datasets, 1)
	assert.Equal(t, "first", detail.Pools[1].Vdevs[0].Name)
	assert.Empty(t, detail.Pools[1].Datasets)
	assert.Equal(t, "second", detail.Pools[2].Vdevs[0].Name)
	label = "renamed"
	zm.backends[0].lastPoolStats = time.Time{}
	zm.backends[1].lastPoolStats = time.Time{}
	zm.Update(&stats)
	require.Len(t, stats.ZfsPools, 3)
	assert.Equal(t, "renamed", stats.ZfsPools[first].DisplayName)
	assert.Equal(t, first, zm.GetDetail(true).Pools[1].Name)
	assert.Equal(t, "renamed", zm.GetDetail(true).Pools[1].DisplayName)
}
