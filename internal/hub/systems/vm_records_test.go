//go:build testing

package systems_test

import (
	"encoding/json"
	"testing"

	"ganigeorgiev.com/pocketbase/dbx"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/entities/vm"
	"github.com/henrygd/beszel/internal/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRecordsPersistsVMStats(t *testing.T) {
	hub, user := tests.GetHubWithUser(t)
	defer hub.Cleanup()
	records, err := tests.CreateSystems(hub, 1, user.Id, "paused")
	require.NoError(t, err)
	monitoredSystem, err := hub.GetSystemManager().GetSystem(records[0].Id)
	require.NoError(t, err)

	_, err = monitoredSystem.CreateRecords(&system.CombinedData{
		VirtualMachines: []*vm.Stats{{
			UUID:  "d5cd3f23-d5f5-4e20-958e-840ff0f37aa9",
			Name:  "emu",
			State: vm.StateRunning,
			CPU:   12.5,
		}},
	})
	require.NoError(t, err)

	var row struct {
		Stats []byte `db:"stats"`
	}
	require.NoError(t, hub.DB().Select("stats").From("vm_stats").
		Where(dbx.HashExp{"system": records[0].Id}).One(&row))
	var stored []vm.Stats
	require.NoError(t, json.Unmarshal(row.Stats, &stored))
	require.Len(t, stored, 1)
	assert.Equal(t, "emu", stored[0].Name)
	assert.Equal(t, float64(12.5), stored[0].CPU)

	_, err = monitoredSystem.CreateRecords(&system.CombinedData{VirtualMachines: []*vm.Stats{}})
	require.NoError(t, err)
	count, err := hub.CountRecords("vm_stats", dbx.HashExp{"system": records[0].Id})
	require.NoError(t, err)
	assert.EqualValues(t, 2, count)
	require.NoError(t, hub.DB().NewQuery(
		"SELECT stats FROM vm_stats WHERE system={:system} ORDER BY rowid DESC LIMIT 1",
	).Bind(dbx.Params{"system": records[0].Id}).One(&row))
	require.NoError(t, json.Unmarshal(row.Stats, &stored))
	assert.Empty(t, stored, "a fresh empty snapshot must clear the final removed VM")
}
