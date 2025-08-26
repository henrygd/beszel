//go:build testing
// +build testing

package systems_test

import (
	"beszel/internal/entities/container"
	"beszel/internal/entities/system"
	"beszel/internal/hub/systems"
	"beszel/internal/tests"
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystemManagerNew(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Cleanup()
	sm := hub.GetSystemManager()

	user, err := tests.CreateUser(hub, "test@test.com", "testtesttest")
	require.NoError(t, err)

	synctest.Test(t, func(t *testing.T) {
		sm.Initialize()

		record, err := tests.CreateRecord(hub, "systems", map[string]any{
			"name":  "it-was-coney-island",
			"host":  "the-playground-of-the-world",
			"port":  "33914",
			"users": []string{user.Id},
		})
		require.NoError(t, err)

		assert.Equal(t, "pending", record.GetString("status"), "System status should be 'pending'")
		assert.Equal(t, "pending", sm.GetSystemStatusFromStore(record.Id), "System status should be 'pending'")

		// Verify the system host and port
		host, port := sm.GetSystemHostPort(record.Id)
		assert.Equal(t, record.GetString("host"), host, "System host should match")
		assert.Equal(t, record.GetString("port"), port, "System port should match")

		time.Sleep(13 * time.Second)
		synctest.Wait()

		assert.Equal(t, "pending", record.Fresh().GetString("status"), "System status should be 'pending'")
		// Verify the system was added by checking if it exists
		assert.True(t, sm.HasSystem(record.Id), "System should exist in the store")

		time.Sleep(10 * time.Second)
		synctest.Wait()

		// system should be set to down after 15 seconds (no websocket connection)
		assert.Equal(t, "down", sm.GetSystemStatusFromStore(record.Id), "System status should be 'down'")
		// make sure the system is down in the db
		record, err = hub.FindRecordById("systems", record.Id)
		require.NoError(t, err)
		assert.Equal(t, "down", record.GetString("status"), "System status should be 'down'")

		assert.Equal(t, 1, sm.GetSystemCount(), "System count should be 1")

		err = sm.RemoveSystem(record.Id)
		assert.NoError(t, err)

		assert.Equal(t, 0, sm.GetSystemCount(), "System count should be 0")
		assert.False(t, sm.HasSystem(record.Id), "System should not exist in the store after removal")

		// let's also make sure a system is removed from the store when the record is deleted
		record, err = tests.CreateRecord(hub, "systems", map[string]any{
			"name":  "there-was-no-place-like-it",
			"host":  "in-the-whole-world",
			"port":  "33914",
			"users": []string{user.Id},
		})
		require.NoError(t, err)

		assert.True(t, sm.HasSystem(record.Id), "System should exist in the store after creation")

		time.Sleep(8 * time.Second)
		synctest.Wait()
		assert.Equal(t, "pending", sm.GetSystemStatusFromStore(record.Id), "System status should be 'pending'")

		sm.SetSystemStatusInDB(record.Id, "up")
		time.Sleep(time.Second)
		synctest.Wait()
		assert.Equal(t, "up", sm.GetSystemStatusFromStore(record.Id), "System status should be 'up'")

		// make sure the system switches to down after 11 seconds
		sm.RemoveSystem(record.Id)
		sm.AddRecord(record, nil)
		assert.Equal(t, "pending", sm.GetSystemStatusFromStore(record.Id), "System status should be 'pending'")
		time.Sleep(12 * time.Second)
		synctest.Wait()
		assert.Equal(t, "down", sm.GetSystemStatusFromStore(record.Id), "System status should be 'down'")

		// sm.SetSystemStatusInDB(record.Id, "paused")
		// time.Sleep(time.Second)
		// synctest.Wait()
		// assert.Equal(t, "paused", sm.GetSystemStatusFromStore(record.Id), "System status should be 'paused'")

		// delete the record
		err = hub.Delete(record)
		require.NoError(t, err)
		assert.False(t, sm.HasSystem(record.Id), "System should not exist in the store after deletion")
	})

	testOld(t, hub)

	synctest.Test(t, func(t *testing.T) {
		time.Sleep(time.Second)
		synctest.Wait()

		for _, systemId := range sm.GetAllSystemIDs() {
			err = sm.RemoveSystem(systemId)
			require.NoError(t, err)
			assert.False(t, sm.HasSystem(systemId), "System should not exist in the store after deletion")
		}

		assert.Equal(t, 0, sm.GetSystemCount(), "System count should be 0")

		// TODO: test with websocket client
	})
}

func testOld(t *testing.T, hub *tests.TestHub) {
	user, err := tests.CreateUser(hub, "test@testy.com", "testtesttest")
	require.NoError(t, err)

	sm := hub.GetSystemManager()
	assert.NotNil(t, sm)

	// error expected when creating a user with a duplicate email
	_, err = tests.CreateUser(hub, "test@test.com", "testtesttest")
	require.Error(t, err)

	// Test collection existence. todo: move to hub package tests
	t.Run("CollectionExistence", func(t *testing.T) {
		// Verify that required collections exist
		systems, err := hub.FindCachedCollectionByNameOrId("systems")
		require.NoError(t, err)
		assert.NotNil(t, systems)

		systemStats, err := hub.FindCachedCollectionByNameOrId("system_stats")
		require.NoError(t, err)
		assert.NotNil(t, systemStats)

		containerStats, err := hub.FindCachedCollectionByNameOrId("container_stats")
		require.NoError(t, err)
		assert.NotNil(t, containerStats)
	})

	t.Run("RemoveSystem", func(t *testing.T) {
		// Get the count before adding the system
		countBefore := sm.GetSystemCount()

		// Create a test system record
		record, err := tests.CreateRecord(hub, "systems", map[string]any{
			"name":  "i-even-got-lost-at-coney-island",
			"host":  "but-they-found-me",
			"port":  "33914",
			"users": []string{user.Id},
		})
		require.NoError(t, err)

		// Verify the system count increased
		countAfterAdd := sm.GetSystemCount()
		assert.Equal(t, countBefore+1, countAfterAdd, "System count should increase after adding a system via event hook")

		// Verify the system exists
		assert.True(t, sm.HasSystem(record.Id), "System should exist in the store")

		// Remove the system
		err = sm.RemoveSystem(record.Id)
		assert.NoError(t, err)

		// Check that the system count decreased
		countAfterRemove := sm.GetSystemCount()
		assert.Equal(t, countAfterAdd-1, countAfterRemove, "System count should decrease after removing a system")

		// Verify the system no longer exists
		assert.False(t, sm.HasSystem(record.Id), "System should not exist in the store after removal")

		// Verify the system is not in the list of all system IDs
		ids := sm.GetAllSystemIDs()
		assert.NotContains(t, ids, record.Id, "System ID should not be in the list of all system IDs after removal")

		// Verify the system status is empty
		status := sm.GetSystemStatusFromStore(record.Id)
		assert.Equal(t, "", status, "System status should be empty after removal")

		// Try to remove it again - should return an error since it's already removed
		err = sm.RemoveSystem(record.Id)
		assert.Error(t, err)
	})

	t.Run("NewRecordPending", func(t *testing.T) {
		// Create a test system
		record, err := tests.CreateRecord(hub, "systems", map[string]any{
			"name":  "and-you-know",
			"host":  "i-feel-very-bad",
			"port":  "33914",
			"users": []string{user.Id},
		})
		require.NoError(t, err)

		// Add the record to the system manager
		err = sm.AddRecord(record, nil)
		require.NoError(t, err)

		// Test filtering records by status - should be "pending" now
		filter := "status = 'pending'"
		pendingSystems, err := hub.FindRecordsByFilter("systems", filter, "-created", 0, 0, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(pendingSystems), 1)
	})

	t.Run("SystemStatusUpdate", func(t *testing.T) {
		// Create a test system record
		record, err := tests.CreateRecord(hub, "systems", map[string]any{
			"name":  "we-used-to-sleep-on-the-beach",
			"host":  "sleep-overnight-here",
			"port":  "33914",
			"users": []string{user.Id},
		})
		require.NoError(t, err)

		// Add the record to the system manager
		err = sm.AddRecord(record, nil)
		require.NoError(t, err)

		// Test status changes
		initialStatus := sm.GetSystemStatusFromStore(record.Id)

		// Set a new status
		sm.SetSystemStatusInDB(record.Id, "up")

		// Verify status was updated
		newStatus := sm.GetSystemStatusFromStore(record.Id)
		assert.Equal(t, "up", newStatus, "System status should be updated to 'up'")
		assert.NotEqual(t, initialStatus, newStatus, "Status should have changed")

		// Verify the database was updated
		updatedRecord, err := hub.FindRecordById("systems", record.Id)
		require.NoError(t, err)
		assert.Equal(t, "up", updatedRecord.Get("status"), "Database status should match")
	})

	t.Run("HandleSystemData", func(t *testing.T) {
		// Create a test system record
		record, err := tests.CreateRecord(hub, "systems", map[string]any{
			"name":  "things-changed-you-know",
			"host":  "they-dont-sleep-anymore-on-the-beach",
			"port":  "33914",
			"users": []string{user.Id},
		})
		require.NoError(t, err)

		// Create test system data
		testData := &system.CombinedData{
			Info: system.Info{
				Hostname:      "data-test.example.com",
				KernelVersion: "5.15.0-generic",
				Cores:         4,
				Threads:       8,
				CpuModel:      "Test CPU",
				Uptime:        3600,
				Cpu:           25.5,
				MemPct:        40.2,
				DiskPct:       60.0,
				Bandwidth:     100.0,
				AgentVersion:  "1.0.0",
			},
			Stats: system.Stats{
				Cpu:         25.5,
				Mem:         16384.0,
				MemUsed:     6553.6,
				MemPct:      40.0,
				DiskTotal:   1024000.0,
				DiskUsed:    614400.0,
				DiskPct:     60.0,
				NetworkSent: 1024.0,
				NetworkRecv: 2048.0,
			},
			Containers: []*container.Stats{},
		}

		// Test handling system data. todo: move to hub/alerts package tests
		err = hub.HandleSystemAlerts(record, testData)
		assert.NoError(t, err)
	})

	t.Run("ErrorHandling", func(t *testing.T) {
		// Try to add a non-existent record
		nonExistentId := "non_existent_id"
		err := sm.RemoveSystem(nonExistentId)
		assert.Error(t, err)

		// Try to add a system with invalid host
		system := &systems.System{
			Host: "",
		}
		err = sm.AddSystem(system)
		assert.Error(t, err)
	})

	t.Run("ConcurrentOperations", func(t *testing.T) {
		// Create a test system
		record, err := tests.CreateRecord(hub, "systems", map[string]any{
			"name":  "jfkjahkfajs",
			"host":  "localhost",
			"port":  "33914",
			"users": []string{user.Id},
		})
		require.NoError(t, err)

		// Run concurrent operations
		const goroutines = 5
		var wg sync.WaitGroup
		wg.Add(goroutines)

		for i := range goroutines {
			go func(i int) {
				defer wg.Done()

				// Alternate between different operations
				switch i % 3 {
				case 0:
					status := fmt.Sprintf("status-%d", i)
					sm.SetSystemStatusInDB(record.Id, status)
				case 1:
					_ = sm.GetSystemStatusFromStore(record.Id)
				case 2:
					_, _ = sm.GetSystemHostPort(record.Id)
				}
			}(i)
		}

		wg.Wait()

		// Verify system still exists and is in a valid state
		assert.True(t, sm.HasSystem(record.Id), "System should still exist after concurrent operations")
		status := sm.GetSystemStatusFromStore(record.Id)
		assert.NotEmpty(t, status, "System should have a status after concurrent operations")
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		// Create a test system record
		record, err := tests.CreateRecord(hub, "systems", map[string]any{
			"name":  "lkhsdfsjf",
			"host":  "localhost",
			"port":  "33914",
			"users": []string{user.Id},
		})
		require.NoError(t, err)

		// Verify the system exists in the store
		assert.True(t, sm.HasSystem(record.Id), "System should exist in the store")

		// Store the original context and cancel function
		originalCtx, originalCancel, err := sm.GetSystemContextFromStore(record.Id)
		assert.NoError(t, err)

		// Ensure the context is not nil
		assert.NotNil(t, originalCtx, "System context should not be nil")
		assert.NotNil(t, originalCancel, "System cancel function should not be nil")

		// Cancel the context
		originalCancel()

		// Wait a short time for cancellation to propagate
		time.Sleep(10 * time.Millisecond)

		// Verify the context is done
		select {
		case <-originalCtx.Done():
			// Context was properly cancelled
		default:
			t.Fatal("Context was not cancelled")
		}

		// Verify the system is still in the store (cancellation shouldn't remove it)
		assert.True(t, sm.HasSystem(record.Id), "System should still exist after context cancellation")

		// Explicitly remove the system
		err = sm.RemoveSystem(record.Id)
		assert.NoError(t, err, "RemoveSystem should succeed")

		// Verify the system is removed
		assert.False(t, sm.HasSystem(record.Id), "System should be removed after RemoveSystem")

		// Try to remove it again - should return an error
		err = sm.RemoveSystem(record.Id)
		assert.Error(t, err, "RemoveSystem should fail for non-existent system")

		// Add the system back
		err = sm.AddRecord(record, nil)
		require.NoError(t, err, "AddRecord should succeed")

		// Verify the system is back in the store
		assert.True(t, sm.HasSystem(record.Id), "System should exist after re-adding")

		// Verify a new context was created
		newCtx, newCancel, err := sm.GetSystemContextFromStore(record.Id)
		assert.NoError(t, err)
		assert.NotNil(t, newCtx, "New system context should not be nil")
		assert.NotNil(t, newCancel, "New system cancel function should not be nil")
		assert.NotEqual(t, originalCtx, newCtx, "New context should be different from original")

		// Clean up
		err = sm.RemoveSystem(record.Id)
		assert.NoError(t, err)
	})
}
