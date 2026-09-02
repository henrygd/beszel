//go:build testing

package systems_test

import (
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/entities/systemd"
	"github.com/henrygd/beszel/internal/hub/systems"
	"github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/dbx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRecordsHandlesSystemdAlertLifecycle(t *testing.T) {
	hub, user := tests.GetHubWithUser(t)
	defer hub.Cleanup()

	settings, err := hub.FindFirstRecordByFilter("user_settings", "user={:user}", dbx.Params{"user": user.Id})
	require.NoError(t, err)
	settings.Set("settings", `{"emails":["test@example.com"],"webhooks":[]}`)
	require.NoError(t, hub.Save(settings))

	systemRecords, err := tests.CreateSystems(hub, 1, user.Id, "paused")
	require.NoError(t, err)
	systemRecord := systemRecords[0]
	alert, err := tests.CreateRecord(hub, "alerts", map[string]any{
		"name":   "SystemdFailed",
		"system": systemRecord.Id,
		"user":   user.Id,
	})
	require.NoError(t, err)

	monitoredSystem, err := hub.GetSystemManager().GetSystem(systemRecord.Id)
	require.NoError(t, err)
	initialEmailCount := hub.TestMailer.TotalSend()

	// Exercise the production path: persist the snapshot transactionally, save the
	// system record, and let its update hook evaluate and deliver the alert.
	_, err = monitoredSystem.CreateRecords(&system.CombinedData{
		Info:                   system.Info{Services: []uint16{1, 1}},
		SystemdServicesUpdated: true,
		SystemdServices: []*systemd.Service{
			{Name: "failed.service", State: systemd.StatusFailed},
		},
	})
	require.NoError(t, err)

	alert, err = hub.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	assert.True(t, alert.GetBool("triggered"))
	assert.Equal(t, initialEmailCount+1, hub.TestMailer.TotalSend())
	serviceCount, err := hub.CountRecords("systemd_services", dbx.HashExp{"system": systemRecord.Id})
	require.NoError(t, err)
	assert.EqualValues(t, 1, serviceCount)
	unresolvedCount, err := hub.CountRecords("alerts_history", dbx.HashExp{"alert_id": alert.Id, "resolved": ""})
	require.NoError(t, err)
	assert.EqualValues(t, 1, unresolvedCount)

	// A fresh empty snapshot must delete the old failed row and resolve the alert.
	_, err = monitoredSystem.CreateRecords(&system.CombinedData{
		Info:                   system.Info{Services: []uint16{0, 0}},
		SystemdServicesUpdated: true,
	})
	require.NoError(t, err)

	alert, err = hub.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	assert.False(t, alert.GetBool("triggered"))
	assert.Equal(t, initialEmailCount+2, hub.TestMailer.TotalSend())
	serviceCount, err = hub.CountRecords("systemd_services", dbx.HashExp{"system": systemRecord.Id})
	require.NoError(t, err)
	assert.Zero(t, serviceCount)
	unresolvedCount, err = hub.CountRecords("alerts_history", dbx.HashExp{"alert_id": alert.Id, "resolved": ""})
	require.NoError(t, err)
	assert.Zero(t, unresolvedCount)
}

// createSystemdStatsRecords upserts the reported services and must drop rows for
// services the agent has stopped reporting, so a unit removed from the host doesn't
// linger with its last known state until the retention sweep.
func TestCreateSystemdStatsRecordsRemovesUnreportedServices(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	user, err := tests.CreateUser(hub, "test@example.com", "password")
	require.NoError(t, err)
	system, err := tests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"host":  "127.0.0.1",
		"users": []string{user.Id},
	})
	require.NoError(t, err)

	serviceNames := func() []string {
		var out []string
		require.NoError(t, hub.DB().Select("name").From("systemd_services").
			Where(dbx.NewExp("system={:s}", dbx.Params{"s": system.Id})).
			OrderBy("name").Column(&out))
		return out
	}

	require.NoError(t, systems.CreateSystemdStatsRecords(hub, []*systemd.Service{
		{Name: "a.service", State: systemd.StatusActive},
		{Name: "gone.service", State: systemd.StatusFailed},
	}, system.Id))
	assert.Equal(t, []string{"a.service", "gone.service"}, serviceNames())

	// Batches are stamped with millisecond precision and update cycles are a minute
	// apart in practice; ensure the next batch gets a distinct timestamp.
	time.Sleep(2 * time.Millisecond)

	// gone.service is no longer reported, so its row must not survive.
	require.NoError(t, systems.CreateSystemdStatsRecords(hub, []*systemd.Service{
		{Name: "a.service", State: systemd.StatusActive},
	}, system.Id))
	assert.Equal(t, []string{"a.service"}, serviceNames())

	// A fresh empty snapshot means the agent no longer reports any services.
	require.NoError(t, systems.CreateSystemdStatsRecords(hub, nil, system.Id))
	assert.Empty(t, serviceNames())
}
