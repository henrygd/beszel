//go:build testing

package alerts_test

import (
	"testing"
	"time"

	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZfsPoolAlertOnlineToDegraded(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"users": []string{user.Id},
		"host":  "127.0.0.1",
	})
	assert.NoError(t, err)

	pool, err := beszelTests.CreateRecord(hub, "zfs_pools", map[string]any{
		"system": system.Id,
		"name":   "tank",
		"health": "ONLINE",
	})
	assert.NoError(t, err)

	// Re-fetch so PocketBase tracks original values
	pool, err = hub.FindRecordById("zfs_pools", pool.Id)
	assert.NoError(t, err)

	pool.Set("health", "DEGRADED")
	err = hub.Save(pool)
	assert.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	assert.EqualValues(t, 1, hub.TestMailer.TotalSend(), "should have 1 email sent after pool became DEGRADED")
	lastMessage := hub.TestMailer.LastMessage()
	assert.Contains(t, lastMessage.Subject, "ZFS pool DEGRADED on test-system")
	assert.Contains(t, lastMessage.Subject, "tank")
	assert.Contains(t, lastMessage.Text, "ONLINE to DEGRADED")
}

func TestZfsPoolAlertDegradedToFaulted(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"users": []string{user.Id},
		"host":  "127.0.0.1",
	})
	assert.NoError(t, err)

	pool, err := beszelTests.CreateRecord(hub, "zfs_pools", map[string]any{
		"system": system.Id,
		"name":   "rpool",
		"health": "DEGRADED",
	})
	assert.NoError(t, err)

	pool, err = hub.FindRecordById("zfs_pools", pool.Id)
	assert.NoError(t, err)

	pool.Set("health", "FAULTED")
	err = hub.Save(pool)
	assert.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	assert.EqualValues(t, 2, hub.TestMailer.TotalSend(), "should alert on initial DEGRADED state and later FAULTED transition")
	lastMessage := hub.TestMailer.LastMessage()
	assert.Contains(t, lastMessage.Subject, "ZFS pool FAULTED on test-system")
}

func TestZfsPoolAlertNoAlertOnRecovery(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"users": []string{user.Id},
		"host":  "127.0.0.1",
	})
	assert.NoError(t, err)

	pool, err := beszelTests.CreateRecord(hub, "zfs_pools", map[string]any{
		"system": system.Id,
		"name":   "tank",
		"health": "DEGRADED",
	})
	assert.NoError(t, err)

	// Trigger a worsening alert first
	pool, err = hub.FindRecordById("zfs_pools", pool.Id)
	assert.NoError(t, err)
	pool.Set("health", "FAULTED")
	err = hub.Save(pool)
	assert.NoError(t, err)
	time.Sleep(50 * time.Millisecond)
	assert.EqualValues(t, 2, hub.TestMailer.TotalSend(), "expected alerts for initial DEGRADED state and DEGRADED -> FAULTED")

	// Recovery back to ONLINE must not send a new alert
	pool, err = hub.FindRecordById("zfs_pools", pool.Id)
	assert.NoError(t, err)
	pool.Set("health", "ONLINE")
	err = hub.Save(pool)
	assert.NoError(t, err)
	time.Sleep(50 * time.Millisecond)
	assert.EqualValues(t, 2, hub.TestMailer.TotalSend(), "recovery should not send a new alert")

	// And the open history entry should have been resolved
	history, err := hub.FindRecordsByFilter("alerts_history", "alert_id={:alert_id}", "", 0, 0, map[string]any{"alert_id": pool.Id})
	assert.NoError(t, err)
	requireHistoryResolved(t, history)
}

func TestZfsPoolAlertUnknownHealthDoesNotResolve(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"users": []string{user.Id},
		"host":  "127.0.0.1",
	})
	require.NoError(t, err)

	pool, err := beszelTests.CreateRecord(hub, "zfs_pools", map[string]any{
		"system": system.Id,
		"name":   "tank",
		"health": "DEGRADED",
	})
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	pool, err = hub.FindRecordById("zfs_pools", pool.Id)
	require.NoError(t, err)
	pool.Set("health", "")
	require.NoError(t, hub.Save(pool))
	time.Sleep(50 * time.Millisecond)

	history, err := hub.FindRecordsByFilter("alerts_history", "alert_id={:alert_id} && resolved=null", "", 0, 0, map[string]any{"alert_id": pool.Id})
	require.NoError(t, err)
	require.Len(t, history, 1, "unknown health must not resolve an active alert")
}

func TestZfsPoolAlertUnknownToFaulted(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"users": []string{user.Id},
		"host":  "127.0.0.1",
	})
	assert.NoError(t, err)

	pool, err := beszelTests.CreateRecord(hub, "zfs_pools", map[string]any{
		"system": system.Id,
		"name":   "tank",
		"health": "",
	})
	assert.NoError(t, err)

	pool, err = hub.FindRecordById("zfs_pools", pool.Id)
	assert.NoError(t, err)

	pool.Set("health", "FAULTED")
	err = hub.Save(pool)
	assert.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	assert.EqualValues(t, 1, hub.TestMailer.TotalSend(), "should alert when a previously unknown pool becomes FAULTED")
}

func TestZfsPoolAlertOnInitialUnhealthyState(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"users": []string{user.Id},
		"host":  "127.0.0.1",
	})
	require.NoError(t, err)

	pool, err := beszelTests.CreateRecord(hub, "zfs_pools", map[string]any{
		"system": system.Id,
		"name":   "tank",
		"health": "DEGRADED",
	})
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	require.EqualValues(t, 1, hub.TestMailer.TotalSend())
	assert.Contains(t, hub.TestMailer.LastMessage().Text, "first observed as DEGRADED")

	pool, err = hub.FindRecordById("zfs_pools", pool.Id)
	require.NoError(t, err)
	require.NoError(t, hub.Save(pool))
	time.Sleep(50 * time.Millisecond)
	assert.EqualValues(t, 1, hub.TestMailer.TotalSend(), "unchanged unhealthy health must not duplicate alerts")
}

func TestZfsPoolAlertWritesHistory(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"users": []string{user.Id},
		"host":  "127.0.0.1",
	})
	assert.NoError(t, err)

	pool, err := beszelTests.CreateRecord(hub, "zfs_pools", map[string]any{
		"system": system.Id,
		"name":   "tank",
		"health": "ONLINE",
	})
	assert.NoError(t, err)

	pool, err = hub.FindRecordById("zfs_pools", pool.Id)
	assert.NoError(t, err)
	pool.Set("health", "FAULTED")
	err = hub.Save(pool)
	assert.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	history, err := hub.FindRecordsByFilter("alerts_history", "alert_id={:alert_id}", "", 0, 0, map[string]any{"alert_id": pool.Id})
	assert.NoError(t, err)
	require.Len(t, history, 1, "expected one history entry per user")
	assert.Equal(t, "ZFS Pool: tank", history[0].GetString("name"))
	assert.Equal(t, system.Id, history[0].GetString("system"))
}

func TestZfsPoolAlertResolvedOnRecordDelete(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"users": []string{user.Id},
		"host":  "127.0.0.1",
	})
	assert.NoError(t, err)

	pool, err := beszelTests.CreateRecord(hub, "zfs_pools", map[string]any{
		"system": system.Id,
		"name":   "tank",
		"health": "ONLINE",
	})
	assert.NoError(t, err)

	// Trigger an alert so an open history entry exists.
	pool, err = hub.FindRecordById("zfs_pools", pool.Id)
	assert.NoError(t, err)
	pool.Set("health", "FAULTED")
	err = hub.Save(pool)
	assert.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	history, err := hub.FindRecordsByFilter("alerts_history", "alert_id={:alert_id} && resolved=null", "", 0, 0, map[string]any{"alert_id": pool.Id})
	assert.NoError(t, err)
	require.Len(t, history, 1, "expected one open history entry")

	// Deleting the pool record must resolve the open entry.
	err = hub.Delete(pool)
	assert.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	history, err = hub.FindRecordsByFilter("alerts_history", "alert_id={:alert_id}", "", 0, 0, map[string]any{"alert_id": pool.Id})
	assert.NoError(t, err)
	require.Len(t, history, 1)
	requireHistoryResolved(t, history)
}

func requireHistoryResolved(t *testing.T, history []*core.Record) {
	t.Helper()
	for _, record := range history {
		assert.False(t, record.GetDateTime("resolved").Time().IsZero(), "expected history entry to be resolved")
	}
}
