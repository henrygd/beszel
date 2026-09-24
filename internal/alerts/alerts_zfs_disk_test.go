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

// TestDiskAlertZfsPoolMultiMinute verifies that ZFS pool usage participates in
// the Disk threshold alert using historical per-minute values, mirroring the
// extra-filesystem behavior.
func TestDiskAlertZfsPoolMultiMinute(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
	require.NoError(t, err)
	systemRecord := systems[0]

	diskAlert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":   "Disk",
		"system": systemRecord.Id,
		"user":   user.Id,
		"value":  80, // threshold: 80%
		"min":    2,  // requires historical averaging
	})
	require.NoError(t, err)

	am := hub.GetAlertManager()
	now := time.Now().UTC()

	poolHigh := map[string]*system.ZfsPool{
		"tank": {Total: 1000, Used: 920}, // 92% - above threshold
	}

	recordTimes := []time.Duration{
		-180 * time.Second,
		-90 * time.Second,
		-60 * time.Second,
		-30 * time.Second,
	}

	for _, offset := range recordTimes {
		stats := system.Stats{
			DiskPct:  30, // root disk at 30% - below threshold
			ZfsPools: poolHigh,
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
			DiskPct:  30,
			ZfsPools: poolHigh,
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
		"Alert should be triggered when ZFS pool average (92%%) exceeds threshold (80%%)")

	// --- Resolution: pool drops to 50%, alert should resolve ---

	poolLow := map[string]*system.ZfsPool{
		"tank": {Total: 1000, Used: 500}, // 50% - below threshold
	}

	newNow := now.Add(2 * time.Minute)
	for _, offset := range recordTimes {
		stats := system.Stats{
			DiskPct:  30,
			ZfsPools: poolLow,
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
			DiskPct:  30,
			ZfsPools: poolLow,
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
		"Alert should be resolved when ZFS pool average (50%%) drops below threshold (80%%)")
}

func TestDiskAlertIgnoresRawPool(t *testing.T) {
	for _, minutes := range []int{0, 2} {
		hub, user := beszelTests.GetHubWithUser(t)
		systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
		require.NoError(t, err)
		alert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{"name": "Disk", "system": systems[0].Id, "user": user.Id, "value": 80, "min": minutes})
		require.NoError(t, err)
		pools := map[string]*system.ZfsPool{"btrfs": {Total: 100, Used: 99, Raw: true}}
		for _, offset := range []time.Duration{-180, -90, -60, -30} {
			data, err := json.Marshal(system.Stats{ZfsPools: pools})
			require.NoError(t, err)
			record, err := beszelTests.CreateRecord(hub, "system_stats", map[string]any{"system": systems[0].Id, "type": "1m", "stats": string(data)})
			require.NoError(t, err)
			record.SetRaw("created", time.Now().UTC().Add(offset*time.Second).Format(types.DefaultDateLayout))
			require.NoError(t, hub.SaveNoValidate(record))
		}
		require.NoError(t, hub.GetAlertManager().HandleSystemAlerts(systems[0], &system.CombinedData{Stats: system.Stats{ZfsPools: pools}}))
		time.Sleep(20 * time.Millisecond)
		record, err := hub.FindRecordById("alerts", alert.Id)
		require.NoError(t, err)
		assert.False(t, record.GetBool("triggered"))
		if minutes > 0 {
			// A current usable sample must not make raw historical values eligible.
			pools["btrfs"].Raw = false
			require.NoError(t, hub.GetAlertManager().HandleSystemAlerts(systems[0], &system.CombinedData{Stats: system.Stats{ZfsPools: pools}}))
			time.Sleep(20 * time.Millisecond)
			record, err = hub.FindRecordById("alerts", alert.Id)
			require.NoError(t, err)
			assert.False(t, record.GetBool("triggered"))
		}
		hub.Cleanup()
	}
}
