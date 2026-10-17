//go:build testing

package alerts_test

import (
	"encoding/json"
	"testing"
	"testing/synctest"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
	beszelTests "github.com/henrygd/beszel/internal/tests"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInodesAlertExtraFsMultiMinute tests that multi-minute inode alerts correctly use
// historical per-minute values for extra (non-root) filesystems, not the current live snapshot.
func TestInodesAlertExtraFsMultiMinute(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "paused")
	require.NoError(t, err)
	systemRecord := systems[0]

	inodeAlert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":   "Inodes",
		"system": systemRecord.Id,
		"user":   user.Id,
		"value":  80,
		"min":    2,
	})
	require.NoError(t, err)
	assert.False(t, inodeAlert.GetBool("triggered"), "Alert should not be triggered initially")

	am := hub.GetAlertManager()
	now := time.Now().UTC()

	extraFsHigh := map[string]*system.FsStats{
		"/mnt/data": {InodesTotal: 1000, InodesUsed: 920}, // 92% - above threshold
	}

	recordTimes := []time.Duration{
		-180 * time.Second,
		-90 * time.Second,
		-60 * time.Second,
		-30 * time.Second,
	}

	for _, offset := range recordTimes {
		stats := system.Stats{
			InodesPct: 30,
			ExtraFs:   extraFsHigh,
		}
		statsJSON, _ := json.Marshal(stats)

		recordTime := now.Add(offset)
		record, err := beszelTests.CreateRecord(hub, "system_stats", map[string]any{
			"system": systemRecord.Id,
			"type":   "1m",
			"stats":  string(statsJSON),
		})
		require.NoError(t, err)
		record.SetRaw("created", recordTime.Format(types.DefaultDateLayout))
		err = hub.SaveNoValidate(record)
		require.NoError(t, err)
	}

	combinedDataHigh := &system.CombinedData{
		Stats: system.Stats{
			InodesPct: 30,
			ExtraFs:   extraFsHigh,
		},
		Info: system.Info{
			InodesPct: 30,
		},
	}

	systemRecord.Set("updated", now)

	synctest.Test(t, func(t *testing.T) {
		require.NoError(t, am.HandleSystemAlerts(systemRecord, combinedDataHigh))
	})

	inodeAlert, err = hub.FindFirstRecordByFilter("alerts", "id={:id}", dbx.Params{"id": inodeAlert.Id})
	require.NoError(t, err)
	assert.True(t, inodeAlert.GetBool("triggered"),
		"Alert SHOULD be triggered when extra filesystem inode average (92%%) exceeds threshold (80%%)")

	extraFsLow := map[string]*system.FsStats{
		"/mnt/data": {InodesTotal: 1000, InodesUsed: 500}, // 50% - below threshold
	}

	newNow := now.Add(2 * time.Minute)
	recordTimesLow := []time.Duration{
		-180 * time.Second,
		-90 * time.Second,
		-60 * time.Second,
		-30 * time.Second,
	}

	for _, offset := range recordTimesLow {
		stats := system.Stats{
			InodesPct: 30,
			ExtraFs:   extraFsLow,
		}
		statsJSON, _ := json.Marshal(stats)

		recordTime := newNow.Add(offset)
		record, err := beszelTests.CreateRecord(hub, "system_stats", map[string]any{
			"system": systemRecord.Id,
			"type":   "1m",
			"stats":  string(statsJSON),
		})
		require.NoError(t, err)
		record.SetRaw("created", recordTime.Format(types.DefaultDateLayout))
		err = hub.SaveNoValidate(record)
		require.NoError(t, err)
	}

	combinedDataLow := &system.CombinedData{
		Stats: system.Stats{
			InodesPct: 30,
			ExtraFs:   extraFsLow,
		},
		Info: system.Info{
			InodesPct: 30,
		},
	}

	systemRecord.Set("updated", newNow)

	synctest.Test(t, func(t *testing.T) {
		require.NoError(t, am.HandleSystemAlerts(systemRecord, combinedDataLow))
	})

	inodeAlert, err = hub.FindFirstRecordByFilter("alerts", "id={:id}", dbx.Params{"id": inodeAlert.Id})
	require.NoError(t, err)
	assert.False(t, inodeAlert.GetBool("triggered"),
		"Alert should be resolved when extra filesystem inode average (50%%) drops below threshold (80%%)")
}
