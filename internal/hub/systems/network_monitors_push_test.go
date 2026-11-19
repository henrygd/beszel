//go:build testing

package systems

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPushMonitorResult(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 30, 20, 0, time.UTC)
	tests := []struct {
		name     string
		interval int
		lastPush time.Time
		wantUp   bool
	}{
		{"just pushed", 60, now, true},
		{"within interval", 3600, now.Add(-50 * time.Minute), true},
		{"within grace period", 60, now.Add(-100 * time.Second), true},
		{"overdue", 60, now.Add(-3 * time.Minute), false},
		{"daily heartbeat on time", 86400, now.Add(-23 * time.Hour), true},
		{"daily heartbeat overdue", 86400, now.Add(-25 * time.Hour), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pushMonitorResult(tt.interval, tt.lastPush.UnixMilli(), now)
			assert.Equal(t, int64(1), result.TotalCount)
			assert.Equal(t, now.Truncate(time.Minute).UnixMilli(), result.LastProbeAt, "probe time must be aligned to the minute")
			if tt.wantUp {
				assert.Equal(t, int64(1), result.SuccessCount)
				assert.Zero(t, result.PacketLoss1h)
			} else {
				assert.Zero(t, result.SuccessCount)
				assert.Equal(t, 100.0, result.PacketLoss1h)
			}
		})
	}
}

func TestPushMonitorResults(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	collection, err := app.FindCachedCollectionByNameOrId("network_monitors")
	require.NoError(t, err)
	create := func(target, protocol string, enabled bool, lastPush int64) *core.Record {
		record := core.NewRecord(collection)
		record.Load(map[string]any{
			"system": sys.Id, "target": target, "protocol": protocol,
			"interval": 60, "enabled": enabled, "lastPush": lastPush,
		})
		require.NoError(t, app.SaveNoValidate(record))
		return record
	}
	now := time.Now()
	active := create("backup", "push", true, now.UnixMilli())
	create("never pushed", "push", true, 0)
	create("disabled", "push", false, now.UnixMilli())
	create("1.1.1.1", "icmp", true, now.UnixMilli())

	results, err := sys.pushMonitorResults(app, now)
	require.NoError(t, err)
	require.Len(t, results, 1, "only enabled push monitors with a heartbeat are evaluated")
	assert.Equal(t, int64(1), results[active.Id].SuccessCount)
}

func TestGetMonitorConfigsForSystemSkipsPushMonitors(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	collection, err := app.FindCachedCollectionByNameOrId("network_monitors")
	require.NoError(t, err)
	for _, protocol := range []string{"push", "icmp"} {
		record := core.NewRecord(collection)
		record.Load(map[string]any{
			"system": sys.Id, "target": protocol + "-monitor", "protocol": protocol,
			"interval": 60, "enabled": true,
		})
		require.NoError(t, app.SaveNoValidate(record))
	}

	configs, err := sys.manager.GetMonitorConfigsForSystem(sys.Id)
	require.NoError(t, err)
	require.Len(t, configs, 1, "push monitors are evaluated by the hub and must not be sent to agents")
	require.Equal(t, "icmp", configs[0].Protocol)
}
