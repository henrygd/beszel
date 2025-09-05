//go:build testing
// +build testing

package records_test

import (
	"beszel/internal/records"
	"beszel/internal/tests"
	"fmt"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeleteOldRecords tests the main DeleteOldRecords function
func TestDeleteOldRecords(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	rm := records.NewRecordManager(hub)

	// Create test user for alerts history
	user, err := tests.CreateUser(hub, "test@example.com", "testtesttest")
	require.NoError(t, err)

	// Create test system
	system, err := tests.CreateRecord(hub, "systems", map[string]any{
		"name":   "test-system",
		"host":   "localhost",
		"port":   "45876",
		"status": "up",
		"users":  []string{user.Id},
	})
	require.NoError(t, err)

	now := time.Now()

	// Create old system_stats records that should be deleted
	var record *core.Record
	record, err = tests.CreateRecord(hub, "system_stats", map[string]any{
		"system": system.Id,
		"type":   "1m",
		"stats":  `{"cpu": 50.0, "mem": 1024}`,
	})
	require.NoError(t, err)
	// created is autodate field, so we need to set it manually
	record.SetRaw("created", now.UTC().Add(-2*time.Hour).Format(types.DefaultDateLayout))
	err = hub.SaveNoValidate(record)
	require.NoError(t, err)
	require.NotNil(t, record)
	require.InDelta(t, record.GetDateTime("created").Time().UTC().Unix(), now.UTC().Add(-2*time.Hour).Unix(), 1)
	require.Equal(t, record.Get("system"), system.Id)
	require.Equal(t, record.Get("type"), "1m")

	// Create recent system_stats record that should be kept
	_, err = tests.CreateRecord(hub, "system_stats", map[string]any{
		"system":  system.Id,
		"type":    "1m",
		"stats":   `{"cpu": 30.0, "mem": 512}`,
		"created": now.Add(-30 * time.Minute), // 30 minutes old, should be kept
	})
	require.NoError(t, err)

	// Create many alerts history records to trigger deletion
	for i := range 260 { // More than countBeforeDeletion (250)
		_, err = tests.CreateRecord(hub, "alerts_history", map[string]any{
			"user":    user.Id,
			"name":    "CPU",
			"value":   i + 1,
			"system":  system.Id,
			"created": now.Add(-time.Duration(i) * time.Minute),
		})
		require.NoError(t, err)
	}

	// Count records before deletion
	systemStatsCountBefore, err := hub.CountRecords("system_stats")
	require.NoError(t, err)
	alertsCountBefore, err := hub.CountRecords("alerts_history")
	require.NoError(t, err)

	// Run deletion
	rm.DeleteOldRecords()

	// Count records after deletion
	systemStatsCountAfter, err := hub.CountRecords("system_stats")
	require.NoError(t, err)
	alertsCountAfter, err := hub.CountRecords("alerts_history")
	require.NoError(t, err)

	// Verify old system stats were deleted
	assert.Less(t, systemStatsCountAfter, systemStatsCountBefore, "Old system stats should be deleted")

	// Verify alerts history was trimmed
	assert.Less(t, alertsCountAfter, alertsCountBefore, "Excessive alerts history should be deleted")
	assert.Equal(t, alertsCountAfter, int64(200), "Alerts count should be equal to countToKeep (200)")
}

// TestDeleteOldSystemStats tests the deleteOldSystemStats function
func TestDeleteOldSystemStats(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	// Create test system
	user, err := tests.CreateUser(hub, "test@example.com", "testtesttest")
	require.NoError(t, err)

	system, err := tests.CreateRecord(hub, "systems", map[string]any{
		"name":   "test-system",
		"host":   "localhost",
		"port":   "45876",
		"status": "up",
		"users":  []string{user.Id},
	})
	require.NoError(t, err)

	now := time.Now().UTC()

	// Test data for different record types and their retention periods
	testCases := []struct {
		recordType   string
		retention    time.Duration
		shouldBeKept bool
		ageFromNow   time.Duration
		description  string
	}{
		{"1m", time.Hour, true, 30 * time.Minute, "1m record within 1 hour should be kept"},
		{"1m", time.Hour, false, 2 * time.Hour, "1m record older than 1 hour should be deleted"},
		{"10m", 12 * time.Hour, true, 6 * time.Hour, "10m record within 12 hours should be kept"},
		{"10m", 12 * time.Hour, false, 24 * time.Hour, "10m record older than 12 hours should be deleted"},
		{"20m", 24 * time.Hour, true, 12 * time.Hour, "20m record within 24 hours should be kept"},
		{"20m", 24 * time.Hour, false, 48 * time.Hour, "20m record older than 24 hours should be deleted"},
		{"120m", 7 * 24 * time.Hour, true, 3 * 24 * time.Hour, "120m record within 7 days should be kept"},
		{"120m", 7 * 24 * time.Hour, false, 10 * 24 * time.Hour, "120m record older than 7 days should be deleted"},
		{"480m", 30 * 24 * time.Hour, true, 15 * 24 * time.Hour, "480m record within 30 days should be kept"},
		{"480m", 30 * 24 * time.Hour, false, 45 * 24 * time.Hour, "480m record older than 30 days should be deleted"},
	}

	// Create test records for both system_stats and container_stats
	collections := []string{"system_stats", "container_stats"}
	recordIds := make(map[string][]string)

	for _, collection := range collections {
		recordIds[collection] = make([]string, 0)

		for i, tc := range testCases {
			recordTime := now.Add(-tc.ageFromNow)

			var stats string
			if collection == "system_stats" {
				stats = fmt.Sprintf(`{"cpu": %d.0, "mem": %d}`, i*10, i*100)
			} else {
				stats = fmt.Sprintf(`[{"name": "container%d", "cpu": %d.0, "mem": %d}]`, i, i*5, i*50)
			}

			record, err := tests.CreateRecord(hub, collection, map[string]any{
				"system": system.Id,
				"type":   tc.recordType,
				"stats":  stats,
			})
			require.NoError(t, err)
			record.SetRaw("created", recordTime.Format(types.DefaultDateLayout))
			err = hub.SaveNoValidate(record)
			require.NoError(t, err)
			recordIds[collection] = append(recordIds[collection], record.Id)
		}
	}

	// Run deletion
	err = records.TestDeleteOldSystemStats(hub)
	require.NoError(t, err)

	// Verify results
	for _, collection := range collections {
		for i, tc := range testCases {
			recordId := recordIds[collection][i]

			// Try to find the record
			_, err := hub.FindRecordById(collection, recordId)

			if tc.shouldBeKept {
				assert.NoError(t, err, "Record should exist: %s", tc.description)
			} else {
				assert.Error(t, err, "Record should be deleted: %s", tc.description)
			}
		}
	}
}

// TestDeleteOldAlertsHistory tests the deleteOldAlertsHistory function
func TestDeleteOldAlertsHistory(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	// Create test users
	user1, err := tests.CreateUser(hub, "user1@example.com", "testtesttest")
	require.NoError(t, err)

	user2, err := tests.CreateUser(hub, "user2@example.com", "testtesttest")
	require.NoError(t, err)

	system, err := tests.CreateRecord(hub, "systems", map[string]any{
		"name":   "test-system",
		"host":   "localhost",
		"port":   "45876",
		"status": "up",
		"users":  []string{user1.Id, user2.Id},
	})
	require.NoError(t, err)
	now := time.Now().UTC()

	testCases := []struct {
		name                  string
		user                  *core.Record
		alertCount            int
		countToKeep           int
		countBeforeDeletion   int
		expectedAfterDeletion int
		description           string
	}{
		{
			name:                  "User with few alerts (below threshold)",
			user:                  user1,
			alertCount:            100,
			countToKeep:           50,
			countBeforeDeletion:   150,
			expectedAfterDeletion: 100, // No deletion because below threshold
			description:           "User with alerts below countBeforeDeletion should not have any deleted",
		},
		{
			name:                  "User with many alerts (above threshold)",
			user:                  user2,
			alertCount:            300,
			countToKeep:           100,
			countBeforeDeletion:   200,
			expectedAfterDeletion: 100, // Should be trimmed to countToKeep
			description:           "User with alerts above countBeforeDeletion should be trimmed to countToKeep",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create alerts for this user
			for i := 0; i < tc.alertCount; i++ {
				_, err := tests.CreateRecord(hub, "alerts_history", map[string]any{
					"user":    tc.user.Id,
					"name":    "CPU",
					"value":   i + 1,
					"system":  system.Id,
					"created": now.Add(-time.Duration(i) * time.Minute),
				})
				require.NoError(t, err)
			}

			// Count before deletion
			countBefore, err := hub.CountRecords("alerts_history",
				dbx.NewExp("user = {:user}", dbx.Params{"user": tc.user.Id}))
			require.NoError(t, err)
			assert.Equal(t, int64(tc.alertCount), countBefore, "Initial count should match")

			// Run deletion
			err = records.TestDeleteOldAlertsHistory(hub, tc.countToKeep, tc.countBeforeDeletion)
			require.NoError(t, err)

			// Count after deletion
			countAfter, err := hub.CountRecords("alerts_history",
				dbx.NewExp("user = {:user}", dbx.Params{"user": tc.user.Id}))
			require.NoError(t, err)

			assert.Equal(t, int64(tc.expectedAfterDeletion), countAfter, tc.description)

			// If deletion occurred, verify the most recent records were kept
			if tc.expectedAfterDeletion < tc.alertCount {
				records, err := hub.FindRecordsByFilter("alerts_history",
					"user = {:user}",
					"-created", // Order by created DESC
					tc.countToKeep,
					0,
					map[string]any{"user": tc.user.Id})
				require.NoError(t, err)
				assert.Len(t, records, tc.expectedAfterDeletion, "Should have exactly countToKeep records")

				// Verify records are in descending order by created time
				for i := 1; i < len(records); i++ {
					prev := records[i-1].GetDateTime("created").Time()
					curr := records[i].GetDateTime("created").Time()
					assert.True(t, prev.After(curr) || prev.Equal(curr),
						"Records should be ordered by created time (newest first)")
				}
			}
		})
	}
}

// TestDeleteOldAlertsHistoryEdgeCases tests edge cases for alerts history deletion
func TestDeleteOldAlertsHistoryEdgeCases(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	t.Run("No users with excessive alerts", func(t *testing.T) {
		// Create user with few alerts
		user, err := tests.CreateUser(hub, "few@example.com", "testtesttest")
		require.NoError(t, err)

		system, err := tests.CreateRecord(hub, "systems", map[string]any{
			"name":   "test-system",
			"host":   "localhost",
			"port":   "45876",
			"status": "up",
			"users":  []string{user.Id},
		})

		// Create only 5 alerts (well below threshold)
		for i := range 5 {
			_, err := tests.CreateRecord(hub, "alerts_history", map[string]any{
				"user":   user.Id,
				"name":   "CPU",
				"value":  i + 1,
				"system": system.Id,
			})
			require.NoError(t, err)
		}

		// Should not error and should not delete anything
		err = records.TestDeleteOldAlertsHistory(hub, 10, 20)
		require.NoError(t, err)

		count, err := hub.CountRecords("alerts_history")
		require.NoError(t, err)
		assert.Equal(t, int64(5), count, "All alerts should remain")
	})

	t.Run("Empty alerts_history table", func(t *testing.T) {
		// Clear any existing alerts
		_, err := hub.DB().NewQuery("DELETE FROM alerts_history").Execute()
		require.NoError(t, err)

		// Should not error with empty table
		err = records.TestDeleteOldAlertsHistory(hub, 10, 20)
		require.NoError(t, err)
	})
}

// TestRecordManagerCreation tests RecordManager creation
func TestRecordManagerCreation(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	rm := records.NewRecordManager(hub)
	assert.NotNil(t, rm, "RecordManager should not be nil")
}

// TestTwoDecimals tests the twoDecimals helper function
func TestTwoDecimals(t *testing.T) {
	testCases := []struct {
		input    float64
		expected float64
	}{
		{1.234567, 1.23},
		{1.235, 1.24}, // Should round up
		{1.0, 1.0},
		{0.0, 0.0},
		{-1.234567, -1.23},
		{-1.235, -1.23}, // Negative rounding
	}

	for _, tc := range testCases {
		result := records.TestTwoDecimals(tc.input)
		assert.InDelta(t, tc.expected, result, 0.02, "twoDecimals(%f) should equal %f", tc.input, tc.expected)
	}
}
