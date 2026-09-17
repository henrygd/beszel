//go:build testing

package records_test

import (
	"testing"
	"time"

	monitorEntity "github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/henrygd/beszel/internal/records"
	"github.com/henrygd/beszel/internal/tests"

	"github.com/pocketbase/dbx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAverageMonitorStats(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	rm := records.NewRecordManager(hub)
	user, err := tests.CreateUser(hub, "monitor-avg@example.com", "testtesttest")
	require.NoError(t, err)
	sys, err := tests.CreateRecord(hub, "systems", map[string]any{
		"name":   "monitor-avg-system",
		"host":   "localhost",
		"port":   "45876",
		"status": "up",
		"users":  []string{user.Id},
	})
	require.NoError(t, err)
	monitor, err := tests.CreateRecord(hub, "network_monitors", map[string]any{
		"system":   sys.Id,
		"name":     "cloudflare",
		"target":   "1.1.1.1",
		"protocol": "icmp",
		"interval": 30,
		"enabled":  true,
	})
	require.NoError(t, err)

	created := time.Now().UnixMilli()
	// Unequal probe counts must weight both latency and loss.
	recordA, err := tests.CreateRecord(hub, "network_monitor_stats", map[string]any{
		"system":      sys.Id,
		"monitor":     monitor.Id,
		"type":        "1m",
		"created":     created,
		"res_avg":     10,
		"res_min":     5,
		"res_max":     20,
		"loss":        0,
		"total_count": 6, "success_count": 6, "response_sum": 60,
	})
	require.NoError(t, err)
	recordB, err := tests.CreateRecord(hub, "network_monitor_stats", map[string]any{
		"system":      sys.Id,
		"monitor":     monitor.Id,
		"type":        "1m",
		"created":     created,
		"res_avg":     22,
		"res_min":     10,
		"res_max":     60,
		"loss":        0,
		"total_count": 1, "success_count": 1, "response_sum": 22,
	})
	require.NoError(t, err)

	result, count, err := rm.AverageMonitorStats(hub.DB(), monitor.Id, "1m", created-1)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
	assert.Equal(t, monitorEntity.Stats{ResAvg: 11.71, ResMin: 5, ResMax: 60, TotalCount: 7, SuccessCount: 7, ResponseSum: 82}, result)

	for _, tc := range []struct {
		name, monitor, recordType string
		after                     int64
	}{
		{"other monitor", "missing", "1m", created - 1},
		{"other type", monitor.Id, "10m", created - 1},
		{"exclusive cutoff", monitor.Id, "1m", created},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stats, count, err := rm.AverageMonitorStats(hub.DB(), tc.monitor, tc.recordType, tc.after)
			require.NoError(t, err)
			assert.Zero(t, count)
			assert.Equal(t, monitorEntity.Stats{}, stats)
		})
	}

	// A failure-only bucket counts toward loss but must not lower latency.
	recordB.Set("res_avg", 0)
	recordB.Set("res_min", 0)
	recordB.Set("res_max", 0)
	recordB.Set("loss", 100)
	recordB.Set("success_count", 0)
	recordB.Set("response_sum", 0)
	require.NoError(t, hub.Save(recordB))
	result, count, err = rm.AverageMonitorStats(hub.DB(), monitor.Id, "1m", created-1)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
	assert.Equal(t, monitorEntity.Stats{ResAvg: 10, ResMin: 5, ResMax: 20, Loss: 14.29, TotalCount: 7, SuccessCount: 6, ResponseSum: 60}, result)
	// Sparse monitor records must propagate through every rollup level.
	rm.CreateLongerRecords()
	for _, recordType := range []string{"10m", "20m", "120m", "480m"} {
		rollups, err := hub.FindAllRecords("network_monitor_stats", dbx.HashExp{"monitor": monitor.Id, "type": recordType})
		require.NoError(t, err)
		require.Len(t, rollups, 1, recordType)
		assert.Equal(t, 10.0, rollups[0].GetFloat("res_avg"))
		assert.Equal(t, 5.0, rollups[0].GetFloat("res_min"))
		assert.Equal(t, 20.0, rollups[0].GetFloat("res_max"))
		assert.Equal(t, 14.29, rollups[0].GetFloat("loss"))
		assert.Equal(t, 7, rollups[0].GetInt("total_count"))
		assert.Equal(t, 6, rollups[0].GetInt("success_count"))
		assert.Equal(t, 60, rollups[0].GetInt("response_sum"))
		// A sibling with a different number of probes must retain its actual
		// weight when the next tier combines already-rounded display metrics.
		_, err = tests.CreateRecord(hub, "network_monitor_stats", map[string]any{
			"system": sys.Id, "monitor": monitor.Id, "type": recordType, "created": created,
			"res_avg": 100, "res_min": 100, "res_max": 100, "loss": 66.67,
			"total_count": 3, "success_count": 1, "response_sum": 100,
		})
		require.NoError(t, err)
		merged, count, err := rm.AverageMonitorStats(hub.DB(), monitor.Id, recordType, created-1)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
		assert.Equal(t, monitorEntity.Stats{
			ResAvg: 22.86, ResMin: 5, ResMax: 100, Loss: 30,
			TotalCount: 10, SuccessCount: 7, ResponseSum: 160,
		}, merged)
	}
	// All failures produce zero latency, while a genuine zero-microsecond
	// success remains a valid minimum (it must not be filtered out as a sentinel).
	recordA.Set("success_count", 0)
	recordA.Set("response_sum", 0)
	require.NoError(t, hub.Save(recordA))
	result, _, err = rm.AverageMonitorStats(hub.DB(), monitor.Id, "1m", created-1)
	require.NoError(t, err)
	assert.Equal(t, monitorEntity.Stats{TotalCount: 7, Loss: 100}, result)
	recordA.Set("success_count", 1)
	recordA.Set("res_min", 0)
	recordA.Set("res_max", 0)
	require.NoError(t, hub.Save(recordA))
	result, _, err = rm.AverageMonitorStats(hub.DB(), monitor.Id, "1m", created-1)
	require.NoError(t, err)
	assert.Equal(t, monitorEntity.Stats{TotalCount: 7, SuccessCount: 1, Loss: 85.71}, result)
}

func TestSparseMonitorRollups(t *testing.T) {
	for _, tc := range []struct {
		name     string
		interval int
		samples  int
		disabled bool
	}{
		{"five minute interval", 300, 2, false},
		{"ten minute interval", 600, 1, false},
		{"fifteen minute interval", 900, 1, false},
		{"empty window", 900, 0, false},
		{"disabled with pending history", 300, 2, true},
		{"disabled empty window", 900, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hub, err := tests.NewTestHub(t.TempDir())
			require.NoError(t, err)
			defer hub.Cleanup()
			user, err := tests.CreateUser(hub, "sparse-monitor@example.com", "testtesttest")
			require.NoError(t, err)
			sys, err := tests.CreateRecord(hub, "systems", map[string]any{
				"name": "sparse-monitor-system", "host": "localhost", "port": "45876", "status": "up",
				"users": []string{user.Id},
			})
			require.NoError(t, err)
			monitor, err := tests.CreateRecord(hub, "network_monitors", map[string]any{
				"system": sys.Id, "target": "1.1.1.1", "protocol": "icmp",
				"interval": tc.interval, "enabled": true,
			})
			require.NoError(t, err)
			now := time.Now()
			for i := range tc.samples {
				_, err := tests.CreateRecord(hub, "network_monitor_stats", map[string]any{
					"system": sys.Id, "monitor": monitor.Id, "type": "1m",
					"created": now.Add(-time.Minute - time.Duration(i*tc.interval)*time.Second).UnixMilli(),
					"res_avg": 12, "res_min": 8, "res_max": 20, "loss": 25,
					"total_count": 4, "success_count": 3, "response_sum": 36,
				})
				require.NoError(t, err)
			}
			// Other collections must still reject fewer than nine minute records.
			for _, collection := range []string{"system_stats", "container_stats"} {
				stats := `{"cpu":10}`
				if collection == "container_stats" {
					stats = `[{"name":"test","cpu":10}]`
				}
				for range 8 {
					_, err := tests.CreateRecord(hub, collection, map[string]any{
						"system": sys.Id, "type": "1m", "stats": stats,
					})
					require.NoError(t, err)
				}
			}
			if tc.disabled {
				monitor.Set("enabled", false)
				require.NoError(t, hub.Save(monitor))
			}
			records.NewRecordManager(hub).CreateLongerRecords()
			for _, recordType := range []string{"10m", "20m", "120m", "480m"} {
				rollups, err := hub.FindAllRecords("network_monitor_stats", dbx.HashExp{"monitor": monitor.Id, "type": recordType})
				require.NoError(t, err)
				if tc.samples == 0 {
					assert.Empty(t, rollups, recordType)
				} else {
					require.Len(t, rollups, 1, recordType)
					assert.Equal(t, 12.0, rollups[0].GetFloat("res_avg"))
					assert.Equal(t, 8.0, rollups[0].GetFloat("res_min"))
					assert.Equal(t, 20.0, rollups[0].GetFloat("res_max"))
					assert.Equal(t, 25.0, rollups[0].GetFloat("loss"))
				}
				for _, collection := range []string{"system_stats", "container_stats"} {
					count, err := hub.CountRecords(collection, dbx.HashExp{"system": sys.Id, "type": recordType})
					require.NoError(t, err)
					assert.Zero(t, count, "%s %s", collection, recordType)
				}
			}
		})
	}
}
