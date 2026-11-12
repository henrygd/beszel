//go:build testing

package systems

import (
	"testing"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/henrygd/beszel/internal/entities/smart"
	"github.com/henrygd/beszel/internal/entities/systemd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Record IDs are derived from agent-supplied values, so one system must never be
// able to take over or overwrite a record that belongs to another system.

func newOtherSystem(t *testing.T, app core.App) string {
	t.Helper()
	col, err := app.FindCachedCollectionByNameOrId("systems")
	require.NoError(t, err)
	rec := core.NewRecord(col)
	rec.Set("name", "other-system")
	rec.Set("host", "127.0.0.2")
	rec.Set("port", "45876")
	require.NoError(t, app.SaveNoValidate(rec))
	return rec.Id
}

func TestCreateContainerRecordsDoesNotTakeOverOtherSystemsRecord(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	other := newOtherSystem(t, app)

	require.NoError(t, createContainerRecords(app, []*container.Stats{{Id: "shared-id", Name: "victim"}}, other))
	require.NoError(t, createContainerRecords(app, []*container.Stats{{Id: "shared-id", Name: "attacker"}}, sys.Id))

	var rec struct {
		System string `db:"system"`
		Name   string `db:"name"`
	}
	require.NoError(t, app.DB().Select("system", "name").From("containers").
		Where(dbx.HashExp{"id": "shared-id"}).One(&rec))
	assert.Equal(t, other, rec.System)
	assert.Equal(t, "victim", rec.Name)
}

func TestCreateSystemdStatsRecordsDoesNotTakeOverOtherSystemsRecord(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	other := newOtherSystem(t, app)

	// Plant a record owned by another system at the ID this system would compute.
	id := MakeStableHashId(sys.Id, "a.service")
	_, err := app.DB().Insert("systemd_services", dbx.Params{
		"id": id, "system": other, "name": "victim.service", "updated": 1,
	}).Execute()
	require.NoError(t, err)

	require.NoError(t, createSystemdStatsRecords(app, []*systemd.Service{{Name: "a.service"}}, sys.Id))

	var rec struct {
		System string `db:"system"`
		Name   string `db:"name"`
	}
	require.NoError(t, app.DB().Select("system", "name").From("systemd_services").
		Where(dbx.HashExp{"id": id}).One(&rec))
	assert.Equal(t, other, rec.System)
	assert.Equal(t, "victim.service", rec.Name)
}

func TestSaveSmartDevicesDoesNotTakeOverOtherSystemsRecord(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	other := newOtherSystem(t, app)

	col, err := app.FindCachedCollectionByNameOrId("smart_devices")
	require.NoError(t, err)
	id := MakeStableHashId(sys.Id, "AAA")
	planted := core.NewRecord(col)
	planted.Set("id", id)
	planted.Set("system", other)
	planted.Set("name", "victim-disk")
	require.NoError(t, app.SaveNoValidate(planted))

	require.NoError(t, sys.saveSmartDevices(map[string]smart.SmartData{
		"AAA": {SerialNumber: "AAA", DiskName: "attacker-disk"},
	}, false))

	rec, err := app.FindRecordById("smart_devices", id)
	require.NoError(t, err)
	assert.Equal(t, other, rec.GetString("system"))
	assert.Equal(t, "victim-disk", rec.GetString("name"))
}
