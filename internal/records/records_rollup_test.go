//go:build testing

package records_test

import (
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/records"
	"github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/stretchr/testify/require"
)

func TestLongerRecordsPreventDuplicates(t *testing.T) {
	for _, collection := range []string{"system_stats", "container_stats", "network_monitor_stats"} {
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
