//go:build testing

package alerts_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/henrygd/beszel/internal/alerts"
	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusAlerts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hub, user := beszelTests.GetHubWithUser(t)
		defer hub.Cleanup()

		systems, err := beszelTests.CreateSystems(hub, 4, user.Id, "paused")
		assert.NoError(t, err)

		var alerts []*core.Record
		for i, system := range systems {
			alert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
				"name":   "Status",
				"system": system.Id,
				"user":   user.Id,
				"min":    i + 1,
			})
			assert.NoError(t, err)
			alerts = append(alerts, alert)
		}

		time.Sleep(10 * time.Millisecond)

		for _, alert := range alerts {
			assert.False(t, alert.GetBool("triggered"), "Alert should not be triggered immediately")
		}
		if hub.TestMailer.TotalSend() != 0 {
			assert.Zero(t, hub.TestMailer.TotalSend(), "Expected 0 messages, got %d", hub.TestMailer.TotalSend())
		}
		for _, system := range systems {
			assert.EqualValues(t, "paused", system.GetString("status"), "System should be paused")
		}
		for _, system := range systems {
			system.Set("status", "up")
			err = hub.SaveNoValidate(system)
			assert.NoError(t, err)
		}
		time.Sleep(time.Second)
		assert.EqualValues(t, 0, hub.GetPendingAlertsCount(), "should have 0 alerts in the pendingAlerts map")
		for _, system := range systems {
			system.Set("status", "down")
			err = hub.SaveNoValidate(system)
			assert.NoError(t, err)
		}
		// after 30 seconds, should have 4 alerts in the pendingAlerts map, no triggered alerts
		time.Sleep(time.Second * 30)
		assert.EqualValues(t, 4, hub.GetPendingAlertsCount(), "should have 4 alerts in the pendingAlerts map")
		triggeredCount, err := hub.CountRecords("alerts", dbx.HashExp{"triggered": true})
		assert.NoError(t, err)
		assert.EqualValues(t, 0, triggeredCount, "should have 0 alert triggered")
		assert.EqualValues(t, 0, hub.TestMailer.TotalSend(), "should have 0 messages sent")
		// after 1:30 seconds, should have 1 triggered alert and 3 pending alerts
		time.Sleep(time.Second * 60)
		assert.EqualValues(t, 3, hub.GetPendingAlertsCount(), "should have 3 alerts in the pendingAlerts map")
		triggeredCount, err = hub.CountRecords("alerts", dbx.HashExp{"triggered": true})
		assert.NoError(t, err)
		assert.EqualValues(t, 1, triggeredCount, "should have 1 alert triggered")
		assert.EqualValues(t, 1, hub.TestMailer.TotalSend(), "should have 1 messages sent")
		// after 2:30 seconds, should have 2 triggered alerts and 2 pending alerts
		time.Sleep(time.Second * 60)
		assert.EqualValues(t, 2, hub.GetPendingAlertsCount(), "should have 2 alerts in the pendingAlerts map")
		triggeredCount, err = hub.CountRecords("alerts", dbx.HashExp{"triggered": true})
		assert.NoError(t, err)
		assert.EqualValues(t, 2, triggeredCount, "should have 2 alert triggered")
		assert.EqualValues(t, 2, hub.TestMailer.TotalSend(), "should have 2 messages sent")
		// now we will bring the remaning systems back up
		for _, system := range systems {
			system.Set("status", "up")
			err = hub.SaveNoValidate(system)
			assert.NoError(t, err)
		}
		time.Sleep(time.Second)
		// should have 0 alerts in the pendingAlerts map and 0 alerts triggered
		assert.EqualValues(t, 0, hub.GetPendingAlertsCount(), "should have 0 alerts in the pendingAlerts map")
		triggeredCount, err = hub.CountRecords("alerts", dbx.HashExp{"triggered": true})
		assert.NoError(t, err)
		assert.Zero(t, triggeredCount, "should have 0 alert triggered")
		// 4 messages sent, 2 down alerts and 2 up alerts for first 2 systems
		assert.EqualValues(t, 4, hub.TestMailer.TotalSend(), "should have 4 messages sent")
	})
}
func TestStatusAlertRecoveryBeforeDeadline(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	// Ensure user settings have an email
	userSettings, _ := hub.FindFirstRecordByFilter("user_settings", "user={:user}", map[string]any{"user": user.Id})
	userSettings.Set("settings", `{"emails":["test@example.com"],"webhooks":[]}`)
	hub.Save(userSettings)

	// Initial email count
	initialEmailCount := hub.TestMailer.TotalSend()

	systemCollection, _ := hub.FindCollectionByNameOrId("systems")
	system := core.NewRecord(systemCollection)
	system.Set("name", "test-system")
	system.Set("status", "up")
	system.Set("host", "127.0.0.1")
	system.Set("users", []string{user.Id})
	hub.Save(system)

	alertCollection, _ := hub.FindCollectionByNameOrId("alerts")
	alert := core.NewRecord(alertCollection)
	alert.Set("user", user.Id)
	alert.Set("system", system.Id)
	alert.Set("name", "Status")
	alert.Set("triggered", false)
	alert.Set("min", 1)
	hub.Save(alert)

	am := hub.AlertManager

	// 1. System goes down
	am.HandleStatusAlerts("down", system)
	assert.Equal(t, 1, am.GetPendingAlertsCount(), "Alert should be scheduled")

	// 2. System goes up BEFORE delay expires
	// Triggering HandleStatusAlerts("up") SHOULD NOT send an alert.
	am.HandleStatusAlerts("up", system)

	assert.Equal(t, 0, am.GetPendingAlertsCount(), "Alert should be canceled if system recovers before delay expires")

	// Verify that NO email was sent.
	assert.Equal(t, initialEmailCount, hub.TestMailer.TotalSend(), "Recovery notification should not be sent if system never went down")

}

func TestStatusAlertNormalRecovery(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	// Ensure user settings have an email
	userSettings, _ := hub.FindFirstRecordByFilter("user_settings", "user={:user}", map[string]any{"user": user.Id})
	userSettings.Set("settings", `{"emails":["test@example.com"],"webhooks":[]}`)
	hub.Save(userSettings)

	systemCollection, _ := hub.FindCollectionByNameOrId("systems")
	system := core.NewRecord(systemCollection)
	system.Set("name", "test-system")
	system.Set("status", "up")
	system.Set("host", "127.0.0.1")
	system.Set("users", []string{user.Id})
	hub.Save(system)

	alertCollection, _ := hub.FindCollectionByNameOrId("alerts")
	alert := core.NewRecord(alertCollection)
	alert.Set("user", user.Id)
	alert.Set("system", system.Id)
	alert.Set("name", "Status")
	alert.Set("triggered", true) // System was confirmed DOWN
	hub.Save(alert)

	am := hub.AlertManager
	initialEmailCount := hub.TestMailer.TotalSend()

	// System goes up
	am.HandleStatusAlerts("up", system)

	// Verify that an email WAS sent (normal recovery).
	assert.Equal(t, initialEmailCount+1, hub.TestMailer.TotalSend(), "Recovery notification should be sent if system was triggered as down")

}

func TestHandleStatusAlertsDoesNotSendRecoveryWhileDownIsOnlyPending(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	userSettings, err := hub.FindFirstRecordByFilter("user_settings", "user={:user}", map[string]any{"user": user.Id})
	require.NoError(t, err)
	userSettings.Set("settings", `{"emails":["test@example.com"],"webhooks":[]}`)
	require.NoError(t, hub.Save(userSettings))

	systemCollection, err := hub.FindCollectionByNameOrId("systems")
	require.NoError(t, err)
	system := core.NewRecord(systemCollection)
	system.Set("name", "test-system")
	system.Set("status", "up")
	system.Set("host", "127.0.0.1")
	system.Set("users", []string{user.Id})
	require.NoError(t, hub.Save(system))

	alertCollection, err := hub.FindCollectionByNameOrId("alerts")
	require.NoError(t, err)
	alert := core.NewRecord(alertCollection)
	alert.Set("user", user.Id)
	alert.Set("system", system.Id)
	alert.Set("name", "Status")
	alert.Set("triggered", false)
	alert.Set("min", 1)
	require.NoError(t, hub.Save(alert))

	initialEmailCount := hub.TestMailer.TotalSend()
	am := alerts.NewTestAlertManagerWithoutWorker(hub)

	require.NoError(t, am.HandleStatusAlerts("down", system))
	assert.Equal(t, 1, am.GetPendingAlertsCount(), "down transition should register a pending alert immediately")

	require.NoError(t, am.HandleStatusAlerts("up", system))
	assert.Zero(t, am.GetPendingAlertsCount(), "recovery should cancel the pending down alert")
	assert.Equal(t, initialEmailCount, hub.TestMailer.TotalSend(), "recovery notification should not be sent before a down alert triggers")

	alertRecord, err := hub.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	assert.False(t, alertRecord.GetBool("triggered"), "alert should remain untriggered when downtime never matured")
}

func TestStatusAlertTimerCancellationPreventsBoundaryDelivery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hub, user := beszelTests.GetHubWithUser(t)
		defer hub.Cleanup()

		userSettings, err := hub.FindFirstRecordByFilter("user_settings", "user={:user}", map[string]any{"user": user.Id})
		require.NoError(t, err)
		userSettings.Set("settings", `{"emails":["test@example.com"],"webhooks":[]}`)
		require.NoError(t, hub.Save(userSettings))

		systemCollection, err := hub.FindCollectionByNameOrId("systems")
		require.NoError(t, err)
		system := core.NewRecord(systemCollection)
		system.Set("name", "test-system")
		system.Set("status", "up")
		system.Set("host", "127.0.0.1")
		system.Set("users", []string{user.Id})
		require.NoError(t, hub.Save(system))

		alertCollection, err := hub.FindCollectionByNameOrId("alerts")
		require.NoError(t, err)
		alert := core.NewRecord(alertCollection)
		alert.Set("user", user.Id)
		alert.Set("system", system.Id)
		alert.Set("name", "Status")
		alert.Set("triggered", false)
		alert.Set("min", 1)
		require.NoError(t, hub.Save(alert))

		initialEmailCount := hub.TestMailer.TotalSend()
		am := alerts.NewTestAlertManagerWithoutWorker(hub)

		require.NoError(t, am.HandleStatusAlerts("down", system))
		assert.Equal(t, 1, am.GetPendingAlertsCount(), "down transition should register a pending alert immediately")
		require.True(t, am.ResetPendingAlertTimer(alert.Id, 25*time.Millisecond), "test should shorten the pending alert timer")

		time.Sleep(10 * time.Millisecond)
		require.NoError(t, am.HandleStatusAlerts("up", system))
		assert.Zero(t, am.GetPendingAlertsCount(), "recovery should remove the pending alert before the timer callback runs")

		time.Sleep(40 * time.Millisecond)
		assert.Equal(t, initialEmailCount, hub.TestMailer.TotalSend(), "timer callback should not deliver after recovery cancels the pending alert")

		alertRecord, err := hub.FindRecordById("alerts", alert.Id)
		require.NoError(t, err)
		assert.False(t, alertRecord.GetBool("triggered"), "alert should remain untriggered when cancellation wins the timer race")

		time.Sleep(time.Minute)
		synctest.Wait()
	})
}

func TestStatusAlertDownFiresAfterDelayExpires(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	userSettings, err := hub.FindFirstRecordByFilter("user_settings", "user={:user}", map[string]any{"user": user.Id})
	require.NoError(t, err)
	userSettings.Set("settings", `{"emails":["test@example.com"],"webhooks":[]}`)
	require.NoError(t, hub.Save(userSettings))

	systemCollection, err := hub.FindCollectionByNameOrId("systems")
	require.NoError(t, err)
	system := core.NewRecord(systemCollection)
	system.Set("name", "test-system")
	system.Set("status", "up")
	system.Set("host", "127.0.0.1")
	system.Set("users", []string{user.Id})
	require.NoError(t, hub.Save(system))

	alertCollection, err := hub.FindCollectionByNameOrId("alerts")
	require.NoError(t, err)
	alert := core.NewRecord(alertCollection)
	alert.Set("user", user.Id)
	alert.Set("system", system.Id)
	alert.Set("name", "Status")
	alert.Set("triggered", false)
	alert.Set("min", 1)
	require.NoError(t, hub.Save(alert))

	initialEmailCount := hub.TestMailer.TotalSend()
	am := alerts.NewTestAlertManagerWithoutWorker(hub)

	require.NoError(t, am.HandleStatusAlerts("down", system))
	assert.Equal(t, 1, am.GetPendingAlertsCount(), "alert should be pending after system goes down")

	// Expire the pending alert and process it
	am.ForceExpirePendingAlerts()
	processed, err := am.ProcessPendingAlerts()
	require.NoError(t, err)
	assert.Len(t, processed, 1, "one alert should have been processed")
	assert.Equal(t, 0, am.GetPendingAlertsCount(), "pending alert should be consumed after processing")

	// Verify down email was sent
	assert.Equal(t, initialEmailCount+1, hub.TestMailer.TotalSend(), "down notification should be sent after delay expires")

	// Verify triggered flag is set in the DB
	alertRecord, err := hub.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	assert.True(t, alertRecord.GetBool("triggered"), "alert should be marked triggered after downtime matures")
}

func TestStatusAlertDuplicateDownCallIsIdempotent(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	userSettings, err := hub.FindFirstRecordByFilter("user_settings", "user={:user}", map[string]any{"user": user.Id})
	require.NoError(t, err)
	userSettings.Set("settings", `{"emails":["test@example.com"],"webhooks":[]}`)
	require.NoError(t, hub.Save(userSettings))

	systemCollection, err := hub.FindCollectionByNameOrId("systems")
	require.NoError(t, err)
	system := core.NewRecord(systemCollection)
	system.Set("name", "test-system")
	system.Set("status", "up")
	system.Set("host", "127.0.0.1")
	system.Set("users", []string{user.Id})
	require.NoError(t, hub.Save(system))

	alertCollection, err := hub.FindCollectionByNameOrId("alerts")
	require.NoError(t, err)
	alert := core.NewRecord(alertCollection)
	alert.Set("user", user.Id)
	alert.Set("system", system.Id)
	alert.Set("name", "Status")
	alert.Set("triggered", false)
	alert.Set("min", 5)
	require.NoError(t, hub.Save(alert))

	am := alerts.NewTestAlertManagerWithoutWorker(hub)

	require.NoError(t, am.HandleStatusAlerts("down", system))
	require.NoError(t, am.HandleStatusAlerts("down", system))
	require.NoError(t, am.HandleStatusAlerts("down", system))

	assert.Equal(t, 1, am.GetPendingAlertsCount(), "repeated down calls should not schedule duplicate pending alerts")
}

func TestStatusAlertNoAlertRecord(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	systemCollection, err := hub.FindCollectionByNameOrId("systems")
	require.NoError(t, err)
	system := core.NewRecord(systemCollection)
	system.Set("name", "test-system")
	system.Set("status", "up")
	system.Set("host", "127.0.0.1")
	system.Set("users", []string{user.Id})
	require.NoError(t, hub.Save(system))

	// No Status alert record created for this system
	initialEmailCount := hub.TestMailer.TotalSend()
	am := alerts.NewTestAlertManagerWithoutWorker(hub)

	require.NoError(t, am.HandleStatusAlerts("down", system))
	assert.Equal(t, 0, am.GetPendingAlertsCount(), "no pending alert when no alert record exists")

	require.NoError(t, am.HandleStatusAlerts("up", system))
	assert.Equal(t, initialEmailCount, hub.TestMailer.TotalSend(), "no email when no alert record exists")
}

func TestRestorePendingStatusAlertsRequeuesDownSystemsAfterRestart(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	userSettings, err := hub.FindFirstRecordByFilter("user_settings", "user={:user}", map[string]any{"user": user.Id})
	require.NoError(t, err)
	userSettings.Set("settings", `{"emails":["test@example.com"],"webhooks":[]}`)
	require.NoError(t, hub.Save(userSettings))

	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "down")
	require.NoError(t, err)
	system := systems[0]

	alertCollection, err := hub.FindCollectionByNameOrId("alerts")
	require.NoError(t, err)
	alert := core.NewRecord(alertCollection)
	alert.Set("user", user.Id)
	alert.Set("system", system.Id)
	alert.Set("name", "Status")
	alert.Set("triggered", false)
	alert.Set("min", 1)
	require.NoError(t, hub.Save(alert))

	initialEmailCount := hub.TestMailer.TotalSend()
	am := alerts.NewTestAlertManagerWithoutWorker(hub)

	require.NoError(t, am.RestorePendingStatusAlerts())
	assert.Equal(t, 1, am.GetPendingAlertsCount(), "startup restore should requeue a pending down alert for a system still marked down")

	am.ForceExpirePendingAlerts()
	processed, err := am.ProcessPendingAlerts()
	require.NoError(t, err)
	assert.Len(t, processed, 1, "restored pending alert should be processable after the delay expires")
	assert.Equal(t, initialEmailCount+1, hub.TestMailer.TotalSend(), "restored pending alert should send the down notification")

	alertRecord, err := hub.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	assert.True(t, alertRecord.GetBool("triggered"), "restored pending alert should mark the alert as triggered once delivered")
}

func TestRestorePendingStatusAlertsSkipsNonDownOrAlreadyTriggeredAlerts(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	systemsDown, err := beszelTests.CreateSystems(hub, 2, user.Id, "down")
	require.NoError(t, err)
	systemDownPending := systemsDown[0]
	systemDownTriggered := systemsDown[1]

	systemUp, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":   "up-system",
		"users":  []string{user.Id},
		"host":   "127.0.0.2",
		"status": "up",
	})
	require.NoError(t, err)

	_, err = beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":      "Status",
		"system":    systemDownPending.Id,
		"user":      user.Id,
		"min":       1,
		"triggered": false,
	})
	require.NoError(t, err)

	_, err = beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":      "Status",
		"system":    systemUp.Id,
		"user":      user.Id,
		"min":       1,
		"triggered": false,
	})
	require.NoError(t, err)

	_, err = beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":      "Status",
		"system":    systemDownTriggered.Id,
		"user":      user.Id,
		"min":       1,
		"triggered": true,
	})
	require.NoError(t, err)

	am := alerts.NewTestAlertManagerWithoutWorker(hub)
	require.NoError(t, am.RestorePendingStatusAlerts())
	assert.Equal(t, 1, am.GetPendingAlertsCount(), "only untriggered alerts for currently down systems should be restored")
}

func TestRestorePendingStatusAlertsIsIdempotent(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "down")
	require.NoError(t, err)
	system := systems[0]

	_, err = beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":      "Status",
		"system":    system.Id,
		"user":      user.Id,
		"min":       1,
		"triggered": false,
	})
	require.NoError(t, err)

	am := alerts.NewTestAlertManagerWithoutWorker(hub)
	require.NoError(t, am.RestorePendingStatusAlerts())
	require.NoError(t, am.RestorePendingStatusAlerts())

	assert.Equal(t, 1, am.GetPendingAlertsCount(), "restoring twice should not create duplicate pending alerts")
	am.ForceExpirePendingAlerts()
	processed, err := am.ProcessPendingAlerts()
	require.NoError(t, err)
	assert.Len(t, processed, 1, "restored alert should still be processable exactly once")
	assert.Zero(t, am.GetPendingAlertsCount(), "processing the restored alert should empty the pending map")
}

func TestResolveStatusAlertsFixesStaleTriggered(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	// CreateSystems uses SaveNoValidate after initial save to bypass the
	// onRecordCreate hook that forces status = "pending".
	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
	require.NoError(t, err)
	system := systems[0]

	alertCollection, err := hub.FindCollectionByNameOrId("alerts")
	require.NoError(t, err)
	alert := core.NewRecord(alertCollection)
	alert.Set("user", user.Id)
	alert.Set("system", system.Id)
	alert.Set("name", "Status")
	alert.Set("triggered", true) // Stale: system is up but alert still says triggered
	require.NoError(t, hub.Save(alert))

	// resolveStatusAlerts should clear the stale triggered flag
	require.NoError(t, alerts.ResolveStatusAlerts(hub))

	alertRecord, err := hub.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	assert.False(t, alertRecord.GetBool("triggered"), "stale triggered flag should be cleared when system is up")
}
func TestResolveStatusAlerts(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	// Create a systemUp
	systemUp, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":   "test-system",
		"users":  []string{user.Id},
		"host":   "127.0.0.1",
		"status": "up",
	})
	assert.NoError(t, err)

	systemDown, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":   "test-system-2",
		"users":  []string{user.Id},
		"host":   "127.0.0.2",
		"status": "up",
	})
	assert.NoError(t, err)

	// Create a status alertUp for the system
	alertUp, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":   "Status",
		"system": systemUp.Id,
		"user":   user.Id,
		"min":    1,
	})
	assert.NoError(t, err)

	alertDown, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":   "Status",
		"system": systemDown.Id,
		"user":   user.Id,
		"min":    1,
	})
	assert.NoError(t, err)

	// Verify alert is not triggered initially
	assert.False(t, alertUp.GetBool("triggered"), "Alert should not be triggered initially")

	// Set the system to 'up' (this should not trigger the alert)
	systemUp.Set("status", "up")
	err = hub.SaveNoValidate(systemUp)
	assert.NoError(t, err)

	systemDown.Set("status", "down")
	err = hub.SaveNoValidate(systemDown)
	assert.NoError(t, err)

	// Wait a moment for any processing
	time.Sleep(10 * time.Millisecond)

	// Verify alertUp is still not triggered after setting system to up
	alertUp, err = hub.FindFirstRecordByFilter("alerts", "id={:id}", dbx.Params{"id": alertUp.Id})
	assert.NoError(t, err)
	assert.False(t, alertUp.GetBool("triggered"), "Alert should not be triggered when system is up")

	// Manually set both alerts triggered to true
	alertUp.Set("triggered", true)
	err = hub.SaveNoValidate(alertUp)
	assert.NoError(t, err)
	alertDown.Set("triggered", true)
	err = hub.SaveNoValidate(alertDown)
	assert.NoError(t, err)

	// Verify we have exactly one alert with triggered true
	triggeredCount, err := hub.CountRecords("alerts", dbx.HashExp{"triggered": true})
	assert.NoError(t, err)
	assert.EqualValues(t, 2, triggeredCount, "Should have exactly two alerts with triggered true")

	// Verify the specific alertUp is triggered
	alertUp, err = hub.FindFirstRecordByFilter("alerts", "id={:id}", dbx.Params{"id": alertUp.Id})
	assert.NoError(t, err)
	assert.True(t, alertUp.GetBool("triggered"), "Alert should be triggered")

	// Verify we have two unresolved alert history records
	alertHistoryCount, err := hub.CountRecords("alerts_history", dbx.HashExp{"resolved": ""})
	assert.NoError(t, err)
	assert.EqualValues(t, 2, alertHistoryCount, "Should have exactly two unresolved alert history records")

	err = alerts.ResolveStatusAlerts(hub)
	assert.NoError(t, err)

	// Verify alertUp is not triggered after resolving
	alertUp, err = hub.FindFirstRecordByFilter("alerts", "id={:id}", dbx.Params{"id": alertUp.Id})
	assert.NoError(t, err)
	assert.False(t, alertUp.GetBool("triggered"), "Alert should not be triggered after resolving")
	// Verify alertDown is still triggered
	alertDown, err = hub.FindFirstRecordByFilter("alerts", "id={:id}", dbx.Params{"id": alertDown.Id})
	assert.NoError(t, err)
	assert.True(t, alertDown.GetBool("triggered"), "Alert should still be triggered after resolving")

	// Verify we have one unresolved alert history record
	alertHistoryCount, err = hub.CountRecords("alerts_history", dbx.HashExp{"resolved": ""})
	assert.NoError(t, err)
	assert.EqualValues(t, 1, alertHistoryCount, "Should have exactly one unresolved alert history record")

}
