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
	// Each record stores named response metrics and packet loss.
	_, err = tests.CreateRecord(hub, "network_monitor_stats", map[string]any{
		"system":  sys.Id,
		"monitor": monitor.Id,
		"type":    "1m",
		"created": created,
		"res_avg": 10,
		"res_min": 5,
		"res_max": 20,
		"loss":    1.5,
	})
	require.NoError(t, err)
	recordB, err := tests.CreateRecord(hub, "network_monitor_stats", map[string]any{
		"system":  sys.Id,
		"monitor": monitor.Id,
		"type":    "1m",
		"created": created,
		"res_avg": 22.5,
		"res_min": 10,
		"res_max": 60,
		"loss":    0,
	})
	require.NoError(t, err)

	result, count, err := rm.AverageMonitorStats(hub.DB(), monitor.Id, "1m", created-1)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
	assert.Equal(t, monitorEntity.Stats{ResAvg: 16.25, ResMin: 5, ResMax: 60, Loss: 0.75}, result)

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

	// Zero response times and complete packet loss remain valid metrics.
	recordB.Set("res_avg", 0)
	recordB.Set("res_min", 0)
	recordB.Set("res_max", 0)
	recordB.Set("loss", 100)
	require.NoError(t, hub.Save(recordB))
	result, count, err = rm.AverageMonitorStats(hub.DB(), monitor.Id, "1m", created-1)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
	assert.Equal(t, monitorEntity.Stats{ResAvg: 5, ResMin: 0, ResMax: 20, Loss: 50.75}, result)
	// Sparse monitor records must propagate through every rollup level.
	rm.CreateLongerRecords()
	for _, recordType := range []string{"10m", "20m", "120m", "480m"} {
		rollups, err := hub.FindAllRecords("network_monitor_stats", dbx.HashExp{"monitor": monitor.Id, "type": recordType})
		require.NoError(t, err)
		require.Len(t, rollups, 1, recordType)
		assert.Equal(t, 5.0, rollups[0].GetFloat("res_avg"))
		assert.Equal(t, 0.0, rollups[0].GetFloat("res_min"))
		assert.Equal(t, 20.0, rollups[0].GetFloat("res_max"))
		assert.Equal(t, 50.75, rollups[0].GetFloat("loss"))
	}
}

func TestSparseMonitorRollups(t *testing.T) {
	for _, tc := range []struct {
		name     string
		interval int
		samples  int
	}{
		{"five minute interval", 300, 2},
		{"ten minute interval", 600, 1},
		{"fifteen minute interval", 900, 1},
		{"empty window", 900, 0},
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
