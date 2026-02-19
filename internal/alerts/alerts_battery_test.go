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

// TestBatteryAlertLogic tests that battery alerts trigger when value drops BELOW threshold
// (opposite of other alerts like CPU, Memory, etc. which trigger when exceeding threshold)
func TestBatteryAlertLogic(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	// Create a system
	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
	require.NoError(t, err)
	systemRecord := systems[0]

	// Create a battery alert with threshold of 20% and min of 1 minute (immediate trigger)
	batteryAlert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":   "Battery",
		"system": systemRecord.Id,
		"user":   user.Id,
		"value":  20, // threshold: 20%
		"min":    1,  // 1 minute (immediate trigger for testing)
	})
	require.NoError(t, err)

	// Verify alert is not triggered initially
	assert.False(t, batteryAlert.GetBool("triggered"), "Alert should not be triggered initially")

	// Create system stats with battery at 50% (above threshold - should NOT trigger)
	statsHigh := system.Stats{
		Cpu:     10,
		MemPct:  30,
		DiskPct: 40,
		Battery: [2]uint8{50, 1}, // 50% battery, discharging
	}
	statsHighJSON, _ := json.Marshal(statsHigh)
	_, err = beszelTests.CreateRecord(hub, "system_stats", map[string]any{
		"system": systemRecord.Id,
		"type":   "1m",
		"stats":  string(statsHighJSON),
	})
	require.NoError(t, err)

	// Create CombinedData for the alert handler
	combinedDataHigh := &system.CombinedData{
		Stats: statsHigh,
		Info: system.Info{
			AgentVersion: "0.12.0",
			Cpu:          10,
			MemPct:       30,
			DiskPct:      40,
		},
	}

	// Simulate system update time
	systemRecord.Set("updated", time.Now().UTC())
	err = hub.SaveNoValidate(systemRecord)
	require.NoError(t, err)

	// Handle system alerts with high battery
	am := hub.GetAlertManager()
	err = am.HandleSystemAlerts(systemRecord, combinedDataHigh)
	require.NoError(t, err)

	// Verify alert is still NOT triggered (battery 50% is above threshold 20%)
	batteryAlert, err = hub.FindFirstRecordByFilter("alerts", "id={:id}", dbx.Params{"id": batteryAlert.Id})
	require.NoError(t, err)
	assert.False(t, batteryAlert.GetBool("triggered"), "Alert should NOT be triggered when battery (50%%) is above threshold (20%%)")

	// Now create stats with battery at 15% (below threshold - should trigger)
	statsLow := system.Stats{
		Cpu:     10,
		MemPct:  30,
		DiskPct: 40,
		Battery: [2]uint8{15, 1}, // 15% battery, discharging
	}
	statsLowJSON, _ := json.Marshal(statsLow)
	_, err = beszelTests.CreateRecord(hub, "system_stats", map[string]any{
		"system": systemRecord.Id,
		"type":   "1m",
		"stats":  string(statsLowJSON),
	})
	require.NoError(t, err)

	combinedDataLow := &system.CombinedData{
		Stats: statsLow,
		Info: system.Info{
			AgentVersion: "0.12.0",
			Cpu:          10,
			MemPct:       30,
			DiskPct:      40,
		},
	}

	// Update system timestamp
	systemRecord.Set("updated", time.Now().UTC())
	err = hub.SaveNoValidate(systemRecord)
	require.NoError(t, err)

	// Handle system alerts with low battery
	err = am.HandleSystemAlerts(systemRecord, combinedDataLow)
	require.NoError(t, err)

	// Wait for the alert to be processed
	time.Sleep(20 * time.Millisecond)

	// Verify alert IS triggered (battery 15% is below threshold 20%)
	batteryAlert, err = hub.FindFirstRecordByFilter("alerts", "id={:id}", dbx.Params{"id": batteryAlert.Id})
	require.NoError(t, err)
	assert.True(t, batteryAlert.GetBool("triggered"), "Alert SHOULD be triggered when battery (15%%) drops below threshold (20%%)")

	// Now test resolution: battery goes back above threshold
	statsRecovered := system.Stats{
		Cpu:     10,
		MemPct:  30,
		DiskPct: 40,
		Battery: [2]uint8{25, 1}, // 25% battery, discharging
	}
	statsRecoveredJSON, _ := json.Marshal(statsRecovered)
	_, err = beszelTests.CreateRecord(hub, "system_stats", map[string]any{
		"system": systemRecord.Id,
		"type":   "1m",
		"stats":  string(statsRecoveredJSON),
	})
	require.NoError(t, err)

	combinedDataRecovered := &system.CombinedData{
		Stats: statsRecovered,
		Info: system.Info{
			AgentVersion: "0.12.0",
			Cpu:          10,
			MemPct:       30,
			DiskPct:      40,
		},
	}

	// Update system timestamp
	systemRecord.Set("updated", time.Now().UTC())
	err = hub.SaveNoValidate(systemRecord)
	require.NoError(t, err)

	// Handle system alerts with recovered battery
	err = am.HandleSystemAlerts(systemRecord, combinedDataRecovered)
	require.NoError(t, err)

	// Wait for the alert to be processed
	time.Sleep(20 * time.Millisecond)

	// Verify alert is now resolved (battery 25% is above threshold 20%)
	batteryAlert, err = hub.FindFirstRecordByFilter("alerts", "id={:id}", dbx.Params{"id": batteryAlert.Id})
	require.NoError(t, err)
	assert.False(t, batteryAlert.GetBool("triggered"), "Alert should be resolved when battery (25%%) goes above threshold (20%%)")
}

// TestBatteryAlertNoBattery verifies that systems without battery data don't trigger alerts
func TestBatteryAlertNoBattery(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	// Create a system
	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
	require.NoError(t, err)
	systemRecord := systems[0]

	// Create a battery alert
	batteryAlert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":   "Battery",
		"system": systemRecord.Id,
		"user":   user.Id,
		"value":  20,
		"min":    1,
	})
	require.NoError(t, err)

	// Create stats with NO battery data (Battery[0] = 0)
	statsNoBattery := system.Stats{
		Cpu:     10,
		MemPct:  30,
		DiskPct: 40,
		Battery: [2]uint8{0, 0}, // No battery
	}

	combinedData := &system.CombinedData{
		Stats: statsNoBattery,
		Info: system.Info{
			AgentVersion: "0.12.0",
			Cpu:          10,
			MemPct:       30,
			DiskPct:      40,
		},
	}

	// Simulate system update time
	systemRecord.Set("updated", time.Now().UTC())
	err = hub.SaveNoValidate(systemRecord)
	require.NoError(t, err)

	// Handle system alerts
	am := hub.GetAlertManager()
	err = am.HandleSystemAlerts(systemRecord, combinedData)
	require.NoError(t, err)

	// Wait a moment for processing
	time.Sleep(20 * time.Millisecond)

	// Verify alert is NOT triggered (no battery data should skip the alert)
	batteryAlert, err = hub.FindFirstRecordByFilter("alerts", "id={:id}", dbx.Params{"id": batteryAlert.Id})
	require.NoError(t, err)
	assert.False(t, batteryAlert.GetBool("triggered"), "Alert should NOT be triggered when system has no battery")
}

// TestBatteryAlertAveragedSamples tests battery alerts with min > 1 (averaging multiple samples)
// This ensures the inverted threshold logic works correctly across averaged time windows
func TestBatteryAlertAveragedSamples(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	// Create a system
	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
	require.NoError(t, err)
	systemRecord := systems[0]

	// Create a battery alert with threshold of 25% and min of 2 minutes (requires averaging)
	batteryAlert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":   "Battery",
		"system": systemRecord.Id,
		"user":   user.Id,
		"value":  25, // threshold: 25%
		"min":    2,  // 2 minutes - requires averaging
	})
	require.NoError(t, err)

	// Verify alert is not triggered initially
	assert.False(t, batteryAlert.GetBool("triggered"), "Alert should not be triggered initially")

	am := hub.GetAlertManager()
	now := time.Now().UTC()

	// Create system_stats records with low battery (below threshold)
	// The alert has min=2 minutes, so alert.time = now - 2 minutes
	// For the alert to be valid, alert.time must be AFTER the oldest record's created time
	// So we need records older than (now - 2 min), plus records within the window
	// Records at: now-3min (oldest, before window), now-90s, now-60s, now-30s
	recordTimes := []time.Duration{
		-180 * time.Second, // 3 min ago - this makes the oldest record before alert.time
		-90 * time.Second,
		-60 * time.Second,
		-30 * time.Second,
	}

	for _, offset := range recordTimes {
		statsLow := system.Stats{
			Cpu:     10,
			MemPct:  30,
			DiskPct: 40,
			Battery: [2]uint8{15, 1}, // 15% battery (below 25% threshold)
		}
		statsLowJSON, _ := json.Marshal(statsLow)

		recordTime := now.Add(offset)
		record, err := beszelTests.CreateRecord(hub, "system_stats", map[string]any{
			"system": systemRecord.Id,
			"type":   "1m",
			"stats":  string(statsLowJSON),
		})
		require.NoError(t, err)
		// Update created time to simulate historical records - use SetRaw with formatted string
		record.SetRaw("created", recordTime.Format(types.DefaultDateLayout))
		err = hub.SaveNoValidate(record)
		require.NoError(t, err)
	}

	// Create combined data with low battery
	combinedDataLow := &system.CombinedData{
		Stats: system.Stats{
			Cpu:     10,
			MemPct:  30,
			DiskPct: 40,
			Battery: [2]uint8{15, 1},
		},
		Info: system.Info{
			AgentVersion: "0.12.0",
			Cpu:          10,
			MemPct:       30,
			DiskPct:      40,
		},
	}

	// Update system timestamp
	systemRecord.Set("updated", now)
	err = hub.SaveNoValidate(systemRecord)
	require.NoError(t, err)

	// Handle system alerts - should trigger because average battery is below threshold
	err = am.HandleSystemAlerts(systemRecord, combinedDataLow)
	require.NoError(t, err)

	// Wait for alert processing
	time.Sleep(20 * time.Millisecond)

	// Verify alert IS triggered (average battery 15% is below threshold 25%)
	batteryAlert, err = hub.FindFirstRecordByFilter("alerts", "id={:id}", dbx.Params{"id": batteryAlert.Id})
	require.NoError(t, err)
	assert.True(t, batteryAlert.GetBool("triggered"),
		"Alert SHOULD be triggered when average battery (15%%) is below threshold (25%%) over min period")

	// Now add records with high battery to test resolution
	// Use a new time window 2 minutes later
	newNow := now.Add(2 * time.Minute)
	// Records need to span before the alert time window (newNow - 2 min)
	recordTimesHigh := []time.Duration{
		-180 * time.Second, // 3 min before newNow - makes oldest record before alert.time
		-90 * time.Second,
		-60 * time.Second,
		-30 * time.Second,
	}

	for _, offset := range recordTimesHigh {
		statsHigh := system.Stats{
			Cpu:     10,
			MemPct:  30,
			DiskPct: 40,
			Battery: [2]uint8{50, 1}, // 50% battery (above 25% threshold)
		}
		statsHighJSON, _ := json.Marshal(statsHigh)

		recordTime := newNow.Add(offset)
		record, err := beszelTests.CreateRecord(hub, "system_stats", map[string]any{
			"system": systemRecord.Id,
			"type":   "1m",
			"stats":  string(statsHighJSON),
		})
		require.NoError(t, err)
		record.SetRaw("created", recordTime.Format(types.DefaultDateLayout))
		err = hub.SaveNoValidate(record)
		require.NoError(t, err)
	}

	// Create combined data with high battery
	combinedDataHigh := &system.CombinedData{
		Stats: system.Stats{
			Cpu:     10,
			MemPct:  30,
			DiskPct: 40,
			Battery: [2]uint8{50, 1},
		},
		Info: system.Info{
			AgentVersion: "0.12.0",
			Cpu:          10,
			MemPct:       30,
			DiskPct:      40,
		},
	}

	// Update system timestamp to the new time window
	systemRecord.Set("updated", newNow)
	err = hub.SaveNoValidate(systemRecord)
	require.NoError(t, err)

	// Handle system alerts - should resolve because average battery is now above threshold
	err = am.HandleSystemAlerts(systemRecord, combinedDataHigh)
	require.NoError(t, err)

	// Wait for alert processing
	time.Sleep(20 * time.Millisecond)

	// Verify alert is resolved (average battery 50% is above threshold 25%)
	batteryAlert, err = hub.FindFirstRecordByFilter("alerts", "id={:id}", dbx.Params{"id": batteryAlert.Id})
	require.NoError(t, err)
	assert.False(t, batteryAlert.GetBool("triggered"),
		"Alert should be resolved when average battery (50%%) is above threshold (25%%) over min period")
}
