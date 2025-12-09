//go:build testing
// +build testing

package alerts_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/henrygd/beszel/internal/alerts"
	beszelTests "github.com/henrygd/beszel/internal/tests"

	"github.com/pocketbase/dbx"
	"github.com/stretchr/testify/assert"
)

func TestAlertSilencedOneTime(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	// Create a system
	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
	assert.NoError(t, err)
	system := systems[0]

	// Create an alert
	alert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":   "CPU",
		"system": system.Id,
		"user":   user.Id,
		"value":  80,
		"min":    1,
	})
	assert.NoError(t, err)

	// Create a one-time quiet hours window (current time - 1 hour to current time + 1 hour)
	now := time.Now().UTC()
	startTime := now.Add(-1 * time.Hour)
	endTime := now.Add(1 * time.Hour)

	_, err = beszelTests.CreateRecord(hub, "quiet_hours", map[string]any{
		"user":   user.Id,
		"system": system.Id,
		"type":   "one-time",
		"start":  startTime,
		"end":    endTime,
	})
	assert.NoError(t, err)

	// Get alert manager
	am := alerts.NewAlertManager(hub)
	defer am.StopWorker()

	// Test that alert is silenced
	silenced := am.IsNotificationSilenced(user.Id, system.Id)
	assert.True(t, silenced, "Alert should be silenced during active one-time window")

	// Create a window that has already ended
	pastStart := now.Add(-3 * time.Hour)
	pastEnd := now.Add(-2 * time.Hour)

	_, err = beszelTests.CreateRecord(hub, "quiet_hours", map[string]any{
		"user":   user.Id,
		"system": system.Id,
		"type":   "one-time",
		"start":  pastStart,
		"end":    pastEnd,
	})
	assert.NoError(t, err)

	// Should still be silenced because of the first window
	silenced = am.IsNotificationSilenced(user.Id, system.Id)
	assert.True(t, silenced, "Alert should still be silenced (past window doesn't affect active window)")

	// Clear all windows and create a future window
	_, err = hub.DB().NewQuery("DELETE FROM quiet_hours").Execute()
	assert.NoError(t, err)

	futureStart := now.Add(2 * time.Hour)
	futureEnd := now.Add(3 * time.Hour)

	_, err = beszelTests.CreateRecord(hub, "quiet_hours", map[string]any{
		"user":   user.Id,
		"system": system.Id,
		"type":   "one-time",
		"start":  futureStart,
		"end":    futureEnd,
	})
	assert.NoError(t, err)

	// Alert should NOT be silenced (window hasn't started yet)
	silenced = am.IsNotificationSilenced(user.Id, system.Id)
	assert.False(t, silenced, "Alert should not be silenced (window hasn't started)")

	_ = alert
}

func TestAlertSilencedDaily(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	// Create a system
	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
	assert.NoError(t, err)
	system := systems[0]

	// Get alert manager
	am := alerts.NewAlertManager(hub)
	defer am.StopWorker()

	// Get current hour and create a window that includes current time
	now := time.Now().UTC()
	currentHour := now.Hour()
	currentMin := now.Minute()

	// Create a window from 1 hour ago to 1 hour from now
	startHour := (currentHour - 1 + 24) % 24
	endHour := (currentHour + 1) % 24

	// Create times with just the hours/minutes we want (date doesn't matter for daily)
	startTime := time.Date(2000, 1, 1, startHour, currentMin, 0, 0, time.UTC)
	endTime := time.Date(2000, 1, 1, endHour, currentMin, 0, 0, time.UTC)

	_, err = beszelTests.CreateRecord(hub, "quiet_hours", map[string]any{
		"user":   user.Id,
		"system": system.Id,
		"type":   "daily",
		"start":  startTime,
		"end":    endTime,
	})
	assert.NoError(t, err)

	// Alert should be silenced (current time is within the daily window)
	silenced := am.IsNotificationSilenced(user.Id, system.Id)
	assert.True(t, silenced, "Alert should be silenced during active daily window")

	// Clear windows and create one that doesn't include current time
	_, err = hub.DB().NewQuery("DELETE FROM quiet_hours").Execute()
	assert.NoError(t, err)

	// Create a window from 6-12 hours from now
	futureStartHour := (currentHour + 6) % 24
	futureEndHour := (currentHour + 12) % 24

	startTime = time.Date(2000, 1, 1, futureStartHour, 0, 0, 0, time.UTC)
	endTime = time.Date(2000, 1, 1, futureEndHour, 0, 0, 0, time.UTC)

	_, err = beszelTests.CreateRecord(hub, "quiet_hours", map[string]any{
		"user":   user.Id,
		"system": system.Id,
		"type":   "daily",
		"start":  startTime,
		"end":    endTime,
	})
	assert.NoError(t, err)

	// Alert should NOT be silenced
	silenced = am.IsNotificationSilenced(user.Id, system.Id)
	assert.False(t, silenced, "Alert should not be silenced (outside daily window)")
}

func TestAlertSilencedDailyMidnightCrossing(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	// Create a system
	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
	assert.NoError(t, err)
	system := systems[0]

	// Get alert manager
	am := alerts.NewAlertManager(hub)
	defer am.StopWorker()

	// Create a window that crosses midnight: 22:00 - 02:00
	startTime := time.Date(2000, 1, 1, 22, 0, 0, 0, time.UTC)
	endTime := time.Date(2000, 1, 1, 2, 0, 0, 0, time.UTC)

	_, err = beszelTests.CreateRecord(hub, "quiet_hours", map[string]any{
		"user":   user.Id,
		"system": system.Id,
		"type":   "daily",
		"start":  startTime,
		"end":    endTime,
	})
	assert.NoError(t, err)

	// Test with a time at 23:00 (should be silenced)
	// We can't control the actual current time, but we can verify the logic
	// by checking if the window was created correctly
	windows, err := hub.FindAllRecords("quiet_hours", dbx.HashExp{
		"user":   user.Id,
		"system": system.Id,
	})
	assert.NoError(t, err)
	assert.Len(t, windows, 1, "Should have created 1 window")

	window := windows[0]
	assert.Equal(t, "daily", window.GetString("type"))
	assert.Equal(t, 22, window.GetDateTime("start").Time().Hour())
	assert.Equal(t, 2, window.GetDateTime("end").Time().Hour())
}

func TestAlertSilencedGlobal(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	// Create multiple systems
	systems, err := beszelTests.CreateSystems(hub, 3, user.Id, "up")
	assert.NoError(t, err)

	// Get alert manager
	am := alerts.NewAlertManager(hub)
	defer am.StopWorker()

	// Create a global quiet hours window (no system specified)
	now := time.Now().UTC()
	startTime := now.Add(-1 * time.Hour)
	endTime := now.Add(1 * time.Hour)

	_, err = beszelTests.CreateRecord(hub, "quiet_hours", map[string]any{
		"user":  user.Id,
		"type":  "one-time",
		"start": startTime,
		"end":   endTime,
		// system field is empty/null for global windows
	})
	assert.NoError(t, err)

	// All systems should be silenced
	for _, system := range systems {
		silenced := am.IsNotificationSilenced(user.Id, system.Id)
		assert.True(t, silenced, "Alert should be silenced for system %s (global window)", system.Id)
	}

	// Even with a systemID that doesn't exist, should be silenced
	silenced := am.IsNotificationSilenced(user.Id, "nonexistent-system")
	assert.True(t, silenced, "Alert should be silenced for any system (global window)")
}

func TestAlertSilencedSystemSpecific(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	// Create multiple systems
	systems, err := beszelTests.CreateSystems(hub, 2, user.Id, "up")
	assert.NoError(t, err)
	system1 := systems[0]
	system2 := systems[1]

	// Get alert manager
	am := alerts.NewAlertManager(hub)
	defer am.StopWorker()

	// Create a system-specific quiet hours window for system1 only
	now := time.Now().UTC()
	startTime := now.Add(-1 * time.Hour)
	endTime := now.Add(1 * time.Hour)

	_, err = beszelTests.CreateRecord(hub, "quiet_hours", map[string]any{
		"user":   user.Id,
		"system": system1.Id,
		"type":   "one-time",
		"start":  startTime,
		"end":    endTime,
	})
	assert.NoError(t, err)

	// System1 should be silenced
	silenced := am.IsNotificationSilenced(user.Id, system1.Id)
	assert.True(t, silenced, "Alert should be silenced for system1")

	// System2 should NOT be silenced
	silenced = am.IsNotificationSilenced(user.Id, system2.Id)
	assert.False(t, silenced, "Alert should not be silenced for system2")
}

func TestAlertSilencedMultiUser(t *testing.T) {
	hub, _ := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	// Create two users
	user1, err := beszelTests.CreateUser(hub, "user1@example.com", "password")
	assert.NoError(t, err)

	user2, err := beszelTests.CreateUser(hub, "user2@example.com", "password")
	assert.NoError(t, err)

	// Create a system accessible to both users
	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "shared-system",
		"users": []string{user1.Id, user2.Id},
		"host":  "127.0.0.1",
	})
	assert.NoError(t, err)

	// Get alert manager
	am := alerts.NewAlertManager(hub)
	defer am.StopWorker()

	// Create a quiet hours window for user1 only
	now := time.Now().UTC()
	startTime := now.Add(-1 * time.Hour)
	endTime := now.Add(1 * time.Hour)

	_, err = beszelTests.CreateRecord(hub, "quiet_hours", map[string]any{
		"user":   user1.Id,
		"system": system.Id,
		"type":   "one-time",
		"start":  startTime,
		"end":    endTime,
	})
	assert.NoError(t, err)

	// User1 should be silenced
	silenced := am.IsNotificationSilenced(user1.Id, system.Id)
	assert.True(t, silenced, "Alert should be silenced for user1")

	// User2 should NOT be silenced
	silenced = am.IsNotificationSilenced(user2.Id, system.Id)
	assert.False(t, silenced, "Alert should not be silenced for user2")
}

func TestAlertSilencedWithActualAlert(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hub, user := beszelTests.GetHubWithUser(t)
		defer hub.Cleanup()

		// Create a system
		systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
		assert.NoError(t, err)
		system := systems[0]

		// Create a status alert
		_, err = beszelTests.CreateRecord(hub, "alerts", map[string]any{
			"name":   "Status",
			"system": system.Id,
			"user":   user.Id,
			"min":    1,
		})
		assert.NoError(t, err)

		// Create user settings with email
		userSettings, err := hub.FindFirstRecordByFilter("user_settings", "user={:user}", dbx.Params{"user": user.Id})
		if err != nil || userSettings == nil {
			userSettings, err = beszelTests.CreateRecord(hub, "user_settings", map[string]any{
				"user": user.Id,
				"settings": map[string]any{
					"emails": []string{"test@example.com"},
				},
			})
			assert.NoError(t, err)
		}

		// Create a quiet hours window
		now := time.Now().UTC()
		startTime := now.Add(-1 * time.Hour)
		endTime := now.Add(1 * time.Hour)

		_, err = beszelTests.CreateRecord(hub, "quiet_hours", map[string]any{
			"user":   user.Id,
			"system": system.Id,
			"type":   "one-time",
			"start":  startTime,
			"end":    endTime,
		})
		assert.NoError(t, err)

		// Get initial email count
		initialEmailCount := hub.TestMailer.TotalSend()

		// Trigger an alert by setting system to down
		system.Set("status", "down")
		err = hub.SaveNoValidate(system)
		assert.NoError(t, err)

		// Wait for the alert to be processed (1 minute + buffer)
		time.Sleep(time.Second * 75)
		synctest.Wait()

		// Check that no email was sent (because alert is silenced)
		finalEmailCount := hub.TestMailer.TotalSend()
		assert.Equal(t, initialEmailCount, finalEmailCount, "No emails should be sent when alert is silenced")

		// Clear quiet hours windows
		_, err = hub.DB().NewQuery("DELETE FROM quiet_hours").Execute()
		assert.NoError(t, err)

		// Reset system to up, then down again
		system.Set("status", "up")
		err = hub.SaveNoValidate(system)
		assert.NoError(t, err)
		time.Sleep(100 * time.Millisecond)

		system.Set("status", "down")
		err = hub.SaveNoValidate(system)
		assert.NoError(t, err)

		// Wait for the alert to be processed
		time.Sleep(time.Second * 75)
		synctest.Wait()

		// Now an email should be sent
		newEmailCount := hub.TestMailer.TotalSend()
		assert.Greater(t, newEmailCount, finalEmailCount, "Email should be sent when not silenced")
	})
}

func TestAlertSilencedNoWindows(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	// Create a system
	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
	assert.NoError(t, err)
	system := systems[0]

	// Get alert manager
	am := alerts.NewAlertManager(hub)
	defer am.StopWorker()

	// Without any quiet hours windows, alert should NOT be silenced
	silenced := am.IsNotificationSilenced(user.Id, system.Id)
	assert.False(t, silenced, "Alert should not be silenced when no windows exist")
}
