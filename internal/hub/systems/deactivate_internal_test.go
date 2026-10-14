//go:build testing

package systems

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	pbTests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/henrygd/beszel/internal/migrations"
)

// TestDeactivateAlertsParameterized pins the deactivate flow with a
// parameterized filter: triggered alerts of a system are reset and alerts of
// other systems stay untouched. Regression target: the filter was built with
// fmt.Sprintf — the only string-concatenated filter in the hub — instead of
// bind parameters.
func TestDeactivateAlertsParameterized(t *testing.T) {
	app, err := pbTests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	defer app.Cleanup()

	usersCol, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	user := core.NewRecord(usersCol)
	user.Set("email", "deact@test.local")
	user.Set("password", "1234567890")
	user.Set("role", "user")
	require.NoError(t, app.Save(user))

	systemsCol, err := app.FindCollectionByNameOrId("systems")
	require.NoError(t, err)
	newSystem := func(name, host string) *core.Record {
		rec := core.NewRecord(systemsCol)
		rec.Set("name", name)
		rec.Set("host", host)
		rec.Set("port", "45998")
		rec.Set("users", []string{user.Id})
		require.NoError(t, app.Save(rec))
		return rec
	}
	system1 := newSystem("deact-test", "127.0.0.1")
	system2 := newSystem("deact-test-2", "127.0.0.2")

	alertsCol, err := app.FindCollectionByNameOrId("alerts")
	require.NoError(t, err)
	addAlert := func(system *core.Record, name string, triggered bool) *core.Record {
		rec := core.NewRecord(alertsCol)
		rec.Set("user", user.Id)
		rec.Set("system", system.Id)
		rec.Set("name", name)
		rec.Set("value", 80)
		rec.Set("triggered", triggered)
		require.NoError(t, app.Save(rec))
		return rec
	}
	cpu := addAlert(system1, "CPU", true)
	status := addAlert(system1, "Status", true)
	memoryOtherSystem := addAlert(system2, "Memory", true)

	require.NoError(t, deactivateAlerts(app, system1.Id, false))

	reloaded, err := app.FindRecordById("alerts", cpu.Id)
	require.NoError(t, err)
	assert.False(t, reloaded.GetBool("triggered"), "triggered alert must be deactivated")

	reloaded, err = app.FindRecordById("alerts", status.Id)
	require.NoError(t, err)
	assert.False(t, reloaded.GetBool("triggered"), "Status alert deactivated when not preserved")

	reloaded, err = app.FindRecordById("alerts", memoryOtherSystem.Id)
	require.NoError(t, err)
	assert.True(t, reloaded.GetBool("triggered"),
		"alerts of other systems must stay untouched")

	// preserveStatusAlert=true keeps a triggered Status alert active so a
	// confirmed recovery can still send the "up" notification. Re-trigger
	// both alerts first: the first pass already reset them.
	status.Set("triggered", true)
	require.NoError(t, app.Save(status))
	cpu.Set("triggered", true)
	require.NoError(t, app.Save(cpu))
	require.NoError(t, deactivateAlerts(app, system1.Id, true))

	reloaded, err = app.FindRecordById("alerts", status.Id)
	require.NoError(t, err)
	assert.True(t, reloaded.GetBool("triggered"),
		"Status alert must be preserved when preserveStatusAlert is set")

	reloaded, err = app.FindRecordById("alerts", cpu.Id)
	require.NoError(t, err)
	assert.False(t, reloaded.GetBool("triggered"),
		"non-Status alerts must still be deactivated when Status is preserved")

	// The NetworkMonitorLoss exclusion: a triggered loss alert is not reset
	// here (a missing observation does not establish recovery).
	loss := addAlert(system1, "NetworkMonitorLoss", true)
	require.NoError(t, deactivateAlerts(app, system1.Id, false))
	reloaded, err = app.FindRecordById("alerts", loss.Id)
	require.NoError(t, err)
	assert.True(t, reloaded.GetBool("triggered"),
		"NetworkMonitorLoss alerts must be excluded from deactivation")
}
