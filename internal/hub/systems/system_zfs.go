package systems

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/entities/zfs"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

var errIncompleteZfsData = errors.New("incomplete ZFS pool inventory")

type zfsFetchState struct {
	LastAttempt int64
	Successful  bool
}

func (sys *System) supportsZfsData() bool {
	return sys.agentVersion.GTE(beszel.MinVersionZfsData)
}

// FetchAndSaveZfsPools fetches ZFS detail data from the agent and saves it to
// the database. force bypasses the agent's detail cache for manual refreshes.
func (sys *System) FetchAndSaveZfsPools(force bool) error {
	zfsData, err := sys.FetchZfsDataFromAgent(force)
	if err != nil {
		sys.recordZfsFetchResult(err, 0)
		return err
	}
	if zfsData == nil || !zfsData.Complete {
		err = errIncompleteZfsData
		sys.recordZfsFetchResult(err, 0)
		return err
	}
	err = sys.saveZfsPools(zfsData)
	sys.recordZfsFetchResult(err, len(zfsData.Pools))
	return err
}

// recordZfsFetchResult stores a cooldown entry for the ZFS interval and marks
// whether the last fetch produced any pools, so failed setup can retry on reconnect.
func (sys *System) recordZfsFetchResult(err error, poolCount int) {
	if sys.manager == nil {
		return
	}
	interval := sys.zfsFetchInterval()
	success := err == nil && poolCount > 0
	if sys.manager.hub != nil {
		sys.manager.hub.Logger().Info("ZFS fetch result", "system", sys.Id, "success", success, "pools", poolCount, "interval", interval.String(), "err", err)
	}
	sys.manager.zfsFetchMap.Set(sys.Id, zfsFetchState{LastAttempt: time.Now().UnixMilli(), Successful: success}, interval+time.Minute)
}

// shouldFetchZfs returns true when there is no active ZFS cooldown entry for this system.
func (sys *System) shouldFetchZfs() bool {
	if sys.manager == nil {
		return true
	}
	state, ok := sys.manager.zfsFetchMap.GetOk(sys.Id)
	if !ok {
		return true
	}
	return !time.UnixMilli(state.LastAttempt).Add(sys.zfsFetchInterval()).After(time.Now())
}

// zfsFetchInterval returns the agent-provided ZFS interval or the default when unset.
func (sys *System) zfsFetchInterval() time.Duration {
	if sys.zfsInterval > 0 {
		return sys.zfsInterval
	}
	return time.Hour
}

// saveZfsPools saves ZFS pool detail data to the zfs_pools collection and
// removes records for pools no longer reported by a complete agent inventory.
func (sys *System) saveZfsPools(zfsData *zfs.ZfsData) error {
	if zfsData == nil || !zfsData.Complete {
		return errIncompleteZfsData
	}

	hub := sys.manager.hub
	collection, err := hub.FindCachedCollectionByNameOrId("zfs_pools")
	if err != nil {
		return err
	}

	return hub.RunInTransaction(func(txApp core.App) error {
		alive := make(map[string]bool, len(zfsData.Pools))
		for _, pool := range zfsData.Pools {
			if pool == nil {
				continue
			}
			alive[pool.Name] = true
			if err := sys.upsertZfsPoolRecord(txApp, collection, pool); err != nil {
				return err
			}
		}

		existing, err := txApp.FindRecordsByFilter(
			collection,
			"system={:system}",
			"", 0, 0,
			dbx.Params{"system": sys.Id},
		)
		if err != nil {
			return err
		}
		for _, record := range existing {
			if !alive[record.GetString("name")] {
				if err := txApp.Delete(record); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (sys *System) upsertZfsPoolRecord(app core.App, collection *core.Collection, pool *zfs.PoolDetail) error {
	recordID := makeStableHashId(sys.Id, pool.Name)

	record, err := app.FindRecordById(collection, recordID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		record = core.NewRecord(collection)
		record.Set("id", recordID)
	}

	record.Set("system", sys.Id)
	record.Set("name", pool.Name)
	record.Set("health", pool.Health)
	record.Set("size", pool.Size)
	record.Set("alloc", pool.Alloc)
	record.Set("free", pool.Free)
	record.Set("scrub", pool.Scrub)
	record.Set("vdevs", pool.Vdevs)
	record.Set("datasets", pool.Datasets)
	record.Set("details_updated", time.Now().UTC())

	return app.SaveNoValidate(record)
}

// syncZfsPoolHealth persists newly discovered pools and health transitions from
// regular system samples. Detailed fields remain owned by the hourly refresh.
func (sys *System) syncZfsPoolHealth(app core.App, pools map[string]*system.ZfsPool) error {
	if len(pools) == 0 {
		return nil
	}
	collection, err := app.FindCachedCollectionByNameOrId("zfs_pools")
	if err != nil {
		return err
	}
	const gib = 1024 * 1024 * 1024
	for name, pool := range pools {
		if pool == nil {
			continue
		}
		recordID := makeStableHashId(sys.Id, name)
		record, err := app.FindRecordById(collection, recordID)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			record = core.NewRecord(collection)
			record.Set("id", recordID)
			record.Set("system", sys.Id)
			record.Set("name", name)
			record.Set("health", pool.Health)
			record.Set("size", uint64(pool.Total*gib))
			record.Set("alloc", uint64(pool.Used*gib))
			record.Set("free", uint64(max(pool.Total-pool.Used, 0)*gib))
			if err := app.SaveNoValidate(record); err != nil {
				return fmt.Errorf("creating ZFS pool summary %q: %w", name, err)
			}
			continue
		}
		if record.GetString("health") == pool.Health {
			continue
		}
		record.Set("health", pool.Health)
		if err := app.SaveNoValidate(record); err != nil {
			return fmt.Errorf("updating ZFS pool health %q: %w", name, err)
		}
	}
	return nil
}
