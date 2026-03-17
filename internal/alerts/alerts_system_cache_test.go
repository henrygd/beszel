//go:build testing

package alerts_test

import (
	"testing"

	"github.com/henrygd/beszel/internal/alerts"
	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystemAlertsCachePopulateAndFilter(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	systems, err := beszelTests.CreateSystems(hub, 2, user.Id, "up")
	require.NoError(t, err)
	system1 := systems[0]
	system2 := systems[1]

	statusAlert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":   "Status",
		"system": system1.Id,
		"user":   user.Id,
		"min":    1,
	})
	require.NoError(t, err)

	cpuAlert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":   "CPU",
		"system": system1.Id,
		"user":   user.Id,
		"value":  80,
		"min":    1,
	})
	require.NoError(t, err)

	memoryAlert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":   "Memory",
		"system": system2.Id,
		"user":   user.Id,
		"value":  90,
		"min":    1,
	})
	require.NoError(t, err)

	cache := alerts.NewAlertsCache(hub)
	cache.PopulateFromDB(false)

	statusAlerts := cache.GetAlertsByName(system1.Id, "Status")
	require.Len(t, statusAlerts, 1)
	assert.Equal(t, statusAlert.Id, statusAlerts[0].Id)

	nonStatusAlerts := cache.GetAlertsExcludingNames(system1.Id, "Status")
	require.Len(t, nonStatusAlerts, 1)
	assert.Equal(t, cpuAlert.Id, nonStatusAlerts[0].Id)

	system2Alerts := cache.GetSystemAlerts(system2.Id)
	require.Len(t, system2Alerts, 1)
	assert.Equal(t, memoryAlert.Id, system2Alerts[0].Id)
}

func TestSystemAlertsCacheLazyLoadUpdateAndDelete(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
	require.NoError(t, err)
	systemRecord := systems[0]

	statusAlert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":   "Status",
		"system": systemRecord.Id,
		"user":   user.Id,
		"min":    1,
	})
	require.NoError(t, err)

	cache := alerts.NewAlertsCache(hub)
	require.Len(t, cache.GetSystemAlerts(systemRecord.Id), 1, "first lookup should lazy-load alerts for the system")

	cpuAlert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":   "CPU",
		"system": systemRecord.Id,
		"user":   user.Id,
		"value":  80,
		"min":    1,
	})
	require.NoError(t, err)

	cache.Update(cpuAlert)

	nonStatusAlerts := cache.GetAlertsExcludingNames(systemRecord.Id, "Status")
	require.Len(t, nonStatusAlerts, 1)
	assert.Equal(t, cpuAlert.Id, nonStatusAlerts[0].Id)

	cache.Delete(statusAlert)
	assert.Empty(t, cache.GetAlertsByName(systemRecord.Id, "Status"), "deleted alerts should be removed from the in-memory cache")
}

func TestSystemAlertsCacheRefreshReturnsLatestCopy(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
	require.NoError(t, err)
	system := systems[0]

	alert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":      "Status",
		"system":    system.Id,
		"user":      user.Id,
		"min":       1,
		"triggered": false,
	})
	require.NoError(t, err)

	cache := alerts.NewAlertsCache(hub)
	snapshot := cache.GetSystemAlerts(system.Id)[0]
	assert.False(t, snapshot.Triggered)

	alert.Set("triggered", true)
	require.NoError(t, hub.Save(alert))

	refreshed, ok := cache.Refresh(snapshot)
	require.True(t, ok)
	assert.Equal(t, snapshot.Id, refreshed.Id)
	assert.True(t, refreshed.Triggered, "refresh should return the updated cached value rather than the stale snapshot")

	require.NoError(t, hub.Delete(alert))
	_, ok = cache.Refresh(snapshot)
	assert.False(t, ok, "refresh should report false when the cached alert no longer exists")
}

func TestAlertManagerCacheLifecycle(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
	require.NoError(t, err)
	system := systems[0]

	// Create an alert
	alert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":   "CPU",
		"system": system.Id,
		"user":   user.Id,
		"value":  80,
		"min":    1,
	})
	require.NoError(t, err)

	am := hub.AlertManager
	cache := am.GetSystemAlertsCache()

	// Verify it's in cache (it should be since CreateRecord triggers the event)
	assert.Len(t, cache.GetSystemAlerts(system.Id), 1)
	assert.Equal(t, alert.Id, cache.GetSystemAlerts(system.Id)[0].Id)
	assert.EqualValues(t, 80, cache.GetSystemAlerts(system.Id)[0].Value)

	// Update the alert through PocketBase to trigger events
	alert.Set("value", 85)
	require.NoError(t, hub.Save(alert))

	// Check if updated value is reflected (or at least that it's still there)
	cachedAlerts := cache.GetSystemAlerts(system.Id)
	assert.Len(t, cachedAlerts, 1)
	assert.EqualValues(t, 85, cachedAlerts[0].Value)

	// Delete the alert through PocketBase to trigger events
	require.NoError(t, hub.Delete(alert))

	// Verify it's removed from cache
	assert.Empty(t, cache.GetSystemAlerts(system.Id), "alert should be removed from cache after PocketBase delete")
}

// func TestAlertManagerCacheMovesAlertToNewSystemOnUpdate(t *testing.T) {
// 	hub, user := beszelTests.GetHubWithUser(t)
// 	defer hub.Cleanup()

// 	systems, err := beszelTests.CreateSystems(hub, 2, user.Id, "up")
// 	require.NoError(t, err)
// 	system1 := systems[0]
// 	system2 := systems[1]

// 	alert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
// 		"name":   "CPU",
// 		"system": system1.Id,
// 		"user":   user.Id,
// 		"value":  80,
// 		"min":    1,
// 	})
// 	require.NoError(t, err)

// 	am := hub.AlertManager
// 	cache := am.GetSystemAlertsCache()

// 	// Initially in system1 cache
// 	assert.Len(t, cache.Get(system1.Id), 1)
// 	assert.Empty(t, cache.Get(system2.Id))

// 	// Move alert to system2
// 	alert.Set("system", system2.Id)
// 	require.NoError(t, hub.Save(alert))

// 	// DEBUG: print if it is found
// 	// fmt.Printf("system1 alerts after update: %v\n", cache.Get(system1.Id))

// 	// Should be removed from system1 and present in system2
// 	assert.Empty(t, cache.GetType(system1.Id, "CPU"), "updated alerts should be evicted from the previous system cache")
// 	require.Len(t, cache.Get(system2.Id), 1)
// 	assert.Equal(t, alert.Id, cache.Get(system2.Id)[0].Id)
// }
