//go:build testing

package systems

import (
	"errors"
	"testing"
	"time"

	"github.com/blang/semver"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/entities/zfs"
	"github.com/henrygd/beszel/internal/hub/expirymap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSupportsZfsData(t *testing.T) {
	sys := &System{agentVersion: semver.MustParse("0.18.8")}
	assert.False(t, sys.supportsZfsData())

	sys.agentVersion = semver.MustParse("0.18.9")
	assert.True(t, sys.supportsZfsData())
}

func TestRecordZfsFetchResult(t *testing.T) {
	sm := &SystemManager{zfsFetchMap: expirymap.New[zfsFetchState](time.Hour)}
	t.Cleanup(sm.zfsFetchMap.StopCleaner)

	sys := &System{
		Id:          "system-1",
		manager:     sm,
		zfsInterval: time.Hour,
	}

	// Successful fetch with pools
	sys.recordZfsFetchResult(nil, 2)
	state, ok := sm.zfsFetchMap.GetOk(sys.Id)
	assert.True(t, ok, "expected zfs fetch result to be stored")
	assert.True(t, state.Successful, "expected successful fetch state to be recorded")

	// Failed fetch
	sys.recordZfsFetchResult(errors.New("failed"), 0)
	state, ok = sm.zfsFetchMap.GetOk(sys.Id)
	assert.True(t, ok, "expected failed zfs fetch state to be stored")
	assert.False(t, state.Successful, "expected failed zfs fetch state to be marked unsuccessful")

	// Successful fetch but no pools
	sys.recordZfsFetchResult(nil, 0)
	state, ok = sm.zfsFetchMap.GetOk(sys.Id)
	assert.True(t, ok, "expected fetch with zero pools to be stored")
	assert.False(t, state.Successful, "expected fetch with zero pools to be marked unsuccessful")
}

func TestShouldFetchZfs(t *testing.T) {
	sm := &SystemManager{zfsFetchMap: expirymap.New[zfsFetchState](time.Hour)}
	t.Cleanup(sm.zfsFetchMap.StopCleaner)

	sys := &System{
		Id:          "system-1",
		manager:     sm,
		zfsInterval: time.Hour,
	}

	assert.True(t, sys.shouldFetchZfs(), "expected initial zfs fetch to be allowed")

	sys.recordZfsFetchResult(errors.New("failed"), 0)
	assert.False(t, sys.shouldFetchZfs(), "expected zfs fetch to be blocked while interval entry exists")

	sm.zfsFetchMap.Remove(sys.Id)
	assert.True(t, sys.shouldFetchZfs(), "expected zfs fetch to be allowed after interval entry is cleared")
}

func TestZfsFetchIntervalDefault(t *testing.T) {
	sys := &System{}
	assert.Equal(t, time.Hour, sys.zfsFetchInterval())

	sys.zfsInterval = 5 * time.Minute
	assert.Equal(t, 5*time.Minute, sys.zfsFetchInterval())
}

func TestResetFailedZfsFetchState(t *testing.T) {
	sm := &SystemManager{zfsFetchMap: expirymap.New[zfsFetchState](time.Hour)}
	t.Cleanup(sm.zfsFetchMap.StopCleaner)

	sm.zfsFetchMap.Set("system-1", zfsFetchState{LastAttempt: time.Now().UnixMilli(), Successful: false}, time.Hour)
	sm.resetFailedZfsFetchState("system-1")
	_, ok := sm.zfsFetchMap.GetOk("system-1")
	assert.False(t, ok, "expected failed zfs fetch state to be cleared on reconnect")

	sm.zfsFetchMap.Set("system-1", zfsFetchState{LastAttempt: time.Now().UnixMilli(), Successful: true}, time.Hour)
	sm.resetFailedZfsFetchState("system-1")
	_, ok = sm.zfsFetchMap.GetOk("system-1")
	assert.True(t, ok, "expected successful zfs fetch state to be preserved")
}

func TestSaveZfsPoolsCompleteEmptyPrunesFinalPool(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	require.NoError(t, sys.saveZfsPools(&zfs.ZfsData{
		Complete: true,
		Pools:    []*zfs.PoolDetail{{Name: "tank", Health: "ONLINE"}},
	}))
	records, err := app.FindRecordsByFilter("zfs_pools", "system={:system}", "", 0, 0, map[string]any{"system": sys.Id})
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.False(t, records[0].GetDateTime("details_updated").Time().IsZero())

	require.NoError(t, sys.saveZfsPools(&zfs.ZfsData{Complete: true}))
	records, err = app.FindRecordsByFilter("zfs_pools", "system={:system}", "", 0, 0, map[string]any{"system": sys.Id})
	require.NoError(t, err)
	assert.Empty(t, records)
}

func TestSaveZfsPoolsIncompletePreservesRecords(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	require.NoError(t, sys.saveZfsPools(&zfs.ZfsData{
		Complete: true,
		Pools:    []*zfs.PoolDetail{{Name: "tank", Health: "ONLINE"}},
	}))
	assert.ErrorIs(t, sys.saveZfsPools(&zfs.ZfsData{}), errIncompleteZfsData)
	records, err := app.FindRecordsByFilter("zfs_pools", "system={:system}", "", 0, 0, map[string]any{"system": sys.Id})
	require.NoError(t, err)
	assert.Len(t, records, 1)
}

func TestSavePartialBackendInventory(t *testing.T) {
	for _, healthy := range []string{"zfs", "btrfs"} {
		t.Run(healthy, func(t *testing.T) {
			sys, app := newTestSystemWithHub(t)
			healthyKey, failedKey := "tank", "b:uuid"
			if healthy == "btrfs" {
				healthyKey, failedKey = failedKey, healthyKey
			}
			initial := &zfs.ZfsData{Complete: true, Pools: []*zfs.PoolDetail{
				{Name: healthyKey, Alloc: 10}, {Name: failedKey, Alloc: 10},
			}}
			require.NoError(t, sys.saveZfsPools(initial))
			failedID := makeStableHashId(sys.Id, failedKey)
			before, err := app.FindRecordById("zfs_pools", failedID)
			require.NoError(t, err)
			partial := &zfs.ZfsData{CompleteBackends: []string{healthy}, Pools: []*zfs.PoolDetail{
				{Name: healthyKey, Alloc: 20}, {Name: failedKey, Alloc: 99},
			}}
			assert.ErrorIs(t, sys.saveZfsPools(partial), errIncompleteZfsData)
			fresh, err := app.FindRecordById("zfs_pools", makeStableHashId(sys.Id, healthyKey))
			require.NoError(t, err)
			assert.EqualValues(t, 20, fresh.GetInt("alloc"))
			cached, err := app.FindRecordById("zfs_pools", failedID)
			require.NoError(t, err)
			assert.EqualValues(t, 10, cached.GetInt("alloc"))
			assert.Equal(t, before.GetDateTime("details_updated"), cached.GetDateTime("details_updated"))
			// An empty successful backend can prune, even while the other fails.
			partial.Pools = nil
			assert.ErrorIs(t, sys.saveZfsPools(partial), errIncompleteZfsData)
			records, err := app.FindRecordsByFilter("zfs_pools", "system={:system}", "", 0, 0, map[string]any{"system": sys.Id})
			require.NoError(t, err)
			require.Len(t, records, 1)
			assert.Equal(t, failedKey, records[0].GetString("name"))
			require.NoError(t, sys.saveZfsPools(&zfs.ZfsData{Complete: true}))
		})
	}
}

func TestSyncZfsPoolHealthWritesOnlyTransitions(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	collection, err := app.FindCachedCollectionByNameOrId("zfs_pools")
	require.NoError(t, err)

	require.NoError(t, sys.syncZfsPoolHealth(app, map[string]*system.ZfsPool{
		"tank": {Total: 100, Used: 25, Health: "ONLINE"},
	}))
	record, err := app.FindRecordById(collection, makeStableHashId(sys.Id, "tank"))
	require.NoError(t, err)
	firstUpdated := record.GetDateTime("updated")
	assert.Equal(t, "ONLINE", record.GetString("health"))
	assert.EqualValues(t, 100*1024*1024*1024, record.GetInt("size"))

	require.NoError(t, sys.syncZfsPoolHealth(app, map[string]*system.ZfsPool{
		"tank": {Total: 100, Used: 30, Health: "ONLINE"},
	}))
	record, err = app.FindRecordById(collection, record.Id)
	require.NoError(t, err)
	assert.Equal(t, firstUpdated, record.GetDateTime("updated"))

	require.NoError(t, sys.syncZfsPoolHealth(app, map[string]*system.ZfsPool{
		"tank": {Total: 100, Used: 30, Health: "DEGRADED"},
	}))
	record, err = app.FindRecordById(collection, record.Id)
	require.NoError(t, err)
	assert.Equal(t, "DEGRADED", record.GetString("health"))
}

func TestZfsRawCapacityPersistence(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	require.NoError(t, sys.saveZfsPools(&zfs.ZfsData{Complete: true, Pools: []*zfs.PoolDetail{{Name: "btrfs", Size: 200, Alloc: 10, Raw: true}}}))
	record, err := app.FindRecordById("zfs_pools", makeStableHashId(sys.Id, "btrfs"))
	require.NoError(t, err)
	require.True(t, record.GetBool("raw"))
	require.NoError(t, sys.syncZfsPoolHealth(app, map[string]*system.ZfsPool{"btrfs": {Total: 1, Used: 0.25}}))
	record, err = app.FindRecordById("zfs_pools", record.Id)
	require.NoError(t, err)
	assert.False(t, record.GetBool("raw"))
	assert.EqualValues(t, 1024*1024*1024, record.GetInt("size"))
}

func TestBtrfsDisplayNameKeepsRecordIdentity(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	key := "b:11111111-1111-4111-8111-111111111111"
	require.NoError(t, sys.syncZfsPoolHealth(app, map[string]*system.ZfsPool{
		key:    {DisplayName: "tank", Health: "ONLINE"},
		"tank": {Health: "ONLINE"},
	}))
	id := makeStableHashId(sys.Id, key)
	record, err := app.FindRecordById("zfs_pools", id)
	require.NoError(t, err)
	assert.Equal(t, "tank", record.GetString("display_name"))
	require.NoError(t, sys.syncZfsPoolHealth(app, map[string]*system.ZfsPool{key: {DisplayName: "renamed", Health: "ONLINE"}}))
	record, err = app.FindRecordById("zfs_pools", id)
	require.NoError(t, err)
	assert.Equal(t, key, record.GetString("name"))
	assert.Equal(t, "renamed", record.GetString("display_name"))
	require.NoError(t, sys.saveZfsPools(&zfs.ZfsData{Complete: true, Pools: []*zfs.PoolDetail{
		{Name: key, DisplayName: "detail name", Health: "ONLINE"}, {Name: "tank", Health: "ONLINE"},
	}}))
	record, err = app.FindRecordById("zfs_pools", id)
	require.NoError(t, err)
	assert.Equal(t, "detail name", record.GetString("display_name"))
	_, err = app.FindRecordById("zfs_pools", makeStableHashId(sys.Id, "tank"))
	require.NoError(t, err)
}
