//go:build testing

package systems

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/subscriptions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetworkMonitorResultOwnership(t *testing.T) {
	for _, realtime := range []bool{false, true} {
		t.Run(fmt.Sprintf("realtime=%t", realtime), func(t *testing.T) {
			sys, app := newTestSystemWithHub(t)
			if realtime {
				client := subscriptions.NewDefaultClient()
				client.Subscribe("network_monitors/*")
				app.SubscriptionsBroker().Register(client)
				t.Cleanup(func() { app.SubscriptionsBroker().Unregister(client.Id()) })
			}
			systems, err := app.FindCachedCollectionByNameOrId("systems")
			require.NoError(t, err)
			foreignSystem := core.NewRecord(systems)
			require.NoError(t, app.SaveNoValidate(foreignSystem))
			other := &System{Id: foreignSystem.Id, manager: sys.manager}
			collection, err := app.FindCachedCollectionByNameOrId("network_monitors")
			require.NoError(t, err)
			for _, cfg := range []struct {
				id, system string
				enabled    bool
			}{
				{"owned", sys.Id, true},
				{"disabled", sys.Id, false},
				{"foreign", other.Id, true},
			} {
				record := core.NewRecord(collection)
				record.Id = cfg.id
				record.Load(map[string]any{"system": cfg.system, "enabled": cfg.enabled})
				require.NoError(t, app.SaveNoValidate(record))
			}
			// Establish a legitimate result and history for the other system.
			_, err = other.createRecords(&system.CombinedData{Monitors: map[string]monitor.Result{
				"foreign": {LastProbeAt: 1000, AvgResponse: 77, TotalCount: 1, SuccessCount: 1, ResponseSum: 77},
			}})
			require.NoError(t, err)
			snapshot := func(id string) []byte {
				t.Helper()
				record, err := app.FindRecordById("network_monitors", id)
				require.NoError(t, err)
				data, err := json.Marshal(record)
				require.NoError(t, err)
				return data
			}
			before := snapshot("foreign")
			result := monitor.Result{
				LastProbeAt: 2000, AvgResponse: 22, AvgResponse1h: 33,
				MinResponse1h: 11, MaxResponse1h: 44, PacketLoss1h: 50,
				TotalCount: 2, SuccessCount: 1, ResponseSum: 22,
				Cert: &monitor.CertInfo{Expires: 1_800_000_000_000, Issuer: "Test CA"},
			}
			data := &system.CombinedData{Monitors: map[string]monitor.Result{
				"owned": result, "disabled": result, "foreign": result, "nonexistent": result,
			}}
			for range 2 { // Repeated results must remain deduplicated.
				_, err = sys.createRecords(data)
				require.NoError(t, err)
			}
			assert.JSONEq(t, string(before), string(snapshot("foreign")))
			assert.Equal(t, map[string]int64{"owned": 2000, "disabled": 2000}, sys.lastSavedMonitorProbe)
			assert.Len(t, data.Monitors, 4, "ingestion must not mutate the shared telemetry payload")
			for _, id := range []string{"owned", "disabled"} {
				record, err := app.FindRecordById("network_monitors", id)
				require.NoError(t, err)
				assert.EqualValues(t, result.AvgResponse, record.GetInt("res"))
				assert.EqualValues(t, result.AvgResponse1h, record.GetInt("resAvg1h"))
				var cert monitor.CertInfo
				require.NoError(t, record.UnmarshalJSONField("certInfo", &cert))
				assert.Equal(t, *result.Cert, cert)
			}
			stats, err := app.FindAllRecords("network_monitor_stats")
			require.NoError(t, err)
			assert.Len(t, stats, 3)
			for _, stat := range stats {
				record, err := app.FindRecordById("network_monitors", stat.GetString("monitor"))
				require.NoError(t, err)
				assert.Equal(t, record.GetString("system"), stat.GetString("system"))
				if record.Id == "foreign" {
					assert.Equal(t, 77, stat.GetInt("res_sum"))
				} else {
					assert.Equal(t, 22, stat.GetInt("res_sum"))
				}
			}

			// Ownership must be read afresh, even for IDs with saved probe markers.
			moved, err := app.FindRecordById("network_monitors", "owned")
			require.NoError(t, err)
			moved.Set("system", other.Id)
			require.NoError(t, app.SaveNoValidate(moved))
			movedBefore := snapshot("owned")
			for id, result := range data.Monitors {
				result.LastProbeAt = 3000
				result.AvgResponse = 999
				data.Monitors[id] = result
			}
			_, err = sys.createRecords(data)
			require.NoError(t, err)
			assert.JSONEq(t, string(movedBefore), string(snapshot("owned")))
			assert.JSONEq(t, string(before), string(snapshot("foreign")))
			count, err := app.CountRecords("network_monitor_stats")
			require.NoError(t, err)
			assert.EqualValues(t, 4, count, "only the still-owned disabled monitor gets another sample")
			assert.Equal(t, map[string]int64{"owned": 2000, "disabled": 3000}, sys.lastSavedMonitorProbe)
		})
	}
}

func TestNetworkMonitorOwnershipQueryFailure(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	_, err := app.DB().NewQuery("DROP TABLE network_monitors").Execute()
	require.NoError(t, err)
	_, err = sys.createRecords(&system.CombinedData{Monitors: map[string]monitor.Result{
		"missing": {LastProbeAt: 1000},
	}})
	require.Error(t, err)
	count, err := app.CountRecords("system_stats")
	require.NoError(t, err)
	assert.Zero(t, count, "an ownership lookup failure must roll back the transaction")
	assert.Empty(t, sys.lastSavedMonitorProbe)
}

func TestNetworkMonitorProbePruning(t *testing.T) {
	for _, tc := range []struct {
		name     string
		monitors map[string]monitor.Result
		fail     bool
		want     map[string]int64
	}{
		{"nil report", nil, false, map[string]int64{"monitor1": 1000, "monitor2": 1000}},
		{"empty report", map[string]monitor.Result{}, false, map[string]int64{}},
		{"removed monitor", map[string]monitor.Result{"monitor1": {LastProbeAt: 1000}}, false, map[string]int64{"monitor1": 1000}},
		{"rolled back report", map[string]monitor.Result{}, true, map[string]int64{"monitor1": 1000, "monitor2": 1000}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sys, app := newTestSystemWithHub(t)
			sys.lastSavedMonitorProbe = map[string]int64{"monitor1": 1000, "monitor2": 1000}
			// Preserve the distinction between nil and empty across the agent transport.
			encoded, err := cbor.Marshal(system.CombinedData{Monitors: tc.monitors})
			require.NoError(t, err)
			var data system.CombinedData
			require.NoError(t, cbor.Unmarshal(encoded, &data))
			if tc.fail {
				_, err = app.DB().NewQuery(`CREATE TRIGGER fail_system_update BEFORE UPDATE ON systems BEGIN SELECT RAISE(ABORT, 'test rollback'); END`).Execute()
				require.NoError(t, err)
			}
			_, err = sys.createRecords(&data)
			if tc.fail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.want, sys.lastSavedMonitorProbe)
		})
	}
}

func TestNetworkMonitorStatsFreshness(t *testing.T) {
	for _, realtime := range []bool{false, true} {
		name := "sql"
		if realtime {
			name = "realtime"
		}
		t.Run(name, func(t *testing.T) {
			sys, app := newTestSystemWithHub(t)
			if realtime {
				client := subscriptions.NewDefaultClient()
				client.Subscribe("network_monitors/*")
				app.SubscriptionsBroker().Register(client)
				t.Cleanup(func() { app.SubscriptionsBroker().Unregister(client.Id()) })
			}
			col, err := app.FindCachedCollectionByNameOrId("network_monitors")
			require.NoError(t, err)
			for _, id := range []string{"monitor1", "monitor2"} {
				record := core.NewRecord(col)
				record.Id = id
				record.Set("system", sys.Id)
				require.NoError(t, app.SaveNoValidate(record))
			}
			data := &system.CombinedData{Monitors: map[string]monitor.Result{
				"monitor1": {LastProbeAt: 1000, AvgResponse: 20, TotalCount: 6, SuccessCount: 6, ResponseSum: 123},
				"monitor2": {LastProbeAt: 1000, PacketLoss: 100, TotalCount: 1},
			}}
			count := func(want int64) {
				t.Helper()
				got, err := app.CountRecords("network_monitor_stats")
				require.NoError(t, err)
				assert.Equal(t, want, got)
			}
			save := func() {
				t.Helper()
				_, err := sys.createRecords(data)
				require.NoError(t, err)
			}
			save()
			count(2)
			stored, err := app.FindAllRecords("network_monitor_stats")
			require.NoError(t, err)
			for _, record := range stored {
				result := data.Monitors[record.GetString("monitor")]
				assert.EqualValues(t, result.TotalCount, record.GetInt("total_count"))
				assert.EqualValues(t, result.SuccessCount, record.GetInt("success_count"))
				assert.EqualValues(t, result.ResponseSum, record.GetInt("res_sum"))
			}
			// A resume can overlap the scheduled update with the same probe.
			errs := make(chan error, 4)
			for range 4 {
				go func() {
					_, err := sys.createRecords(data)
					errs <- err
				}()
			}
			for range 4 {
				require.NoError(t, <-errs)
			}
			count(2)

			// A rolling hourly value can change without a new probe.
			result := data.Monitors["monitor1"]
			result.AvgResponse1h = 42
			data.Monitors["monitor1"] = result
			save()
			count(2)
			record, err := app.FindRecordById("network_monitors", "monitor1")
			require.NoError(t, err)
			assert.Equal(t, 42, record.GetInt("resAvg1h"))

			// Identical response values and failed probes still count as new measurements.
			for id, result := range data.Monitors {
				result.LastProbeAt = 301000
				data.Monitors[id] = result
			}
			save()
			count(4)

			// A failed individual insert must remain retryable, even if others commit.
			_, err = app.DB().NewQuery(`CREATE TRIGGER fail_monitor_insert BEFORE INSERT ON network_monitor_stats WHEN NEW.monitor = 'monitor1' BEGIN SELECT RAISE(ABORT, 'test insert failure'); END`).Execute()
			require.NoError(t, err)
			for id, result := range data.Monitors {
				result.LastProbeAt = 601000
				data.Monitors[id] = result
			}
			save()
			count(5)
			assert.Equal(t, int64(301000), sys.lastSavedMonitorProbe["monitor1"])
			assert.Equal(t, int64(601000), sys.lastSavedMonitorProbe["monitor2"])
			_, err = app.DB().NewQuery("DROP TRIGGER fail_monitor_insert").Execute()
			require.NoError(t, err)
			save()
			count(6)

			// Failure after inserting stats rolls back the whole transaction and its markers.
			_, err = app.DB().NewQuery(`CREATE TRIGGER fail_system_update BEFORE UPDATE ON systems BEGIN SELECT RAISE(ABORT, 'test rollback'); END`).Execute()
			require.NoError(t, err)
			result = data.Monitors["monitor1"]
			result.LastProbeAt = 901000
			data.Monitors["monitor1"] = result
			_, err = sys.createRecords(data)
			require.Error(t, err)
			count(6)
			assert.Equal(t, int64(601000), sys.lastSavedMonitorProbe["monitor1"])
			_, err = app.DB().NewQuery("DROP TRIGGER fail_system_update").Execute()
			require.NoError(t, err)
			save()
			count(7)

			// Clock rollback is a new probe identity, not a reason to stall writes.
			result.LastProbeAt = 500
			data.Monitors["monitor1"] = result
			save()
			count(8)

			// Recreated systems intentionally accept the first result without restoring state.
			sys = &System{Id: sys.Id, manager: sys.manager}
			save()
			count(10)
		})
	}
}

// Observes the committed DB through the hub, not the transaction's app.
type monitorAlertHub struct {
	stubHub
	handle func(*core.Record, map[string]monitor.Result) error
}

func (h monitorAlertHub) HandleNetworkMonitorAlerts(record *core.Record, results map[string]monitor.Result) error {
	return h.handle(record, results)
}

func TestNetworkMonitorAlertsAfterCommit(t *testing.T) {
	for _, realtime := range []bool{false, true} {
		t.Run(fmt.Sprint(realtime), func(t *testing.T) {
			sys, app := newTestSystemWithHub(t)
			if realtime {
				client := subscriptions.NewDefaultClient()
				client.Subscribe("network_monitors/*")
				app.SubscriptionsBroker().Register(client)
			}
			collection, err := app.FindCachedCollectionByNameOrId("network_monitors")
			require.NoError(t, err)
			record := core.NewRecord(collection)
			record.Set("system", sys.Id)
			require.NoError(t, app.SaveNoValidate(record))
			called := 0
			result := monitor.Result{LastProbeAt: time.Now().UnixMilli(), SampleCount: 3, PacketLoss1h: 10}
			sys.manager.hub = monitorAlertHub{stubHub: stubHub{app}, handle: func(systemRecord *core.Record, results map[string]monitor.Result) error {
				called++
				assert.Equal(t, sys.Id, systemRecord.Id)
				assert.Equal(t, result, results[record.Id])
				saved, err := app.FindRecordById("network_monitors", record.Id)
				require.NoError(t, err)
				assert.Equal(t, 10.0, saved.GetFloat("loss1h"))
				return nil
			}}
			data := &system.CombinedData{Monitors: map[string]monitor.Result{record.Id: result}}
			_, err = sys.createRecords(data)
			require.NoError(t, err)
			assert.Equal(t, 1, called)
			// A transaction that fails after writing monitor stats must not notify.
			_, err = app.DB().NewQuery(`CREATE TRIGGER fail_system BEFORE UPDATE ON systems BEGIN SELECT RAISE(ABORT, 'test rollback'); END`).Execute()
			require.NoError(t, err)
			_, err = sys.createRecords(data)
			require.Error(t, err)
			assert.Equal(t, 1, called)
		})
	}
}

func TestNetworkMonitorCertPersistence(t *testing.T) {
	for _, realtime := range []bool{false, true} {
		name := "sql"
		if realtime {
			name = "realtime"
		}
		t.Run(name, func(t *testing.T) {
			sys, app := newTestSystemWithHub(t)
			if realtime {
				client := subscriptions.NewDefaultClient()
				client.Subscribe("network_monitors/*")
				app.SubscriptionsBroker().Register(client)
				t.Cleanup(func() { app.SubscriptionsBroker().Unregister(client.Id()) })
			}
			col, err := app.FindCachedCollectionByNameOrId("network_monitors")
			require.NoError(t, err)
			record := core.NewRecord(col)
			record.Id = "monitor1"
			record.Set("system", sys.Id)
			require.NoError(t, app.SaveNoValidate(record))

			storedCert := func() monitor.CertInfo {
				t.Helper()
				record, err := app.FindRecordById("network_monitors", "monitor1")
				require.NoError(t, err)
				var cert monitor.CertInfo
				require.NoError(t, record.UnmarshalJSONField("certInfo", &cert))
				return cert
			}
			cert := &monitor.CertInfo{Expires: 1_800_000_000_000, Issuer: "Test CA"}
			_, err = sys.createRecords(&system.CombinedData{Monitors: map[string]monitor.Result{
				"monitor1": {LastProbeAt: 1000, Cert: cert},
			}})
			require.NoError(t, err)
			assert.Equal(t, *cert, storedCert())

			// Results without cert info keep the stored certificate.
			_, err = sys.createRecords(&system.CombinedData{Monitors: map[string]monitor.Result{
				"monitor1": {LastProbeAt: 2000},
			}})
			require.NoError(t, err)
			assert.Equal(t, *cert, storedCert())
		})
	}
}
