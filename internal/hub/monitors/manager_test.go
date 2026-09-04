//go:build testing

package monitors_test

import (
	"testing"

	"github.com/henrygd/beszel/internal/hub/monitors"
	_ "github.com/henrygd/beszel/internal/migrations"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMonitorTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { app.Cleanup() })
	return app
}

func seedMonitor(t *testing.T, app *tests.TestApp) *core.Record {
	t.Helper()
	users, err := app.FindCachedCollectionByNameOrId("users")
	require.NoError(t, err)
	user := core.NewRecord(users)
	user.Set("email", "w@example.com")
	user.Set("password", "password12345")
	require.NoError(t, app.Save(user))

	col, err := app.FindCachedCollectionByNameOrId("monitors")
	require.NoError(t, err)
	mon := core.NewRecord(col)
	mon.Set("name", "web")
	mon.Set("type", "http")
	mon.Set("target", "https://example.com")
	mon.Set("interval", 60)
	mon.Set("timeout", 10)
	mon.Set("max_retries", 2)
	mon.Set("status", "pending")
	mon.Set("users", []string{user.Id})
	require.NoError(t, app.Save(mon))
	return mon
}

func TestSaveCheckResult_WritesHistoryAndStatusAtomically(t *testing.T) {
	app := newMonitorTestApp(t)
	mon := seedMonitor(t, app)

	recs, err := monitors.LoadMonitors(app)
	require.NoError(t, err)
	require.Len(t, recs, 1)

	code := 200
	res := monitors.CheckResult{Status: "up", LatencyMs: 12.5, Code: &code, Message: "ok", Details: map[string]any{"final_url": "https://example.com"}}
	require.NoError(t, monitors.SaveCheckResult(app, recs[0], res, 0, false))

	updated, err := app.FindRecordById("monitors", mon.Id)
	require.NoError(t, err)
	assert.Equal(t, "up", updated.GetString("status"))
	assert.Equal(t, 12.5, updated.GetFloat("last_latency_ms"))
	assert.False(t, updated.GetDateTime("last_check").IsZero())

	total, err := app.CountRecords("monitor_checks")
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
}

func TestSaveCheckResult_TracksConsecutiveFailures(t *testing.T) {
	app := newMonitorTestApp(t)
	seedMonitor(t, app)

	recs, err := monitors.LoadMonitors(app)
	require.NoError(t, err)
	rec := recs[0]

	down := monitors.CheckResult{Status: "down", Message: "boom"}
	require.NoError(t, monitors.SaveCheckResult(app, rec, down, 1, false))
	updated, err := app.FindRecordById("monitors", rec.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, updated.GetFloat("consecutive_failures"))

	recs, err = monitors.LoadMonitors(app)
	require.NoError(t, err)
	require.NoError(t, monitors.SaveCheckResult(app, recs[0], monitors.CheckResult{Status: "up"}, 0, false))
	updated, err = app.FindRecordById("monitors", rec.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 0, updated.GetFloat("consecutive_failures"))
}

func TestLoadMonitors_SkipsPaused(t *testing.T) {
	app := newMonitorTestApp(t)
	mon := seedMonitor(t, app)
	mon.Set("paused", true)
	require.NoError(t, app.Save(mon))

	recs, err := monitors.LoadMonitors(app)
	require.NoError(t, err)
	assert.Empty(t, recs)
}

func TestUptime24h_ComputesRatio(t *testing.T) {
	app := newMonitorTestApp(t)
	mon := seedMonitor(t, app)

	insert := func(status string, n int) {
		for range n {
			_, err := app.DB().NewQuery("INSERT INTO monitor_checks (monitor, status, created, updated) VALUES ({:mon}, {:st}, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)").Bind(dbx.Params{"mon": mon.Id, "st": status}).Execute()
			require.NoError(t, err)
		}
	}
	insert("up", 3)
	insert("down", 1)

	ratio, err := monitors.Uptime24h(app, mon.Id)
	require.NoError(t, err)
	assert.InDelta(t, 75.0, ratio, 0.001)

	// Warn counts as success: the endpoint answers.
	insert("warn", 1)
	ratio, err = monitors.Uptime24h(app, mon.Id)
	require.NoError(t, err)
	assert.InDelta(t, 80.0, ratio, 0.001)

	empty, err := monitors.Uptime24h(app, "nonexistent")
	require.NoError(t, err)
	assert.Equal(t, 0.0, empty)
}

func TestSaveCheckResult_RejectsUnknownMonitor(t *testing.T) {
	app := newMonitorTestApp(t)
	rec := monitors.MonitorRecord{ID: "does-not-exist", Name: "x"}
	err := monitors.SaveCheckResult(app, rec, monitors.CheckResult{Status: "up"}, 0, false)
	require.Error(t, err, "saving for unknown monitor must fail the transaction")

	total, err := app.CountRecords("monitor_checks")
	require.NoError(t, err)
	assert.EqualValues(t, 0, total, "failed transaction must leave no history row")
}
