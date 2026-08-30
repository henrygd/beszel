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
	sys, err := tests.CreateRecord(hub, "systems", map[string]any{
		"name":   "probe-avg-system",
		"host":   "localhost",
		"port":   "45876",
		"status": "up",
		"users":  []string{user.Id},
	})
	require.NoError(t, err)
	probe, err := tests.CreateRecord(hub, "network_probes", map[string]any{
		"system":   sys.Id,
		"name":     "cloudflare",
		"target":   "1.1.1.1",
		"protocol": "icmp",
		"interval": 30,
		"enabled":  true,
	})
	require.NoError(t, err)

	// Per-probe records: flat stats array [avg, min, max, loss]
	recordA, err := tests.CreateRecord(hub, "network_probe_stats", map[string]any{
		"system": sys.Id,
		"probe":  probe.Id,
		"type":   "1m",
		"stats":  `[10,5,20,1.5]`,
	})
	require.NoError(t, err)
	recordB, err := tests.CreateRecord(hub, "network_probe_stats", map[string]any{
		"system": sys.Id,
		"probe":  probe.Id,
		"type":   "1m",
		"stats":  `[22.5,10,60,0]`,
	})
	require.NoError(t, err)

	result := rm.AverageProbeStats(hub.DB(), records.RecordIds{
		{Id: recordA.Id},
		{Id: recordB.Id},
	})

	require.Len(t, result, 4)
	assert.InDelta(t, 16.25, result[0], 0.001) // avg of avg
	assert.InDelta(t, 5, result[1], 0.001)     // min of mins
	assert.InDelta(t, 60, result[2], 0.001)    // max of maxes
	assert.InDelta(t, 0.75, result[3], 0.001)  // avg of packet loss
}
