//go:build testing

package records_test

import (
	"testing"

	monitorEntity "github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/henrygd/beszel/internal/records"
	"github.com/henrygd/beszel/internal/tests"

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

	// Each record stores named response metrics and packet loss.
	recordA, err := tests.CreateRecord(hub, "network_monitor_stats", map[string]any{
		"system":  sys.Id,
		"monitor": monitor.Id,
		"type":    "1m",
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
		"res_avg": 22.5,
		"res_min": 10,
		"res_max": 60,
		"loss":    0,
	})
	require.NoError(t, err)

	result := rm.AverageMonitorStats(hub.DB(), records.RecordIds{
		{Id: recordA.Id},
		{Id: recordB.Id},
	})

	assert.Equal(t, monitorEntity.Stats{ResAvg: 16.25, ResMin: 5, ResMax: 60, Loss: 0.75}, result)
	assert.Equal(t, monitorEntity.Stats{}, rm.AverageMonitorStats(hub.DB(), nil))
	assert.Equal(t, monitorEntity.Stats{}, rm.AverageMonitorStats(hub.DB(), records.RecordIds{{Id: "missing"}}))

	// Zero response times and complete packet loss remain valid metrics.
	recordB.Set("res_avg", 0)
	recordB.Set("res_min", 0)
	recordB.Set("res_max", 0)
	recordB.Set("loss", 100)
	require.NoError(t, hub.Save(recordB))
	result = rm.AverageMonitorStats(hub.DB(), records.RecordIds{{Id: recordA.Id}, {Id: recordB.Id}})
	assert.Equal(t, monitorEntity.Stats{ResAvg: 5, ResMin: 0, ResMax: 20, Loss: 50.75}, result)
}
