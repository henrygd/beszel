//go:build testing

package alerts_test

import (
	"testing"
	"time"

	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/dbx"
	"github.com/stretchr/testify/assert"
)

// TestGlobalAlertCreateSyncsToAllSystems verifies that creating a global alert
// creates matching per-system alerts for all existing systems.
func TestGlobalAlertCreateSyncsToAllSystems(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	systems, err := beszelTests.CreateSystems(hub, 3, user.Id, "up")
	assert.NoError(t, err)

	// Create a global alert for CPU
	_, err = beszelTests.CreateRecord(hub, "global_alerts", map[string]any{
		"name":  "CPU",
		"value": 80,
		"min":   5,
	})
	assert.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	// All 3 systems should now have a CPU alert
	for _, system := range systems {
		count, err := hub.CountRecords("alerts", dbx.HashExp{"system": system.Id, "name": "CPU"})
		assert.NoError(t, err)
		assert.EqualValues(t, 1, count, "system %s should have a CPU alert", system.Id)

		// Verify value and min were set correctly
		record, err := hub.FindFirstRecordByFilter("alerts",
			"system={:system} && name={:name}",
			dbx.Params{"system": system.Id, "name": "CPU"})
		assert.NoError(t, err)
		assert.EqualValues(t, 80, record.GetFloat("value"))
		assert.EqualValues(t, 5, record.GetInt("min"))
	}
}

// TestGlobalAlertCreateRespectsExclusions verifies that systems in excluded_systems
// do not receive per-system alerts when a global alert is created.
func TestGlobalAlertCreateRespectsExclusions(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	systems, err := beszelTests.CreateSystems(hub, 3, user.Id, "up")
	assert.NoError(t, err)

	excludedSystem := systems[1]

	// Create a global alert excluding one system
	_, err = beszelTests.CreateRecord(hub, "global_alerts", map[string]any{
		"name":             "Memory",
		"value":            90,
		"min":              10,
		"excluded_systems": []string{excludedSystem.Id},
	})
	assert.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	for i, system := range systems {
		count, err := hub.CountRecords("alerts", dbx.HashExp{"system": system.Id, "name": "Memory"})
		assert.NoError(t, err)
		if i == 1 {
			assert.EqualValues(t, 0, count, "excluded system should have no Memory alert")
		} else {
			assert.EqualValues(t, 1, count, "non-excluded system %s should have a Memory alert", system.Id)
		}
	}
}

// TestGlobalAlertUpdateSyncsToAllSystems verifies that updating a global alert
// updates the matching per-system alerts on all non-excluded systems.
func TestGlobalAlertUpdateSyncsToAllSystems(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	systems, err := beszelTests.CreateSystems(hub, 2, user.Id, "up")
	assert.NoError(t, err)

	ga, err := beszelTests.CreateRecord(hub, "global_alerts", map[string]any{
		"name":  "Disk",
		"value": 80,
		"min":   5,
	})
	assert.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	// Update the global alert threshold
	ga.Set("value", 95)
	ga.Set("min", 15)
	err = hub.Save(ga)
	assert.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	for _, system := range systems {
		record, err := hub.FindFirstRecordByFilter("alerts",
			"system={:system} && name={:name}",
			dbx.Params{"system": system.Id, "name": "Disk"})
		assert.NoError(t, err)
		assert.EqualValues(t, 95, record.GetFloat("value"), "value should be updated for system %s", system.Id)
		assert.EqualValues(t, 15, record.GetInt("min"), "min should be updated for system %s", system.Id)
	}
}

// TestGlobalAlertUpdateNewExclusionDeletesAlert verifies that when a system is newly
// added to excluded_systems on update, its per-system alert is deleted.
func TestGlobalAlertUpdateNewExclusionDeletesAlert(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	systems, err := beszelTests.CreateSystems(hub, 2, user.Id, "up")
	assert.NoError(t, err)
	targetSystem := systems[0]

	// Create a global alert with no exclusions
	ga, err := beszelTests.CreateRecord(hub, "global_alerts", map[string]any{
		"name":  "Temperature",
		"value": 70,
		"min":   5,
	})
	assert.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	// Both systems should have the alert
	count, err := hub.CountRecords("alerts", dbx.HashExp{"name": "Temperature"})
	assert.NoError(t, err)
	assert.EqualValues(t, 2, count)

	// Fetch fresh copy so PocketBase tracks original values
	ga, err = hub.FindRecordById("global_alerts", ga.Id)
	assert.NoError(t, err)

	// Now exclude the first system
	ga.Set("excluded_systems", []string{targetSystem.Id})
	err = hub.Save(ga)
	assert.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	// The newly-excluded system's alert should be gone
	count, err = hub.CountRecords("alerts", dbx.HashExp{"system": targetSystem.Id, "name": "Temperature"})
	assert.NoError(t, err)
	assert.EqualValues(t, 0, count, "newly-excluded system should have its alert deleted")

	// The other system should still have the alert
	count, err = hub.CountRecords("alerts", dbx.HashExp{"system": systems[1].Id, "name": "Temperature"})
	assert.NoError(t, err)
	assert.EqualValues(t, 1, count, "non-excluded system should still have the alert")
}

// TestGlobalAlertDeleteRemovesFromAllSystems verifies that deleting a global alert
// removes per-system alerts from all systems (including previously-excluded ones).
func TestGlobalAlertDeleteRemovesFromAllSystems(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	systems, err := beszelTests.CreateSystems(hub, 3, user.Id, "up")
	assert.NoError(t, err)
	excludedSystem := systems[2]

	ga, err := beszelTests.CreateRecord(hub, "global_alerts", map[string]any{
		"name":             "Bandwidth",
		"value":            100,
		"min":              5,
		"excluded_systems": []string{excludedSystem.Id},
	})
	assert.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	// Manually create an alert for the excluded system (simulating a per-system alert)
	_, err = beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"system": excludedSystem.Id,
		"name":   "Bandwidth",
		"value":  50,
		"min":    5,
	})
	assert.NoError(t, err)

	// Verify 3 alerts exist (2 from global sync + 1 manual)
	count, err := hub.CountRecords("alerts", dbx.HashExp{"name": "Bandwidth"})
	assert.NoError(t, err)
	assert.EqualValues(t, 3, count)

	// Delete the global alert
	ga, err = hub.FindRecordById("global_alerts", ga.Id)
	assert.NoError(t, err)
	err = hub.Delete(ga)
	assert.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	// All Bandwidth alerts (including the excluded system's) should be gone
	count, err = hub.CountRecords("alerts", dbx.HashExp{"name": "Bandwidth"})
	assert.NoError(t, err)
	assert.EqualValues(t, 0, count, "all Bandwidth alerts should be removed after global alert deletion")
}

// TestGlobalAlertAppliedToNewSystem verifies that when a new system is created,
// it automatically receives all active global alerts (except those that exclude it).
func TestGlobalAlertAppliedToNewSystem(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	// Create global alerts first (no systems yet)
	_, err := beszelTests.CreateRecord(hub, "global_alerts", map[string]any{
		"name":  "CPU",
		"value": 80,
		"min":   5,
	})
	assert.NoError(t, err)
	_, err = beszelTests.CreateRecord(hub, "global_alerts", map[string]any{
		"name":  "Memory",
		"value": 90,
		"min":   10,
	})
	assert.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	// Create a new system — should receive both global alerts
	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
	assert.NoError(t, err)
	system := systems[0]
	time.Sleep(50 * time.Millisecond)

	cpuCount, err := hub.CountRecords("alerts", dbx.HashExp{"system": system.Id, "name": "CPU"})
	assert.NoError(t, err)
	assert.EqualValues(t, 1, cpuCount, "new system should get the CPU global alert")

	memCount, err := hub.CountRecords("alerts", dbx.HashExp{"system": system.Id, "name": "Memory"})
	assert.NoError(t, err)
	assert.EqualValues(t, 1, memCount, "new system should get the Memory global alert")
}

// TestGlobalAlertExcludedSystemNotAppliedOnCreate verifies that a new system in
// an exclusion list does not receive that global alert when created.
func TestGlobalAlertExcludedSystemNotAppliedOnCreate(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	// Create the system first so we have its ID
	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
	assert.NoError(t, err)
	system := systems[0]

	// Create global alert that explicitly excludes the system
	_, err = beszelTests.CreateRecord(hub, "global_alerts", map[string]any{
		"name":             "GPU",
		"value":            90,
		"min":              5,
		"excluded_systems": []string{system.Id},
	})
	assert.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	count, err := hub.CountRecords("alerts", dbx.HashExp{"system": system.Id, "name": "GPU"})
	assert.NoError(t, err)
	assert.EqualValues(t, 0, count, "excluded system should not receive global GPU alert")
}
