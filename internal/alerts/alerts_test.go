//go:build testing

package alerts_test

import (
	"testing"
	"testing/synctest"
	"time"

	beszelTests "github.com/henrygd/beszel/internal/tests"

	"github.com/henrygd/beszel/internal/alerts"
	"github.com/pocketbase/dbx"
	"github.com/stretchr/testify/assert"
)

func TestAlertsHistory(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hub, user := beszelTests.GetHubWithUser(t)
		defer hub.Cleanup()

		// Create systems and alerts
		systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
		assert.NoError(t, err)
		system := systems[0]

		alert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
			"name":   "Status",
			"system": system.Id,
			"user":   user.Id,
			"min":    1,
		})
		assert.NoError(t, err)

		// Initially, no alert history records should exist
		initialHistoryCount, err := hub.CountRecords("alerts_history", nil)
		assert.NoError(t, err)
		assert.Zero(t, initialHistoryCount, "Should have 0 alert history records initially")

		// Set system to up initially
		system.Set("status", "up")
		err = hub.SaveNoValidate(system)
		assert.NoError(t, err)
		time.Sleep(10 * time.Millisecond)

		// Set system to down to trigger alert
		system.Set("status", "down")
		err = hub.SaveNoValidate(system)
		assert.NoError(t, err)

		// Wait for alert to trigger (after the downtime delay)
		// With 1 minute delay, we need to wait at least 1 minute + some buffer
		time.Sleep(time.Second * 75)

		// Check that alert is triggered
		triggeredCount, err := hub.CountRecords("alerts", dbx.HashExp{"triggered": true, "id": alert.Id})
		assert.NoError(t, err)
		assert.EqualValues(t, 1, triggeredCount, "Alert should be triggered")

		// Check that alert history record was created
		historyCount, err := hub.CountRecords("alerts_history", dbx.HashExp{"alert_id": alert.Id})
		assert.NoError(t, err)
		assert.EqualValues(t, 1, historyCount, "Should have 1 alert history record for triggered alert")

		// Get the alert history record and verify it's not resolved immediately
		historyRecord, err := hub.FindFirstRecordByFilter("alerts_history", "alert_id={:alert_id}", dbx.Params{"alert_id": alert.Id})
		assert.NoError(t, err)
		assert.NotNil(t, historyRecord, "Alert history record should exist")
		assert.Equal(t, alert.Id, historyRecord.GetString("alert_id"), "Alert history should reference correct alert")
		assert.Equal(t, system.Id, historyRecord.GetString("system"), "Alert history should reference correct system")
		assert.Equal(t, "Status", historyRecord.GetString("name"), "Alert history should have correct name")

		// The alert history might be resolved immediately in some cases, so let's check the alert's triggered status
		alertRecord, err := hub.FindFirstRecordByFilter("alerts", "id={:id}", dbx.Params{"id": alert.Id})
		assert.NoError(t, err)
		assert.True(t, alertRecord.GetBool("triggered"), "Alert should still be triggered when checking history")

		// Now resolve the alert by setting system back to up
		system.Set("status", "up")
		err = hub.SaveNoValidate(system)
		assert.NoError(t, err)
		time.Sleep(200 * time.Millisecond)

		// Check that alert is no longer triggered
		triggeredCount, err = hub.CountRecords("alerts", dbx.HashExp{"triggered": true, "id": alert.Id})
		assert.NoError(t, err)
		assert.Zero(t, triggeredCount, "Alert should not be triggered after system is back up")

		// Check that alert history record is now resolved
		historyRecord, err = hub.FindFirstRecordByFilter("alerts_history", "alert_id={:alert_id}", dbx.Params{"alert_id": alert.Id})
		assert.NoError(t, err)
		assert.NotNil(t, historyRecord, "Alert history record should still exist")
		assert.NotNil(t, historyRecord.Get("resolved"), "Alert history should be resolved")

		// Test deleting a triggered alert resolves its history
		// Create another system and alert
		systems2, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
		assert.NoError(t, err)
		system2 := systems2[0]
		system2.Set("name", "test-system-2") // Rename for clarity
		err = hub.SaveNoValidate(system2)
		assert.NoError(t, err)

		alert2, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
			"name":   "Status",
			"system": system2.Id,
			"user":   user.Id,
			"min":    1,
		})
		assert.NoError(t, err)

		// Set system2 to down to trigger alert
		system2.Set("status", "down")
		err = hub.SaveNoValidate(system2)
		assert.NoError(t, err)

		// Wait for alert to trigger
		time.Sleep(time.Second * 75)

		// Verify alert is triggered and history record exists
		triggeredCount, err = hub.CountRecords("alerts", dbx.HashExp{"triggered": true, "id": alert2.Id})
		assert.NoError(t, err)
		assert.EqualValues(t, 1, triggeredCount, "Second alert should be triggered")

		historyCount, err = hub.CountRecords("alerts_history", dbx.HashExp{"alert_id": alert2.Id})
		assert.NoError(t, err)
		assert.EqualValues(t, 1, historyCount, "Should have 1 alert history record for second alert")

		// Delete the triggered alert
		err = hub.Delete(alert2)
		assert.NoError(t, err)

		// Check that alert history record is resolved after deletion
		historyRecord2, err := hub.FindFirstRecordByFilter("alerts_history", "alert_id={:alert_id}", dbx.Params{"alert_id": alert2.Id})
		assert.NoError(t, err)
		assert.NotNil(t, historyRecord2, "Alert history record should still exist after alert deletion")
		assert.NotNil(t, historyRecord2.Get("resolved"), "Alert history should be resolved after alert deletion")

		// Verify total history count is correct (2 records total)
		totalHistoryCount, err := hub.CountRecords("alerts_history", nil)
		assert.NoError(t, err)
		assert.EqualValues(t, 2, totalHistoryCount, "Should have 2 total alert history records")
	})
}

func TestSetAlertTriggered(t *testing.T) {
	hub, _ := beszelTests.NewTestHub(t.TempDir())
	defer hub.Cleanup()

	hub.StartHub()

	user, _ := beszelTests.CreateUser(hub, "test@example.com", "password")
	system, _ := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"users": []string{user.Id},
		"host":  "127.0.0.1",
	})

	alertRecord, _ := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":      "CPU",
		"system":    system.Id,
		"user":      user.Id,
		"value":     80,
		"triggered": false,
	})

	am := alerts.NewAlertManager(hub)

	var alert alerts.CachedAlertData
	alert.PopulateFromRecord(alertRecord)

	// Test triggering the alert
	err := am.SetAlertTriggered(alert, true)
	assert.NoError(t, err)

	updatedRecord, err := hub.FindRecordById("alerts", alert.Id)
	assert.NoError(t, err)
	assert.True(t, updatedRecord.GetBool("triggered"))

	// Test un-triggering the alert
	err = am.SetAlertTriggered(alert, false)
	assert.NoError(t, err)

	updatedRecord, err = hub.FindRecordById("alerts", alert.Id)
	assert.NoError(t, err)
	assert.False(t, updatedRecord.GetBool("triggered"))
}
