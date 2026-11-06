//go:build testing

package records_test

import (
	"github.com/henrygd/beszel/internal/records"
	"github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestNutPowerRollup(t *testing.T) {
	for _, source := range []string{"measured", "estimated", "unknown", ""} {
		t.Run(source, func(t *testing.T) {
			hub, err := tests.NewTestHub(t.TempDir())
			require.NoError(t, err)
			user, err := tests.CreateUser(hub, "nut@example.com", "testtesttest")
			require.NoError(t, err)
			defer hub.Cleanup()
			sys, err := tests.CreateRecord(hub, "systems", map[string]any{"name": "nut-rollup", "host": "localhost", "status": "up", "users": []string{user.Id}})
			require.NoError(t, err)
			// One valid sample and eight missing samples must not dilute the average.
			for i := 0; i < 9; i++ {
				sampleSource, power := "unknown", 0.0
				if i == 0 {
					sampleSource, power = source, 90
				}
				_, err = tests.CreateRecord(hub, "nut_stats", map[string]any{
					"system": sys.Id, "device": "testups00000001", "type": "1m",
					"created":      time.Now().UTC().Add(-time.Minute).Format(types.DefaultDateLayout),
					"output_power": power, "output_power_source": sampleSource,
				})
				require.NoError(t, err)
			}
			records.NewRecordManager(hub).CreateLongerRecords()
			rows, err := hub.FindAllRecords("nut_stats", dbx.HashExp{"system": sys.Id, "type": "10m"})
			require.NoError(t, err)
			require.Len(t, rows, 1)
			if source == "measured" || source == "estimated" {
				require.Equal(t, 90.0, rows[0].GetFloat("output_power"))
				require.Equal(t, source, rows[0].GetString("output_power_source"))
			} else {
				require.Equal(t, "unknown", rows[0].GetString("output_power_source"))
			}
		})
	}
}

func TestNutSparseAggregation(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	user, err := tests.CreateUser(hub, "nut-sparse@example.com", "testtesttest")
	require.NoError(t, err)
	defer hub.Cleanup()
	sys, err := tests.CreateRecord(hub, "systems", map[string]any{"name": "nut-sparse", "host": "localhost", "status": "up", "users": []string{user.Id}})
	require.NoError(t, err)

	// NUT's default polling interval (one hour) exceeds the aggregation window,
	// so a single sample must still advance through every tier.
	_, err = tests.CreateRecord(hub, "nut_stats", map[string]any{
		"system": sys.Id, "device": "testups00000001", "type": "1m",
		"created":        time.Now().UTC().Add(-time.Minute).Format(types.DefaultDateLayout),
		"battery_charge": 65.0,
	})
	require.NoError(t, err)

	records.NewRecordManager(hub).CreateLongerRecords()

	rows, err := hub.FindAllRecords("nut_stats", dbx.HashExp{"system": sys.Id, "type": "10m"})
	require.NoError(t, err)
	require.Len(t, rows, 1, "single sample must aggregate despite the minShorterRecords threshold")
	require.Equal(t, 65.0, rows[0].GetFloat("battery_charge"))
}
