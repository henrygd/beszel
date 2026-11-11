//go:build testing

package records_test

import (
	"encoding/json"
	"fmt"
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

// TestLongerRecordsExcludePreviousInterval verifies that a shorter record saved
// just after the cutoff still belongs to the previous interval and must not be
// averaged into the rollup. See https://github.com/henrygd/beszel/issues/2805.
func TestLongerRecordsExcludePreviousInterval(t *testing.T) {
	for _, collection := range []string{"system_stats", "container_stats"} {
		for _, tier := range []struct {
			shorter, longer string
			window          time.Duration
			count           int
		}{
			{"10m", "20m", 20 * time.Minute, 2},
			{"20m", "120m", 120 * time.Minute, 6},
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

				now := time.Now().UTC()
				shorterPeriod := tier.window / time.Duration(tier.count)
				stats := func(cpu int) string {
					if collection == "container_stats" {
						return fmt.Sprintf(`[{"n":"test","c":%d}]`, cpu)
					}
					return fmt.Sprintf(`{"cpu":%d}`, cpu)
				}
				seed := func(created time.Time, cpu int) {
					record, err := tests.CreateRecord(hub, collection, map[string]any{
						"system": sys.Id, "type": tier.shorter, "stats": stats(cpu),
					})
					require.NoError(t, err)
					// created is an autodate field, so we need to set it manually
					record.SetRaw("created", created.Format(types.DefaultDateLayout))
					require.NoError(t, hub.SaveNoValidate(record))
				}
				// records covering the intervals inside the rollup window, each saved
				// slightly after its interval ends like a real rollup write; the last
				// one lands around the job's `now`, like the record the same run just
				// wrote in the previous aggregation tier
				for i := range tier.count {
					seed(now.Add(-tier.window+time.Duration(i+1)*shorterPeriod+400*time.Millisecond), 10)
				}
				// a record saved just after the cutoff still covers the previous
				// interval and must not be included
				seed(now.Add(-tier.window+5*time.Second), 100)

				rm := records.NewRecordManager(hub)
				rm.CreateLongerRecords()

				rollups, err := hub.FindAllRecords(collection, dbx.HashExp{"system": sys.Id, "type": tier.longer})
				require.NoError(t, err)
				require.Len(t, rollups, 1)

				var decoded []struct {
					Cpu float64 `json:"cpu"`
					C   float64 `json:"c"`
				}
				statsJSON := rollups[0].GetString("stats")
				if collection == "system_stats" {
					statsJSON = "[" + statsJSON + "]"
				}
				require.NoError(t, json.Unmarshal([]byte(statsJSON), &decoded))
				for _, d := range decoded {
					assert.Equal(t, 10.0, max(d.Cpu, d.C), "rollup must average only the records covering its interval")
				}
			})
		}
	}
}
