//go:build testing

package agent

import (
	"testing"
	"time"

	"github.com/henrygd/beszel/agent/zfs"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdatePopulatesZfsPools(t *testing.T) {
	zm := &ZfsManager{}
	zm.poolStatsFn = func() ([]zfs.PoolStat, error) {
		return []zfs.PoolStat{{Name: "tank", Size: 23999000000000, Alloc: 12000000000000, Free: 11999000000000, Health: "DEGRADED"}}, nil
	}
	zm.datasetsFn = func() ([]zfs.Dataset, error) {
		return []zfs.Dataset{
			{Name: "tank/apps", Used: 5000000000000, Avail: 11999000000000, Mountpoint: "/tank/apps"},
			{Name: "tank/backup", Used: 6000000000000, Avail: 11999000000000, Mountpoint: "/tank/backup"},
			// Small zvol (Proxmox VM EFI disk): must not round to zero.
			{Name: "rpool/vm-100-disk-2", Used: 4194304, Avail: 0, Mountpoint: "-"},
		}, nil
	}
	var kernelCalls int
	zm.kernelStatsFn = func() ([]zfs.PoolKernelStat, error) {
		kernelCalls++
		return []zfs.PoolKernelStat{{
			Name: "tank", Health: "ONLINE",
			NRead: uint64(kernelCalls-1) * 1250, NWrite: uint64(kernelCalls-1) * 5120,
		}}, nil
	}

	var stats system.Stats
	// The first kernel sample establishes the cumulative-counter baseline.
	zm.Update(&stats)
	zm.kernelSamples["tank"] = poolKernelSample{at: time.Now().Add(-time.Second)}
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
	zm := &ZfsManager{}
	zm.poolStatsFn = func() ([]zfs.PoolStat, error) {
		return []zfs.PoolStat{{Name: "tank", Size: 1, Alloc: 1, Health: "ONLINE"}}, nil
	}
	zm.datasetsFn = func() ([]zfs.Dataset, error) { return nil, nil }
	zm.kernelStatsFn = func() ([]zfs.PoolKernelStat, error) {
		return nil, zfs.ErrNoZfs
	}

	var stats system.Stats
	zm.Update(&stats)
	require.NotNil(t, stats.ZfsPools)
	assert.Equal(t, uint64(0), stats.ZfsPools["tank"].ReadBytes)
	assert.Equal(t, uint64(0), stats.ZfsPools["tank"].WriteBytes)
}

func TestUpdateKernelCounterReset(t *testing.T) {
	zm := &ZfsManager{}
	zm.poolStatsFn = func() ([]zfs.PoolStat, error) {
		return []zfs.PoolStat{{Name: "tank", Health: "ONLINE"}}, nil
	}
	zm.datasetsFn = func() ([]zfs.Dataset, error) { return nil, nil }
	zm.kernelSamples = map[string]poolKernelSample{
		"tank": {nread: 100, nwrite: 200, at: time.Now().Add(-time.Second)},
	}
	zm.kernelStatsFn = func() ([]zfs.PoolKernelStat, error) {
		return []zfs.PoolKernelStat{{Name: "tank", Health: "ONLINE", NRead: 10, NWrite: 20}}, nil
	}

	var stats system.Stats
	zm.Update(&stats)
	assert.Equal(t, uint64(0), stats.ZfsPools["tank"].ReadBytes)
	assert.Equal(t, uint64(0), stats.ZfsPools["tank"].WriteBytes)
}

func TestUpdateNoZfs(t *testing.T) {
	zm := &ZfsManager{}
	calls := 0
	zm.poolStatsFn = func() ([]zfs.PoolStat, error) {
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
	zm := &ZfsManager{}
	calls := 0
	zm.poolStatsFn = func() ([]zfs.PoolStat, error) {
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
	zm := &ZfsManager{}
	calls := 0
	zm.datasetsFn = func() ([]zfs.Dataset, error) {
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
	zm := &ZfsManager{}
	zm.datasetsFn = func() ([]zfs.Dataset, error) {
		return []zfs.Dataset{{Name: "tank", Used: 1, Avail: 1, Mountpoint: "/tank"}}, nil
	}
	assert.Len(t, zm.DatasetUsage(), 1)

	// Force refresh window expiry, then a failing collector.
	zm.lastUsageRefresh = time.Now().Add(-10 * time.Minute)
	zm.datasetsFn = func() ([]zfs.Dataset, error) {
		return nil, zfs.ErrNoZfs
	}
	usage := zm.DatasetUsage()
	assert.Len(t, usage, 1, "previous usage should be retained on error")
}

func TestGetDetailForceRefresh(t *testing.T) {
	zm := &ZfsManager{detailInterval: time.Hour}
	poolCalls := 0
	zm.poolStatsFn = func() ([]zfs.PoolStat, error) {
		poolCalls++
		return []zfs.PoolStat{{Name: "tank", Alloc: uint64(poolCalls)}}, nil
	}
	zm.poolStatusesFn = func() ([]zfs.PoolStatus, error) { return nil, nil }
	zm.datasetsFn = func() ([]zfs.Dataset, error) { return nil, nil }

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
	zm := &ZfsManager{detailInterval: time.Hour}
	zm.poolStatsFn = func() ([]zfs.PoolStat, error) {
		return []zfs.PoolStat{{Name: "tank"}}, nil
	}
	zm.poolStatusesFn = func() ([]zfs.PoolStatus, error) { return nil, nil }
	zm.datasetsFn = func() ([]zfs.Dataset, error) { return nil, nil }

	require.Len(t, zm.GetDetail(false).Pools, 1)
	zm.poolStatsFn = func() ([]zfs.PoolStat, error) { return nil, nil }
	empty := zm.GetDetail(true)
	assert.True(t, empty.Complete)
	assert.Empty(t, empty.Pools)
}

func TestGetDetailFailureReturnsIncompleteCachedInventory(t *testing.T) {
	zm := &ZfsManager{detailInterval: time.Hour}
	zm.poolStatsFn = func() ([]zfs.PoolStat, error) {
		return []zfs.PoolStat{{Name: "tank"}}, nil
	}
	zm.poolStatusesFn = func() ([]zfs.PoolStatus, error) {
		return []zfs.PoolStatus{{Name: "tank", Vdevs: []zfs.VdevStatus{{Name: "mirror-0"}}}}, nil
	}
	zm.datasetsFn = func() ([]zfs.Dataset, error) {
		return []zfs.Dataset{{Name: "tank/data"}}, nil
	}
	first := zm.GetDetail(false)
	require.True(t, first.Complete)
	require.Len(t, first.Pools[0].Vdevs, 1)
	require.Len(t, first.Pools[0].Datasets, 1)

	zm.poolStatusesFn = func() ([]zfs.PoolStatus, error) { return nil, zfs.ErrNoZfs }
	zm.datasetsFn = func() ([]zfs.Dataset, error) { return nil, zfs.ErrNoZfs }
	partial := zm.GetDetail(true)
	require.True(t, partial.Complete)
	require.Len(t, partial.Pools[0].Vdevs, 1)
	require.Len(t, partial.Pools[0].Datasets, 1)

	zm.poolStatsFn = func() ([]zfs.PoolStat, error) { return nil, zfs.ErrNoZfs }
	lastSuccessfulRefresh := zm.lastDetailRefresh
	failed := zm.GetDetail(true)
	assert.False(t, failed.Complete)
	require.Len(t, failed.Pools, 1)
	assert.Equal(t, "tank", failed.Pools[0].Name)
	assert.Equal(t, lastSuccessfulRefresh, zm.lastDetailRefresh)
}

func TestZfsMountpoints(t *testing.T) {
	zm := &ZfsManager{}
	zm.datasetsFn = func() ([]zfs.Dataset, error) {
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
