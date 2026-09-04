//go:build testing

package migrations_test

import (
	"testing"

	_ "github.com/henrygd/beszel/internal/migrations"
	"github.com/henrygd/beszel/internal/records"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMonitorCollectionsExist(t *testing.T) {
	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	defer app.Cleanup()

	monitors, err := app.FindCachedCollectionByNameOrId("monitors")
	require.NoError(t, err, "monitors collection must exist")
	assert.NotEmpty(t, monitors.Fields)

	checks, err := app.FindCachedCollectionByNameOrId("monitor_checks")
	require.NoError(t, err, "monitor_checks collection must exist")
	assert.NotEmpty(t, checks.Fields)
}

func createMonitorRecord(t *testing.T, app *tests.TestApp, userID string) *core.Record {
	t.Helper()
	monitors, err := app.FindCachedCollectionByNameOrId("monitors")
	require.NoError(t, err)
	mon := core.NewRecord(monitors)
	mon.Set("name", "cascade")
	mon.Set("type", "ping")
	mon.Set("target", "example.com")
	mon.Set("interval", 60)
	mon.Set("timeout", 10)
	mon.Set("users", []string{userID})
	require.NoError(t, app.Save(mon))
	return mon
}

func TestMonitorChecksCascadeOnMonitorDelete(t *testing.T) {
	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	defer app.Cleanup()

	users, err := app.FindCachedCollectionByNameOrId("users")
	require.NoError(t, err)
	user := core.NewRecord(users)
	user.Set("email", "mon@example.com")
	user.Set("password", "password12345")
	require.NoError(t, app.Save(user))

	mon := createMonitorRecord(t, app, user.Id)

	checksCol, err := app.FindCachedCollectionByNameOrId("monitor_checks")
	require.NoError(t, err)
	check := core.NewRecord(checksCol)
	check.Set("monitor", mon.Id)
	check.Set("status", "up")
	check.Set("latency_ms", 1.5)
	require.NoError(t, app.Save(check))

	require.NoError(t, app.Delete(mon))
	remaining, err := app.CountRecords("monitor_checks")
	require.NoError(t, err)
	assert.EqualValues(t, 0, remaining, "checks must cascade-delete with monitor")
}

func TestMonitorChecksRetention(t *testing.T) {
	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	defer app.Cleanup()

	users, err := app.FindCachedCollectionByNameOrId("users")
	require.NoError(t, err)
	user := core.NewRecord(users)
	user.Set("email", "ret@example.com")
	user.Set("password", "password12345")
	require.NoError(t, app.Save(user))
	mon := createMonitorRecord(t, app, user.Id)

	// 40 days old (beyond 30d retention) + 1h old (kept).
	old := "2020-01-01 00:00:00.000Z"
	_, err = app.DB().NewQuery("INSERT INTO monitor_checks (monitor, status, created, updated) VALUES ({:mon}, 'up', {:old}, {:old})").Bind(dbx.Params{"mon": mon.Id, "old": old}).Execute()
	require.NoError(t, err)

	require.NoError(t, records.DeleteOldMonitorChecks(app))

	total, err := app.CountRecords("monitor_checks")
	require.NoError(t, err)
	assert.EqualValues(t, 0, total, "old checks must be purged")
}
