//go:build testing

package records_test

import (
	"testing"

	"github.com/henrygd/beszel/internal/records"
	"github.com/henrygd/beszel/internal/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAverageProbeStats(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	rm := records.NewRecordManager(hub)
	user, err := tests.CreateUser(hub, "probe-avg@example.com", "testtesttest")
	require.NoError(t, err)
	system, err := tests.CreateRecord(hub, "systems", map[string]any{
		"name":   "probe-avg-system",
		"host":   "localhost",
		"port":   "45876",
		"status": "up",
		"users":  []string{user.Id},
	})
	require.NoError(t, err)

	recordA, err := tests.CreateRecord(hub, "network_probe_stats", map[string]any{
		"system": system.Id,
		"type":   "1m",
		"stats":  `{"icmp:1.1.1.1":[10,5,20,1.5]}`,
	})
	require.NoError(t, err)
	recordB, err := tests.CreateRecord(hub, "network_probe_stats", map[string]any{
		"system": system.Id,
		"type":   "1m",
		"stats":  `{"icmp:1.1.1.1":[22.5,10,60,0]}`,
	})
	require.NoError(t, err)

	result := rm.AverageProbeStats(hub.DB(), records.RecordIds{
		{Id: recordA.Id},
		{Id: recordB.Id},
	})

	stats, ok := result["icmp:1.1.1.1"]
	require.True(t, ok)
	require.Len(t, stats, 4)
	assert.InDelta(t, 16.25, stats[0], 0.001) // avg of avg
	assert.InDelta(t, 5, stats[1], 0.001)     // min of mins
	assert.InDelta(t, 60, stats[2], 0.001)    // max of maxes
	assert.InDelta(t, 0.75, stats[3], 0.001)  // avg of packet loss
}
