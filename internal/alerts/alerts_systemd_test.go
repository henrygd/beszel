//go:build testing

package alerts_test

import (
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/alerts"
	systemEntity "github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/entities/systemd"
	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setSystemdServiceState upserts a systemd_services row mirroring the raw SQL write
// path used by the hub (createSystemdStatsRecords), which bypasses record hooks.
func setSystemdServiceState(t *testing.T, hub core.App, systemID, name string, state systemd.ServiceState, updated int64) {
	t.Helper()

	_, err := hub.DB().NewQuery(
		"INSERT INTO systemd_services (id, system, name, state, sub, cpu, cpuPeak, memory, memPeak, updated) " +
			"VALUES ({:id}, {:system}, {:name}, {:state}, 0, 0, 0, 0, 0, {:updated}) " +
			"ON CONFLICT(id) DO UPDATE SET state = excluded.state, updated = excluded.updated",
	).Bind(dbx.Params{
		"id":      systemID + "-" + name,
		"system":  systemID,
		"name":    name,
		"state":   state,
		"updated": updated,
	}).Execute()
	require.NoError(t, err)
}

// seedServices writes a set of services into the systemd_services snapshot, which is the
// source HandleSystemdAlerts reads from. All rows share one updated timestamp, matching
// how the hub writes a batch in createSystemdStatsRecords.
func seedServices(t *testing.T, hub core.App, systemID string, states ...systemd.ServiceState) {
	t.Helper()
	seedServicesAt(t, hub, systemID, time.Now().UTC().UnixMilli(), states...)
}

// seedServicesAt writes services with an explicit batch timestamp.
func seedServicesAt(t *testing.T, hub core.App, systemID string, updated int64, states ...systemd.ServiceState) {
	t.Helper()
	for i, state := range states {
		setSystemdServiceState(t, hub, systemID, serviceName(i), state, updated)
	}
}

func serviceName(i int) string {
	return string(rune('a'+i)) + ".service"
}

// systemdTestSetup creates a user with an email, a system, and a SystemdFailed alert.
func systemdTestSetup(t *testing.T, triggered bool) (*beszelTests.TestHub, *core.Record, *core.Record) {
	t.Helper()

	hub, user := beszelTests.GetHubWithUser(t)

	userSettings, err := hub.FindFirstRecordByFilter("user_settings", "user={:user}", map[string]any{"user": user.Id})
	require.NoError(t, err)
	userSettings.Set("settings", `{"emails":["test@example.com"],"webhooks":[]}`)
	require.NoError(t, hub.Save(userSettings))

	// "paused" avoids spawning a background updater goroutine that would outlive
	// the test hub; these tests drive HandleSystemdAlerts directly.
	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "paused")
	require.NoError(t, err)
	system := systems[0]

	alert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":      "SystemdFailed",
		"system":    system.Id,
		"user":      user.Id,
		"triggered": triggered,
	})
	require.NoError(t, err)

	return hub, system, alert
}

func TestSystemdAlertFiresImmediately(t *testing.T) {
	hub, system, alert := systemdTestSetup(t, false)
	defer hub.Cleanup()

	initialEmailCount := hub.TestMailer.TotalSend()
	am := alerts.NewTestAlertManagerWithoutWorker(hub)

	seedServices(t, hub, system.Id, systemd.StatusFailed, systemd.StatusActive)
	require.NoError(t, am.HandleSystemdAlerts(system))

	assert.Equal(t, initialEmailCount+1, hub.TestMailer.TotalSend(), "failed service should notify on first observation")

	messages := hub.TestMailer.Messages()
	require.NotEmpty(t, messages)
	last := messages[len(messages)-1]
	assert.Contains(t, last.Subject, "Failed services")
	assert.Contains(t, last.Text, "a.service", "notification should name the failed service")

	alertRecord, err := hub.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	assert.True(t, alertRecord.GetBool("triggered"), "alert should be marked triggered")

	// history record should be created via the alerts update hook
	historyCount, err := hub.CountRecords("alerts_history", dbx.HashExp{"resolved": ""})
	require.NoError(t, err)
	assert.EqualValues(t, 1, historyCount, "should have one unresolved alert history record")
}

func TestSystemdAlertFullCycle(t *testing.T) {
	hub, system, alert := systemdTestSetup(t, false)
	defer hub.Cleanup()

	initialEmailCount := hub.TestMailer.TotalSend()
	am := alerts.NewTestAlertManagerWithoutWorker(hub)

	// Fail, then recover.
	seedServices(t, hub, system.Id, systemd.StatusFailed)
	require.NoError(t, am.HandleSystemdAlerts(system))
	seedServices(t, hub, system.Id, systemd.StatusActive)
	require.NoError(t, am.HandleSystemdAlerts(system))

	assert.Equal(t, initialEmailCount+2, hub.TestMailer.TotalSend(), "should send a failure and a recovery notification")

	messages := hub.TestMailer.Messages()
	require.Len(t, messages, 2)
	assert.Contains(t, messages[0].Subject, "Failed services")
	assert.Contains(t, messages[1].Subject, "Services recovered")

	alertRecord, err := hub.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	assert.False(t, alertRecord.GetBool("triggered"), "alert should be cleared after recovery")

	// history record should be resolved
	historyCount, err := hub.CountRecords("alerts_history", dbx.HashExp{"resolved": ""})
	require.NoError(t, err)
	assert.Zero(t, historyCount, "alert history record should be resolved")
}

func TestSystemdAlertSendsRecoveryWhenTriggered(t *testing.T) {
	hub, system, alert := systemdTestSetup(t, true)
	defer hub.Cleanup()

	initialEmailCount := hub.TestMailer.TotalSend()
	am := alerts.NewTestAlertManagerWithoutWorker(hub)

	seedServices(t, hub, system.Id, systemd.StatusActive, systemd.StatusInactive)
	require.NoError(t, am.HandleSystemdAlerts(system))

	assert.Equal(t, initialEmailCount+1, hub.TestMailer.TotalSend(), "recovery notification should be sent")
	messages := hub.TestMailer.Messages()
	require.NotEmpty(t, messages)
	assert.Contains(t, messages[len(messages)-1].Subject, "Services recovered")

	alertRecord, err := hub.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	assert.False(t, alertRecord.GetBool("triggered"), "alert should be cleared after recovery")
}

func TestSystemdAlertDoesNotResendWhileTriggered(t *testing.T) {
	hub, system, _ := systemdTestSetup(t, true)
	defer hub.Cleanup()

	initialEmailCount := hub.TestMailer.TotalSend()
	am := alerts.NewTestAlertManagerWithoutWorker(hub)

	// Still failing across several cycles — should not re-notify.
	for range 3 {
		seedServices(t, hub, system.Id, systemd.StatusFailed)
		require.NoError(t, am.HandleSystemdAlerts(system))
	}

	assert.Equal(t, initialEmailCount, hub.TestMailer.TotalSend(), "should not re-notify while still triggered")
}

func TestSystemdAlertRepeatedFailureNotifiesOnce(t *testing.T) {
	hub, system, _ := systemdTestSetup(t, false)
	defer hub.Cleanup()

	initialEmailCount := hub.TestMailer.TotalSend()
	am := alerts.NewTestAlertManagerWithoutWorker(hub)

	for range 3 {
		seedServices(t, hub, system.Id, systemd.StatusFailed)
		require.NoError(t, am.HandleSystemdAlerts(system))
	}

	assert.Equal(t, initialEmailCount+1, hub.TestMailer.TotalSend(), "repeated failures should only notify once")
}

// A service that no longer exists on the host stops being reported, but its row stays
// in systemd_services with its last known state until the retention sweep. That stale
// row must not keep the alert triggered.
func TestSystemdAlertIgnoresServicesNoLongerReported(t *testing.T) {
	hub, system, alert := systemdTestSetup(t, true)
	defer hub.Cleanup()

	initialEmailCount := hub.TestMailer.TotalSend()
	am := alerts.NewTestAlertManagerWithoutWorker(hub)

	now := time.Now().UTC().UnixMilli()
	// Older batch still holding a failed service that has since been removed.
	setSystemdServiceState(t, hub, system.Id, "gone.service", systemd.StatusFailed, now-60_000)
	// Current batch reports only healthy services.
	seedServicesAt(t, hub, system.Id, now, systemd.StatusActive, systemd.StatusActive)

	require.NoError(t, am.HandleSystemdAlerts(system))

	assert.Equal(t, initialEmailCount+1, hub.TestMailer.TotalSend(), "stale failed row should not block recovery")
	alertRecord, err := hub.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	assert.False(t, alertRecord.GetBool("triggered"), "alert should resolve once the service stops being reported")
}

func TestResolveSystemdAlertsIgnoresStaleFailedRows(t *testing.T) {
	hub, system, alert := systemdTestSetup(t, true)
	defer hub.Cleanup()

	now := time.Now().UTC().UnixMilli()
	setSystemdServiceState(t, hub, system.Id, "gone.service", systemd.StatusFailed, now-60_000)
	seedServicesAt(t, hub, system.Id, now, systemd.StatusActive)

	require.NoError(t, alerts.ResolveSystemdAlerts(hub))

	alertRecord, err := hub.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	assert.False(t, alertRecord.GetBool("triggered"), "stale failed row should not keep the alert triggered")
}

func TestSystemdAlertNoSystemdDataIsIgnored(t *testing.T) {
	hub, system, alert := systemdTestSetup(t, true)
	defer hub.Cleanup()

	initialEmailCount := hub.TestMailer.TotalSend()
	am := alerts.NewTestAlertManagerWithoutWorker(hub)

	// A system with no systemd_services rows (agent without systemd, or nothing
	// reported yet) must not be treated as a recovery.
	require.NoError(t, am.HandleSystemdAlerts(system))
	require.NoError(t, am.HandleSystemdAlerts(system))

	assert.Equal(t, initialEmailCount, hub.TestMailer.TotalSend(), "missing systemd data should not send a recovery")
	alertRecord, err := hub.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	assert.True(t, alertRecord.GetBool("triggered"), "triggered state should be preserved when data is absent")
}

func TestSystemdAlertFreshEmptySnapshotResolves(t *testing.T) {
	hub, system, alert := systemdTestSetup(t, true)
	defer hub.Cleanup()

	initialEmailCount := hub.TestMailer.TotalSend()
	am := alerts.NewTestAlertManagerWithoutWorker(hub)

	// An explicit zero service count on the saved system record distinguishes a
	// confirmed empty snapshot from an agent response that omitted systemd data.
	system.Set("info", systemEntity.Info{Services: []uint16{0, 0}})
	require.NoError(t, am.HandleSystemAlerts(system, nil))

	assert.Equal(t, initialEmailCount+1, hub.TestMailer.TotalSend(), "fresh empty snapshot should send a recovery")
	alertRecord, err := hub.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	assert.False(t, alertRecord.GetBool("triggered"), "fresh empty snapshot should resolve the alert")
}

func TestSystemdAlertNoAlertRecord(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "paused")
	require.NoError(t, err)
	system := systems[0]

	initialEmailCount := hub.TestMailer.TotalSend()
	am := alerts.NewTestAlertManagerWithoutWorker(hub)

	seedServices(t, hub, system.Id, systemd.StatusFailed)
	require.NoError(t, am.HandleSystemdAlerts(system))
	assert.Equal(t, initialEmailCount, hub.TestMailer.TotalSend(), "no email when no alert record exists")
}

func TestResolveSystemdAlertsClearsStaleTriggered(t *testing.T) {
	hub, system, alert := systemdTestSetup(t, true)
	defer hub.Cleanup()

	// No failed services in the snapshot, but the alert is still marked triggered
	// (e.g. the hub restarted while the alert was active).
	setSystemdServiceState(t, hub, system.Id, "a.service", systemd.StatusActive, time.Now().UTC().UnixMilli())

	require.NoError(t, alerts.ResolveSystemdAlerts(hub))

	alertRecord, err := hub.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	assert.False(t, alertRecord.GetBool("triggered"), "stale triggered flag should be cleared")
}

func TestResolveSystemdAlertsKeepsTriggeredWithoutSystemdData(t *testing.T) {
	hub, _, alert := systemdTestSetup(t, true)
	defer hub.Cleanup()

	// Missing rows do not prove recovery. This can happen when a system is offline
	// and its last service snapshot has been removed by retention.
	require.NoError(t, alerts.ResolveSystemdAlerts(hub))

	alertRecord, err := hub.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	assert.True(t, alertRecord.GetBool("triggered"), "missing systemd data should preserve triggered state")
}

func TestResolveSystemdAlertsClearsConfirmedEmptySnapshot(t *testing.T) {
	hub, system, alert := systemdTestSetup(t, true)
	defer hub.Cleanup()

	// Update the persisted snapshot directly so record hooks don't alter alert state
	// before the startup resolver is exercised.
	_, err := hub.DB().NewQuery(
		"UPDATE systems SET info = {:info} WHERE id = {:id}",
	).Bind(dbx.Params{"info": `{"sv":[0,0]}`, "id": system.Id}).Execute()
	require.NoError(t, err)
	require.NoError(t, alerts.ResolveSystemdAlerts(hub))

	alertRecord, err := hub.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	assert.False(t, alertRecord.GetBool("triggered"), "confirmed empty snapshot should clear triggered state")
}

func TestResolveSystemdAlertsKeepsStillFailing(t *testing.T) {
	hub, system, alert := systemdTestSetup(t, true)
	defer hub.Cleanup()

	setSystemdServiceState(t, hub, system.Id, "a.service", systemd.StatusFailed, time.Now().UTC().UnixMilli())

	require.NoError(t, alerts.ResolveSystemdAlerts(hub))

	alertRecord, err := hub.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	assert.True(t, alertRecord.GetBool("triggered"), "alert should stay triggered while a service is still failed")
}

func TestSystemdAlertMultipleUsersRespectOwnAlerts(t *testing.T) {
	hub, user1 := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	setStatusAlertEmail(t, hub, user1.Id, "user1@example.com")

	user2, err := beszelTests.CreateUser(hub, "user2@example.com", "password")
	require.NoError(t, err)
	_, err = beszelTests.CreateRecord(hub, "user_settings", map[string]any{
		"user": user2.Id,
		"settings": map[string]any{
			"emails":   []string{"user2@example.com"},
			"webhooks": []string{},
		},
	})
	require.NoError(t, err)

	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "shared-system",
		"users": []string{user1.Id, user2.Id},
		"host":  "127.0.0.1",
	})
	require.NoError(t, err)

	for _, user := range []*core.Record{user1, user2} {
		_, err = beszelTests.CreateRecord(hub, "alerts", map[string]any{
			"name":   "SystemdFailed",
			"system": system.Id,
			"user":   user.Id,
		})
		require.NoError(t, err)
	}

	am := alerts.NewTestAlertManagerWithoutWorker(hub)
	seedServices(t, hub, system.Id, systemd.StatusFailed)
	require.NoError(t, am.HandleSystemdAlerts(system))

	messages := hub.TestMailer.Messages()
	require.Len(t, messages, 2, "each user should receive their own alert")
}
