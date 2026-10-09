//go:build testing

package records_test

import (
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/records"
	"github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLongerRecordsPreventDuplicates(t *testing.T) {
	for _, collection := range []string{"system_stats", "container_stats", "custom_stats", "network_monitor_stats"} {
		for _, tier := range []struct {
			shorter, longer string
			count           int
		}{
			{"10m", "20m", 2},
			{"20m", "120m", 6},
			{"120m", "480m", 4},
		} {
			t.Run(collection+"/"+tier.longer, func(t *testing.T) {
				hub, err := tests.NewTestHub(t.TempDir())
				require.NoError(t, err)
				defer hub.Cleanup()

				user, err := tests.CreateUser(hub, "rollup@example.com", "testtesttest")
				require.NoError(t, err)
				sys, err := tests.CreateRecord(hub, "systems", map[string]any{
					"name": "rollup-system", "host": "localhost", "port": "45876",
					"status": "up", "users": []string{user.Id},
				})
				require.NoError(t, err)

				created := time.Now().UTC().Add(-time.Minute)
				data := map[string]any{
					"system": sys.Id, "type": tier.shorter,
					"created": created.Format(types.DefaultDateLayout),
				}
				filter := dbx.HashExp{"system": sys.Id, "type": tier.longer}
				switch collection {
				case "system_stats":
					data["stats"] = `{"cpu":10}`
				case "container_stats":
					data["stats"] = `[{"name":"test","cpu":10}]`
				case "custom_stats":
					data["stats"] = `{"power_watts":10}`
				case "network_monitor_stats":
					monitor, err := tests.CreateRecord(hub, "network_monitors", map[string]any{
						"system": sys.Id, "target": "1.1.1.1", "protocol": "icmp",
						"interval": 30, "enabled": true,
					})
					require.NoError(t, err)
					data["monitor"] = monitor.Id
					data["created"] = created.UnixMilli()
					data["total_count"] = 1
					data["success_count"] = 1
					data["res_sum"] = 10
					data["res_min"] = 10
					data["res_max"] = 10
					filter["monitor"] = monitor.Id
				}
				for range tier.count {
					_, err := tests.CreateRecord(hub, collection, data)
					require.NoError(t, err)
				}

				rm := records.NewRecordManager(hub)
				rm.CreateLongerRecords()
				first, err := hub.FindAllRecords(collection, filter)
				require.NoError(t, err)
				require.Len(t, first, 1)

				// The shorter records remain eligible, but the existing longer
				// record must prevent another rollup on a subsequent invocation.
				rm.CreateLongerRecords()
				second, err := hub.FindAllRecords(collection, filter)
				require.NoError(t, err)
				require.Len(t, second, 1)
				require.Equal(t, first[0].Id, second[0].Id)
			})
		}
	}
}

// custom_stats has rows only for minutes with values, so a 10m record averages
// whatever rows the window has, per key, where system_stats needs 9 of 10. A
// window without rows gets no record.
func TestCustomStatsRollupAveragesAnyRows(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	user, err := tests.CreateUser(hub, "custom-rollup@example.com", "testtesttest")
	require.NoError(t, err)
	newSystem := func(name string) string {
		t.Helper()
		sys, err := tests.CreateRecord(hub, "systems", map[string]any{
			"name": name, "host": "localhost", "port": "45876",
			"status": "up", "users": []string{user.Id},
		})
		require.NoError(t, err)
		return sys.Id
	}
	withValues := newSystem("with-values")
	withoutValues := newSystem("without-values")

	now := time.Now().UTC()
	for i, values := range []string{`{"a":1}`, `{"a":3,"b":5}`, `{"b":7}`} {
		created := now.Add(-time.Duration(i+1) * time.Minute).Format(types.DefaultDateLayout)
		for collection, stats := range map[string]string{"custom_stats": values, "system_stats": `{"cpu":10}`} {
			_, err := tests.CreateRecord(hub, collection, map[string]any{
				"system": withValues, "type": "1m", "stats": stats, "created": created,
			})
			require.NoError(t, err)
		}
	}

	records.NewRecordManager(hub).CreateLongerRecords()

	rollups, err := hub.FindAllRecords("custom_stats", dbx.HashExp{"system": withValues, "type": "10m"})
	require.NoError(t, err)
	require.Len(t, rollups, 1, "three rows are enough")
	var averages map[string]float64
	require.NoError(t, rollups[0].UnmarshalJSONField("stats", &averages))
	assert.Equal(t, map[string]float64{"a": 2, "b": 6}, averages, "each key over the rows it appears in")

	systemRollups, err := hub.FindAllRecords("system_stats", dbx.HashExp{"system": withValues, "type": "10m"})
	require.NoError(t, err)
	assert.Empty(t, systemRollups, "system_stats keeps its 9-of-10 rule")

	none, err := hub.FindAllRecords("custom_stats", dbx.HashExp{"system": withoutValues})
	require.NoError(t, err)
	assert.Empty(t, none, "no rows, no rollup")
}
