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

	ok := speedtest.Result{RunAt: 1000, Download: 125_000_000, Upload: 50_000_000, Ping: 8.5, Jitter: 0.4, Loss: 0, DownloadLatency: speedtest.Latency{IQM: 21.4, Jitter: 2.2}, UploadLatency: speedtest.Latency{IQM: 3.7, Jitter: 0.3}, ServerID: 42, ServerName: "Example", ServerLocation: "Amsterdam", ISP: "ISP", URL: "https://www.speedtest.net/result/c/abc"}
	record = save(ok)
	assert.Equal(t, 1000, record.GetInt("last_run"))
	assert.Equal(t, 125_000_000.0, record.GetFloat("download"))
	assert.Equal(t, "Example", record.GetString("server_name"))
	assert.Equal(t, 21.4, record.GetFloat("download_latency"))
	assert.Equal(t, 0.3, record.GetFloat("upload_jitter"))
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
	assert.Zero(t, failed.GetFloat("download"))
	assert.Zero(t, failed.GetFloat("ping"))

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
