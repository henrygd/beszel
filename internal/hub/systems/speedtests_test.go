//go:build testing

package systems

import (
	"testing"

	"github.com/henrygd/beszel/internal/entities/speedtest"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateSpeedtestRecords(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	col, err := app.FindCachedCollectionByNameOrId("speedtests")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Id = "speedtest1"
	record.Set("system", sys.Id)
	record.Set("interval", 60)
	record.Set("enabled", true)
	require.NoError(t, app.SaveNoValidate(record))

	statsCount := func() int64 {
		count, err := app.CountRecords("speedtest_stats", dbx.HashExp{"speedtest": "speedtest1"})
		require.NoError(t, err)
		return count
	}
	save := func(result speedtest.Result) *core.Record {
		t.Helper()
		_, err := sys.createRecords(&system.CombinedData{Speedtests: map[string]speedtest.Result{
			"speedtest1": result,
			// Results for unknown speedtests are ignored.
			"missing": {RunAt: 1},
		}})
		require.NoError(t, err)
		record, err := app.FindRecordById("speedtests", "speedtest1")
		require.NoError(t, err)
		return record
	}

	ok := speedtest.Result{RunAt: 1000, Download: 125_000_000, Upload: 50_000_000, Ping: 8.5, Jitter: 0.4, PingLow: 7.9, PingHigh: 9.2, Loss: 0, DownloadLatency: speedtest.Latency{IQM: 21.4, Low: 4, High: 48.7, Jitter: 2.2}, UploadLatency: speedtest.Latency{IQM: 3.7, Low: 3.3, High: 6.4, Jitter: 0.3}, ServerID: 42, ServerName: "Example", ServerLocation: "Amsterdam", ISP: "ISP", InterfaceName: "eth1", ExternalIP: "203.0.113.1", IsVPN: true, URL: "https://www.speedtest.net/result/c/abc"}
	record = save(ok)
	assert.Equal(t, 1000, record.GetInt("last_run"))
	assert.Equal(t, 125_000_000.0, record.GetFloat("download"))
	assert.Equal(t, "Example", record.GetString("server_name"))
	assert.Equal(t, 21.4, record.GetFloat("download_latency"))
	assert.Equal(t, 0.3, record.GetFloat("upload_jitter"))
	assert.Equal(t, 7.9, record.GetFloat("ping_low"))
	assert.Equal(t, "ISP", record.GetString("isp"))
	assert.Equal(t, "eth1", record.GetString("interface_name"))
	assert.Equal(t, "203.0.113.1", record.GetString("external_ip"))
	assert.True(t, record.GetBool("is_vpn"))
	assert.Empty(t, record.GetString("error"))
	assert.Equal(t, int64(1), statsCount())

	// The agent reports the same result until the next run; it must be stored once.
	save(ok)
	assert.Equal(t, int64(1), statsCount())

	// A failed run records the error and clears the previous measurements,
	// but keeps the server name for display.
	record = save(speedtest.Result{RunAt: 2000, ServerID: 42, Error: "no servers"})
	assert.Equal(t, 2000, record.GetInt("last_run"))
	assert.Equal(t, "no servers", record.GetString("error"))
	assert.Zero(t, record.GetFloat("download"))
	assert.Zero(t, record.GetFloat("ping"))
	assert.Empty(t, record.GetString("url"))
	assert.Equal(t, "Example", record.GetString("server_name"))
	// Its history row has the error and no measurements.
	assert.Equal(t, int64(2), statsCount())
	failed, err := app.FindFirstRecordByFilter("speedtest_stats", "created = 2000")
	require.NoError(t, err)
	assert.Equal(t, "no servers", failed.GetString("error"))
	assert.Equal(t, 42, failed.GetInt("server_id"))
	assert.Empty(t, failed.GetString("url"))
	assert.Zero(t, failed.GetFloat("download"))
	assert.Zero(t, failed.GetFloat("ping"))
	assert.Empty(t, failed.GetString("external_ip"))

	// A later successful run clears the error.
	ok.RunAt = 3000
	record = save(ok)
	assert.Empty(t, record.GetString("error"))
	assert.Equal(t, int64(3), statsCount())
	stats, err := app.FindFirstRecordByFilter("speedtest_stats", "created = 3000")
	require.NoError(t, err)
	assert.Empty(t, stats.GetString("error"))
	assert.Equal(t, sys.Id, stats.GetString("system"))
	assert.Equal(t, 50_000_000.0, stats.GetFloat("upload"))
	assert.Equal(t, 42, stats.GetInt("server_id"))
	assert.Equal(t, ok.URL, stats.GetString("url"))
	assert.Equal(t, 9.2, stats.GetFloat("ping_high"))
	assert.Equal(t, 48.7, stats.GetFloat("download_latency_high"))
	assert.Equal(t, 3.3, stats.GetFloat("upload_latency_low"))
	assert.Equal(t, "ISP", stats.GetString("isp"))
	assert.Equal(t, "eth1", stats.GetString("interface_name"))
	assert.Equal(t, "203.0.113.1", stats.GetString("external_ip"))
	assert.True(t, stats.GetBool("is_vpn"))
}

// Results are keyed by agent-supplied IDs, so a system may only write to its own speedtests.
func TestSpeedtestResultOwnership(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	systems, err := app.FindCachedCollectionByNameOrId("systems")
	require.NoError(t, err)
	foreignSystem := core.NewRecord(systems)
	require.NoError(t, app.SaveNoValidate(foreignSystem))
	col, err := app.FindCachedCollectionByNameOrId("speedtests")
	require.NoError(t, err)
	for id, systemID := range map[string]string{"owned": sys.Id, "foreign": foreignSystem.Id} {
		record := core.NewRecord(col)
		record.Id = id
		record.Load(map[string]any{"system": systemID, "interval": 60, "enabled": true})
		require.NoError(t, app.SaveNoValidate(record))
	}
	report := func(runAt int64) {
		t.Helper()
		result := speedtest.Result{RunAt: runAt, Download: 1000, ServerID: 42}
		_, err := sys.createRecords(&system.CombinedData{Speedtests: map[string]speedtest.Result{
			"owned": result, "foreign": result,
		}})
		require.NoError(t, err)
	}
	lastRun := func(id string) int {
		t.Helper()
		record, err := app.FindRecordById("speedtests", id)
		require.NoError(t, err)
		return record.GetInt("last_run")
	}
	statsFor := func(id string) int64 {
		t.Helper()
		count, err := app.CountRecords("speedtest_stats", dbx.HashExp{"speedtest": id})
		require.NoError(t, err)
		return count
	}

	report(1000)
	assert.Equal(t, 1000, lastRun("owned"))
	assert.Equal(t, int64(1), statsFor("owned"))
	assert.Zero(t, lastRun("foreign"))
	assert.Zero(t, statsFor("foreign"))

	// Ownership is read afresh, so a speedtest moved to another system stops accepting results.
	moved, err := app.FindRecordById("speedtests", "owned")
	require.NoError(t, err)
	moved.Set("system", foreignSystem.Id)
	require.NoError(t, app.SaveNoValidate(moved))
	report(2000)
	assert.Equal(t, 1000, lastRun("owned"))
	assert.Equal(t, int64(1), statsFor("owned"))
}

func TestGetSpeedtestConfigsForSystem(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	col, err := app.FindCachedCollectionByNameOrId("speedtests")
	require.NoError(t, err)
	for _, data := range []map[string]any{
		{"id": "enabled1", "server_id": 42, "interval": 60, "enabled": true},
		{"id": "auto1", "interval": 30, "enabled": true},
		{"id": "paused1", "interval": 60, "enabled": false},
	} {
		record := core.NewRecord(col)
		record.Load(data)
		record.Set("system", sys.Id)
		require.NoError(t, app.SaveNoValidate(record))
	}
	configs, err := sys.manager.GetSpeedtestConfigsForSystem(sys.Id)
	require.NoError(t, err)
	assert.ElementsMatch(t, []speedtest.Config{
		{ID: "enabled1", ServerID: 42, Interval: 60},
		{ID: "auto1", Interval: 30},
	}, configs)
}
