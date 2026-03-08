//go:build testing

package alerts_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
	beszelTests "github.com/henrygd/beszel/internal/tests"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDiskAlertExtraFsMultiMinute tests that multi-minute disk alerts correctly use
// historical per-minute values for extra (non-root) filesystems, not the current live snapshot.
func TestDiskAlertExtraFsMultiMinute(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
	require.NoError(t, err)
	systemRecord := systems[0]

	// Disk alert: threshold 80%, min=2 (requires historical averaging)
	diskAlert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":   "Disk",
		"system": systemRecord.Id,
		"user":   user.Id,
		"value":  80, // threshold: 80%
		"min":    2,  // 2 minutes - requires historical averaging
	})
	require.NoError(t, err)
	assert.False(t, diskAlert.GetBool("triggered"), "Alert should not be triggered initially")

	am := hub.GetAlertManager()
	now := time.Now().UTC()

	extraFsHigh := map[string]*system.FsStats{
		"/mnt/data": {DiskTotal: 1000, DiskUsed: 920}, // 92% - above threshold
	}

	// Insert 4 historical records spread over 3 minutes (same pattern as battery tests).
	// The oldest record must predate (now - 2min) so the alert time window is valid.
	recordTimes := []time.Duration{
		-180 * time.Second, // 3 min ago - anchors oldest record before alert.time
		-90 * time.Second,
		-60 * time.Second,
		-30 * time.Second,
	}

	for _, offset := range recordTimes {
		stats := system.Stats{
			DiskPct: 30, // root disk at 30% - below threshold
			ExtraFs: extraFsHigh,
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
			DiskPct: 30,
			ExtraFs: extraFsHigh,
		},
		Info: system.Info{
			DiskPct: 30,
		},
	}

	systemRecord.Set("updated", now)
	err = hub.SaveNoValidate(systemRecord)
	require.NoError(t, err)

	err = am.HandleSystemAlerts(systemRecord, combinedDataHigh)
	require.NoError(t, err)

	time.Sleep(20 * time.Millisecond)

	diskAlert, err = hub.FindFirstRecordByFilter("alerts", "id={:id}", dbx.Params{"id": diskAlert.Id})
	require.NoError(t, err)
	assert.True(t, diskAlert.GetBool("triggered"),
		"Alert SHOULD be triggered when extra disk average (92%%) exceeds threshold (80%%)")

	// --- Resolution: extra disk drops to 50%, alert should resolve ---

	extraFsLow := map[string]*system.FsStats{
		"/mnt/data": {DiskTotal: 1000, DiskUsed: 500}, // 50% - below threshold
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
			DiskPct: 30,
			ExtraFs: extraFsLow,
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
			DiskPct: 30,
			ExtraFs: extraFsLow,
		},
		Info: system.Info{
			DiskPct: 30,
		},
	}

	systemRecord.Set("updated", newNow)
	err = hub.SaveNoValidate(systemRecord)
	require.NoError(t, err)

	err = am.HandleSystemAlerts(systemRecord, combinedDataLow)
	require.NoError(t, err)

	time.Sleep(20 * time.Millisecond)

	diskAlert, err = hub.FindFirstRecordByFilter("alerts", "id={:id}", dbx.Params{"id": diskAlert.Id})
	require.NoError(t, err)
	assert.False(t, diskAlert.GetBool("triggered"),
		"Alert should be resolved when extra disk average (50%%) drops below threshold (80%%)")
}
