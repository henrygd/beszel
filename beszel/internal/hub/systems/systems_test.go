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
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestSystem creates a test system record with a unique host name
// and returns the created record and any error
func createTestSystem(t *testing.T, hub *tests.TestHub, options map[string]any) (*core.Record, error) {
	collection, err := hub.FindCachedCollectionByNameOrId("systems")
	if err != nil {
		return nil, err
	}

	// get user record
	var firstUser *core.Record
	users, err := hub.FindAllRecords("users", dbx.NewExp("id != ''"))
	if err != nil {
		t.Fatal(err)
	}
	if len(users) > 0 {
		firstUser = users[0]
	}
	// Generate a unique host name to ensure we're adding a new system
	uniqueHost := fmt.Sprintf("test-host-%d.example.com", time.Now().UnixNano())

	// Create the record
	record := core.NewRecord(collection)
	record.Set("name", uniqueHost)
	record.Set("host", uniqueHost)
	record.Set("port", "45876")
	record.Set("status", "pending")
	record.Set("users", []string{firstUser.Id})

	// Apply any custom options
	for key, value := range options {
		record.Set(key, value)
	}

	// Save the record to the database
	err = hub.Save(record)
	if err != nil {
		return nil, err
	}

	return record, nil
}

func TestSystemManagerIntegration(t *testing.T) {
	// Create a test hub
	hub, err := tests.NewTestHub()
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Cleanup()

	// Create independent system manager
	sm := systems.NewSystemManager(hub)
	assert.NotNil(t, sm)

	// Test initialization
	sm.Initialize()

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

	// Test adding a system record
	t.Run("AddRecord", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(2)

		// Get the count before adding the system
		countBefore := sm.GetSystemCount()

		// record should be pending on create
		hub.OnRecordCreate("systems").BindFunc(func(e *core.RecordEvent) error {
			record := e.Record
			if record.GetString("name") == "welcometoarcoampm" {
				assert.Equal(t, "pending", e.Record.GetString("status"), "System status should be 'pending'")
				wg.Done()
			}
			return e.Next()
		})

		// record should be down on update
		hub.OnRecordAfterUpdateSuccess("systems").BindFunc(func(e *core.RecordEvent) error {
			record := e.Record
			if record.GetString("name") == "welcometoarcoampm" {
				assert.Equal(t, "down", e.Record.GetString("status"), "System status should be 'pending'")
				wg.Done()
			}
			return e.Next()
		})
		// Create a test system with the first user assigned
		record, err := createTestSystem(t, hub, map[string]any{
			"name": "welcometoarcoampm",
			"host": "localhost",
			"port": "33914",
		})
		require.NoError(t, err)

		wg.Wait()

		// system should be down if grabbed from the store
		assert.Equal(t, "down", sm.GetSystemStatusFromStore(record.Id), "System status should be 'down'")

		// Check that the system count increased
		countAfter := sm.GetSystemCount()
		assert.Equal(t, countBefore+1, countAfter, "System count should increase after adding a system via event hook")

		// Verify the system was added by checking if it exists
		assert.True(t, sm.HasSystem(record.Id), "System should exist in the store")

		// Verify the system host and port
		host, port := sm.GetSystemHostPort(record.Id)
		assert.Equal(t, record.Get("host"), host, "System host should match")
		assert.Equal(t, record.Get("port"), port, "System port should match")

		// Verify the system is in the list of all system IDs
		ids := sm.GetAllSystemIDs()
		assert.Contains(t, ids, record.Id, "System ID should be in the list of all system IDs")

		// Verify the system was added by checking if removing it works
		err = sm.RemoveSystem(record.Id)
		assert.NoError(t, err, "System should exist and be removable")

		// Verify the system no longer exists
		assert.False(t, sm.HasSystem(record.Id), "System should not exist in the store after removal")

		// Verify the system is not in the list of all system IDs
		newIds := sm.GetAllSystemIDs()
		assert.NotContains(t, newIds, record.Id, "System ID should not be in the list of all system IDs after removal")

	})

	t.Run("RemoveSystem", func(t *testing.T) {
		// Get the count before adding the system
		countBefore := sm.GetSystemCount()

		// Create a test system record
		record, err := createTestSystem(t, hub, map[string]any{})
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
		record, err := createTestSystem(t, hub, map[string]any{})
		require.NoError(t, err)

		// Add the record to the system manager
		err = sm.AddRecord(record)
		require.NoError(t, err)

		// Test filtering records by status - should be "pending" now
		filter := "status = 'pending'"
		pendingSystems, err := hub.FindRecordsByFilter("systems", filter, "-created", 0, 0, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(pendingSystems), 1)
	})

	t.Run("SystemStatusUpdate", func(t *testing.T) {
		// Create a test system record
		record, err := createTestSystem(t, hub, map[string]any{})
		require.NoError(t, err)

		// Add the record to the system manager
		err = sm.AddRecord(record)
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
		record, err := createTestSystem(t, hub, map[string]any{})
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

	t.Run("DeleteRecord", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(2)

		runs := 0

		hub.OnRecordUpdate("systems").BindFunc(func(e *core.RecordEvent) error {
			runs++
			record := e.Record
			if record.GetString("name") == "deadflagblues" {
				if runs == 1 {
					assert.Equal(t, "up", e.Record.GetString("status"), "System status should be 'up'")
					wg.Done()
				} else if runs == 2 {
					assert.Equal(t, "paused", e.Record.GetString("status"), "System status should be 'paused'")
					wg.Done()
				}
			}
			return e.Next()
		})

		// Create a test system record
		record, err := createTestSystem(t, hub, map[string]any{
			"name": "deadflagblues",
		})
		require.NoError(t, err)

		// Verify the system exists
		assert.True(t, sm.HasSystem(record.Id), "System should exist in the store")

		// set the status manually to up
		sm.SetSystemStatusInDB(record.Id, "up")

		// verify the status is up
		assert.Equal(t, "up", sm.GetSystemStatusFromStore(record.Id), "System status should be 'up'")

		// Set the status to "paused" which should cause it to be deleted from the store
		sm.SetSystemStatusInDB(record.Id, "paused")

		wg.Wait()

		// Verify the system no longer exists
		assert.False(t, sm.HasSystem(record.Id), "System should not exist in the store after deletion")
	})

	t.Run("ConcurrentOperations", func(t *testing.T) {
		// Create a test system
		record, err := createTestSystem(t, hub, map[string]any{})
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
		record, err := createTestSystem(t, hub, map[string]any{})
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
		err = sm.AddRecord(record)
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
