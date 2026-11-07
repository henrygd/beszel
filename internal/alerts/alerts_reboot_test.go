//go:build testing

package alerts_test

import (
	"testing"

	"github.com/henrygd/beszel/internal/alerts"
	systemEntity "github.com/henrygd/beszel/internal/entities/system"
	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rebootTestSetup(t *testing.T, triggered bool) (*beszelTests.TestHub, *core.Record, *core.Record) {
	t.Helper()

	hub, user := beszelTests.GetHubWithUser(t)

	userSettings, err := hub.FindFirstRecordByFilter("user_settings", "user={:user}", map[string]any{"user": user.Id})
	require.NoError(t, err)
	userSettings.Set("settings", `{"emails":["test@example.com"],"webhooks":[]}`)
	require.NoError(t, hub.Save(userSettings))

	// "paused" avoids spawning a background updater goroutine that would outlive
	// the test hub; these tests drive HandleRebootAlerts directly.
	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "paused")
	require.NoError(t, err)
	system := systems[0]

	alert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":      "RebootRequired",
		"system":    system.Id,
		"user":      user.Id,
		"triggered": triggered,
	})
	require.NoError(t, err)

	return hub, system, alert
}

func rebootData(packageUpdates []uint16, rebootRequired bool) *systemEntity.CombinedData {
	return &systemEntity.CombinedData{Info: systemEntity.Info{PackageUpdates: packageUpdates, RebootRequired: rebootRequired}}
}

func TestRebootAlertFullCycle(t *testing.T) {
	hub, system, alert := rebootTestSetup(t, false)
	defer hub.Cleanup()

	initialEmailCount := hub.TestMailer.TotalSend()
	am := alerts.NewTestAlertManagerWithoutWorker(hub)

	require.NoError(t, am.HandleRebootAlerts(system, rebootData([]uint16{0}, true)))
	alertRecord, err := hub.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	assert.True(t, alertRecord.GetBool("triggered"), "alert should be marked triggered")

	// still required: no repeat notification
	require.NoError(t, am.HandleRebootAlerts(system, rebootData([]uint16{0}, true)))

	require.NoError(t, am.HandleRebootAlerts(system, rebootData([]uint16{0}, false)))

	assert.Equal(t, initialEmailCount+2, hub.TestMailer.TotalSend(), "should send one alert and one resolved notification")
	messages := hub.TestMailer.Messages()
	require.Len(t, messages, 2)
	assert.Contains(t, messages[0].Subject, "Reboot required")
	assert.Contains(t, messages[1].Subject, "Reboot no longer required")

	alertRecord, err = hub.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	assert.False(t, alertRecord.GetBool("triggered"), "alert should be cleared")
}

func TestRebootAlertIgnoresUnknownState(t *testing.T) {
	hub, system, alert := rebootTestSetup(t, true)
	defer hub.Cleanup()

	initialEmailCount := hub.TestMailer.TotalSend()
	am := alerts.NewTestAlertManagerWithoutWorker(hub)

	// no package updates counts: agent has not finished a check (e.g. just restarted)
	require.NoError(t, am.HandleRebootAlerts(system, rebootData(nil, false)))
	require.NoError(t, am.HandleRebootAlerts(system, nil))

	assert.Equal(t, initialEmailCount, hub.TestMailer.TotalSend(), "unknown state must not resolve the alert")
	alertRecord, err := hub.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	assert.True(t, alertRecord.GetBool("triggered"))
}
