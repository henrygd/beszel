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
		"stats":  `{"icmp:1.1.1.1":[10,80,8,14,1]}`,
	})
	require.NoError(t, err)
	recordB, err := tests.CreateRecord(hub, "network_probe_stats", map[string]any{
		"system": system.Id,
		"type":   "1m",
		"stats":  `{"icmp:1.1.1.1":[40,100,9,50,5]}`,
	})
	require.NoError(t, err)

	result := rm.AverageProbeStats(hub.DB(), records.RecordIds{
		{Id: recordA.Id},
		{Id: recordB.Id},
	})

	stats, ok := result["icmp:1.1.1.1"]
	require.True(t, ok)
	require.Len(t, stats, 5)
	assert.Equal(t, 25.0, stats[0])
	assert.Equal(t, 90.0, stats[1])
	assert.Equal(t, 8.0, stats[2])
	assert.Equal(t, 50.0, stats[3])
	assert.Equal(t, 3.0, stats[4])
}
