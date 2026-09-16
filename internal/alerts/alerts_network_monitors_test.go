//go:build testing

package alerts_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/alerts"
	"github.com/henrygd/beszel/internal/entities/monitor"
	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	pbTests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func networkAlertSetup(t *testing.T) (*beszelTests.TestHub, *core.Record, *core.Record, []*core.Record) {
	t.Helper()
	hub, system, alert := systemdTestSetup(t, false)
	t.Cleanup(hub.Cleanup)
	alert.Set("name", "NetworkMonitorLoss")
	alert.Set("value", 5)
	require.NoError(t, hub.Save(alert))
	var monitors []*core.Record
	for _, name := range []string{"gateway", "website"} {
		record, err := beszelTests.CreateRecord(hub, "network_monitors", map[string]any{
			"system": system.Id, "target": name + ".example.com", "protocol": "icmp", "interval": 60, "enabled": true,
		})
		require.NoError(t, err)
		monitors = append(monitors, record)
	}
	// Avoid starting a system update worker in tests.
	_, err := hub.DB().Update("systems", dbx.Params{"status": "up"}, dbx.HashExp{"id": system.Id}).Execute()
	require.NoError(t, err)
	system.Set("status", "up")
	return hub, system, alert, monitors
}

func monitorResult(loss float64) monitor.Result {
	return monitor.Result{LastProbeAt: time.Now().UnixMilli(), SampleCount: 60, PacketLoss1h: loss}
}

func TestNetworkMonitorAlertIndependentIncidents(t *testing.T) {
	hub, system, alert, monitors := networkAlertSetup(t)
	am := alerts.NewTestAlertManagerWithoutWorker(hub)
	count := hub.TestMailer.TotalSend()
	results := map[string]monitor.Result{monitors[0].Id: monitorResult(10), monitors[1].Id: monitorResult(0)}
	check := func(active bool, open, sent int) {
		t.Helper()
		record, err := hub.FindRecordById("alerts", alert.Id)
		require.NoError(t, err)
		assert.Equal(t, active, record.GetBool("triggered"))
		total, err := hub.CountRecords("alerts_history", dbx.HashExp{"alert_id": alert.Id, "resolved": ""})
		require.NoError(t, err)
		assert.EqualValues(t, open, total)
		assert.Equal(t, count+sent, hub.TestMailer.TotalSend())
	}
	require.NoError(t, am.HandleNetworkMonitorAlerts(system, results))
	check(true, 1, 1)
	message := hub.TestMailer.Messages()[count]
	assert.Contains(t, message.Text, "gateway.example.com")
	assert.Contains(t, message.Text, "10.00%")
	assert.Contains(t, message.Text, "5.00%")
	require.NoError(t, am.HandleNetworkMonitorAlerts(system, results))
	check(true, 1, 1)
	// Persisted monitor state prevents duplicate notifications after a hub restart.
	am = alerts.NewTestAlertManagerWithoutWorker(hub)
	results[monitors[1].Id] = monitorResult(20)
	require.NoError(t, am.HandleNetworkMonitorAlerts(system, results))
	check(true, 2, 2)
	results[monitors[0].Id] = monitorResult(5) // Equality is a recovery.
	require.NoError(t, am.HandleNetworkMonitorAlerts(system, results))
	check(true, 1, 3)
	results[monitors[1].Id] = monitorResult(0)
	require.NoError(t, am.HandleNetworkMonitorAlerts(system, results))
	check(false, 0, 4)
	histories, err := hub.FindAllRecords("alerts_history", dbx.HashExp{"alert_id": alert.Id})
	require.NoError(t, err)
	require.Len(t, histories, 2)
	for _, history := range histories {
		assert.NotEmpty(t, history.GetString("monitor_name"))
	}
}

func TestNetworkMonitorAlertTargetLabel(t *testing.T) {
	for _, tc := range []struct {
		protocol, target, label string
		port                    int
	}{
		{"icmp", "gateway.example.com", "gateway.example.com", 0},
		{"http", "https://example.com/health", "https://example.com/health", 0},
		{"tcp", "example.com", "example.com:8443", 8443},
		{"tcp", "2001:db8::1", "[2001:db8::1]:443", 443},
	} {
		t.Run(tc.label, func(t *testing.T) {
			hub, system, alert, monitors := networkAlertSetup(t)
			m := monitors[0]
			m.Set("protocol", tc.protocol)
			m.Set("target", tc.target)
			m.Set("port", tc.port)
			require.NoError(t, hub.Save(m))
			am := alerts.NewTestAlertManagerWithoutWorker(hub)
			require.NoError(t, am.HandleNetworkMonitorAlerts(system, map[string]monitor.Result{m.Id: monitorResult(10)}))
			histories, err := hub.FindAllRecords("alerts_history", dbx.HashExp{"alert_id": alert.Id})
			require.NoError(t, err)
			require.Len(t, histories, 1)
			assert.Equal(t, tc.label, histories[0].GetString("monitor_name"))
			assert.Contains(t, hub.TestMailer.Messages()[hub.TestMailer.TotalSend()-1].Text, tc.label)
		})
	}
}

func TestNetworkMonitorAlertIgnoresUnknownResults(t *testing.T) {
	for _, scenario := range []string{"missing", "stale", "warmup", "no probes", "down", "paused", "future", "expired hourly window"} {
		t.Run(scenario, func(t *testing.T) {
			hub, system, alert, monitors := networkAlertSetup(t)
			am := alerts.NewTestAlertManagerWithoutWorker(hub)
			apply := func(loss float64) {
				result := monitorResult(loss)
				results := map[string]monitor.Result{monitors[0].Id: result}
				switch scenario {
				case "missing":
					results = nil
				case "stale":
					result.LastProbeAt = time.Now().Add(-10 * time.Minute).UnixMilli()
					results[monitors[0].Id] = result
				case "expired hourly window":
					monitors[0].Set("interval", 3600)
					require.NoError(t, hub.Save(monitors[0]))
					result.LastProbeAt = time.Now().Add(-2 * time.Hour).UnixMilli()
					results[monitors[0].Id] = result
				case "future":
					result.LastProbeAt = time.Now().Add(time.Hour).UnixMilli()
					results[monitors[0].Id] = result
				case "warmup":
					result.SampleCount = 2
					results[monitors[0].Id] = result
				case "no probes":
					result.SampleCount = 0
					results[monitors[0].Id] = result
				case "down":
					_, err := hub.DB().Update("systems", dbx.Params{"status": scenario}, dbx.HashExp{"id": system.Id}).Execute()
					require.NoError(t, err)
				case "paused":
					record, err := hub.FindRecordById("systems", system.Id)
					require.NoError(t, err)
					record.Set("status", "paused")
					require.NoError(t, hub.Save(record))
				}
				require.NoError(t, am.HandleNetworkMonitorAlerts(system, results))
			}
			count := hub.TestMailer.TotalSend()
			apply(100)
			assert.Equal(t, count, hub.TestMailer.TotalSend())
			_, err := hub.DB().Update("systems", dbx.Params{"status": "up"}, dbx.HashExp{"id": system.Id}).Execute()
			require.NoError(t, err)
			require.NoError(t, am.HandleNetworkMonitorAlerts(system, map[string]monitor.Result{monitors[0].Id: monitorResult(10)}))
			apply(0)
			assert.Equal(t, count+1, hub.TestMailer.TotalSend(), "unknown data must not recover an incident")
			record, err := hub.FindRecordById("alerts", alert.Id)
			require.NoError(t, err)
			assert.True(t, record.GetBool("triggered"))
		})
	}
}

func TestNetworkMonitorAlertCleanup(t *testing.T) {
	for _, scenario := range []string{"disable monitor", "delete monitor", "disable alert", "purge history", "delete system"} {
		t.Run(scenario, func(t *testing.T) {
			hub, system, alert, monitors := networkAlertSetup(t)
			am := alerts.NewTestAlertManagerWithoutWorker(hub)
			results := map[string]monitor.Result{monitors[0].Id: monitorResult(10), monitors[1].Id: monitorResult(20)}
			require.NoError(t, am.HandleNetworkMonitorAlerts(system, results))
			count := hub.TestMailer.TotalSend()
			switch scenario {
			case "disable monitor":
				monitors[0].Set("enabled", false)
				require.NoError(t, hub.Save(monitors[0]))
			case "delete monitor":
				require.NoError(t, hub.Delete(monitors[0]))
			case "disable alert":
				require.NoError(t, hub.Delete(alert))
			case "delete system":
				require.NoError(t, hub.Delete(system))
			case "purge history":
				history, err := hub.FindAllRecords("alerts_history")
				require.NoError(t, err)
				for _, record := range history {
					require.NoError(t, hub.Delete(record))
				}
			}
			require.NoError(t, am.HandleNetworkMonitorAlerts(system, results))
			assert.Equal(t, count, hub.TestMailer.TotalSend())
			open, err := hub.CountRecords("alerts_history", dbx.HashExp{"alert_id": alert.Id, "resolved": ""})
			require.NoError(t, err)
			if scenario == "disable monitor" || scenario == "delete monitor" {
				assert.EqualValues(t, 1, open)
				record, err := hub.FindRecordById("alerts", alert.Id)
				require.NoError(t, err)
				assert.True(t, record.GetBool("triggered"))
				require.NoError(t, hub.Delete(monitors[1]))
				record, err = hub.FindRecordById("alerts", alert.Id)
				require.NoError(t, err)
				assert.False(t, record.GetBool("triggered"))
			} else {
				assert.Zero(t, open)
			}
		})
	}
}

func TestNetworkMonitorAlertPerUserThresholds(t *testing.T) {
	hub, system, alert, monitors := networkAlertSetup(t)
	user, err := beszelTests.CreateUser(hub, "monitor2@example.com", "password")
	require.NoError(t, err)
	other, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{"name": "NetworkMonitorLoss", "system": system.Id, "user": user.Id, "value": 20})
	require.NoError(t, err)
	am := alerts.NewTestAlertManagerWithoutWorker(hub)
	results := map[string]monitor.Result{monitors[0].Id: monitorResult(10)}
	require.NoError(t, am.HandleNetworkMonitorAlerts(system, results))
	other, err = hub.FindRecordById("alerts", other.Id)
	require.NoError(t, err)
	assert.False(t, other.GetBool("triggered"))
	// Editing the threshold re-evaluates on the next batch, without losing state.
	alert, err = hub.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	alert.Set("value", 15)
	require.NoError(t, hub.Save(alert))
	require.NoError(t, am.HandleNetworkMonitorAlerts(system, results))
	alert, err = hub.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	assert.False(t, alert.GetBool("triggered"))
}

func TestNetworkMonitorAlertAPI(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		value                 float64
		direct, denied, patch bool
		status                int
	}{
		{name: "zero threshold", value: 0, status: 200},
		{name: "fractional threshold", value: 5.5, status: 200},
		{name: "negative threshold", value: -1, status: 400},
		{name: "unreachable threshold", value: 100, status: 400},
		{name: "bulk inaccessible system", value: 5, denied: true, status: 200},
		{name: "direct inaccessible system", value: 5, direct: true, denied: true, status: 403},
		{name: "direct invalid threshold", value: -1, direct: true, status: 400},
		{name: "direct private state", value: 5, direct: true, status: 200},
		{name: "patch preserves state", value: 10, direct: true, patch: true, status: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hub, user := beszelTests.GetHubWithUser(t)
			defer hub.Cleanup()
			owner := user.Id
			if tc.denied {
				other, err := beszelTests.CreateUser(hub, "other@example.com", "password")
				require.NoError(t, err)
				owner = other.Id
			}
			systems, err := beszelTests.CreateSystems(hub, 1, owner, "paused")
			require.NoError(t, err)
			token, err := user.NewAuthToken()
			require.NoError(t, err)
			body := map[string]any{"name": "NetworkMonitorLoss", "value": tc.value, "min": 60, "systems": []string{systems[0].Id}, "overwrite": true}
			url, method := "/api/beszel/user-alerts", "POST"
			if tc.direct {
				url = "/api/collections/alerts/records"
				body["system"], body["user"] = systems[0].Id, user.Id
				body["state"], body["triggered"] = map[string]any{"monitors": map[string]string{"fake": "fake"}}, true
			}
			if tc.patch {
				alert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{"name": "NetworkMonitorLoss", "system": systems[0].Id, "user": user.Id, "value": 5, "triggered": true, "state": map[string]any{"monitors": map[string]string{"real": "history"}}})
				require.NoError(t, err)
				url += "/" + alert.Id
				method = "PATCH"
				body["triggered"] = false
			}
			content := `"success":true`
			if tc.direct {
				content = `"name":"NetworkMonitorLoss"`
			}
			if tc.status == 400 {
				content = `"status":400`
			}
			if tc.status == 403 {
				content = `"status":403`
			}
			scenario := beszelTests.ApiScenario{
				Name: tc.name, Method: method, URL: url, Body: jsonReader(body),
				Headers: map[string]string{"Authorization": token}, ExpectedStatus: tc.status, ExpectedContent: []string{content},
				TestAppFactory: func(testing.TB) *pbTests.TestApp { return hub.TestApp },
			}
			scenario.Test(t)
			records, err := hub.FindAllRecords("alerts")
			require.NoError(t, err)
			if tc.status != 200 || tc.denied {
				assert.Empty(t, records)
				return
			}
			require.Len(t, records, 1)
			assert.Equal(t, tc.value, records[0].GetFloat("value"))
			assert.Zero(t, records[0].GetInt("min"))
			state := struct {
				Monitors map[string]string `json:"monitors"`
			}{}
			require.NoError(t, records[0].UnmarshalJSONField("state", &state))
			states := state.Monitors
			if tc.patch {
				assert.Equal(t, map[string]string{"real": "history"}, states)
				assert.True(t, records[0].GetBool("triggered"))
			} else {
				assert.Empty(t, states)
				assert.False(t, records[0].GetBool("triggered"))
			}
		})
	}
}

type monitorCountingHub struct {
	*beszelTests.TestHub
	transactions      atomic.Int64
	beforeTransaction func()
}

func (h *monitorCountingHub) RunInTransaction(fn func(core.App) error) error {
	h.transactions.Add(1)
	if h.beforeTransaction != nil {
		h.beforeTransaction()
	}
	return h.App.RunInTransaction(fn)
}

// Count actual SQL on both DB connections, including queries through record APIs.
func monitorSQLCounter(t *testing.T, app core.App) *atomic.Int64 {
	t.Helper()
	count := &atomic.Int64{}
	for _, builder := range []dbx.Builder{app.ConcurrentDB(), app.NonconcurrentDB()} {
		db := builder.(*dbx.DB)
		old := db.LogFunc
		db.LogFunc = func(string, ...any) { count.Add(1) }
		t.Cleanup(func() { db.LogFunc = old })
	}
	return count
}

func TestNetworkMonitorAlertSteadyStateNoDatabaseWork(t *testing.T) {
	hub, system, alert, monitors := networkAlertSetup(t)
	counted := &monitorCountingHub{TestHub: hub}
	am := alerts.NewTestAlertManagerWithoutWorker(counted)
	sql := monitorSQLCounter(t, hub)
	results := map[string]monitor.Result{monitors[0].Id: monitorResult(0)}
	evaluate := func() { t.Helper(); require.NoError(t, am.HandleNetworkMonitorAlerts(system, results)) }
	noWork := func() {
		t.Helper()
		sql.Store(0)
		counted.transactions.Store(0)
		for range 100 {
			evaluate()
		}
		assert.Zero(t, sql.Load(), "steady state must not issue SQL")
		assert.Zero(t, counted.transactions.Load(), "steady state must not open transactions")
	}
	// One-time lazy loads are permitted, including on hub restart.
	evaluate()
	assert.Positive(t, sql.Load())
	noWork()
	// Realtime metric saves invoke record hooks but must not invalidate config.
	fresh, err := hub.FindRecordById("network_monitors", monitors[0].Id)
	require.NoError(t, err)
	monitors[0] = fresh
	monitors[0].Set("loss1h", 0)
	monitors[0].Set("res", 100)
	require.NoError(t, hub.Save(monitors[0]))
	noWork()
	results[monitors[0].Id] = monitorResult(10)
	evaluate()
	assert.Positive(t, sql.Load(), "transitions must still be persisted")
	assert.EqualValues(t, 1, counted.transactions.Load())
	noWork()
	// Missing and stale observations must not enter the transaction either.
	results = nil
	noWork()
	results = map[string]monitor.Result{monitors[0].Id: {SampleCount: 60, LastProbeAt: time.Now().Add(-10 * time.Minute).UnixMilli()}}
	noWork()
	results[monitors[0].Id] = monitorResult(0)
	evaluate()
	noWork()
	require.NoError(t, hub.Delete(alert))
	noWork()
}

func TestNetworkMonitorAlertConfigCacheInvalidation(t *testing.T) {
	hub, system, alert, monitors := networkAlertSetup(t)
	am := alerts.NewTestAlertManagerWithoutWorker(hub)
	results := map[string]monitor.Result{monitors[0].Id: monitorResult(0)}
	require.NoError(t, am.HandleNetworkMonitorAlerts(system, results))
	count := hub.TestMailer.TotalSend()
	// Widening the interval makes this observation fresh. A stale interval cache
	// would miss the failure indefinitely, even though results keep arriving.
	result := monitorResult(10)
	result.LastProbeAt = time.Now().Add(-4 * time.Minute).UnixMilli()
	results[monitors[0].Id] = result
	require.NoError(t, am.HandleNetworkMonitorAlerts(system, results))
	assert.Equal(t, count, hub.TestMailer.TotalSend())
	monitors[0].Set("interval", 120)
	require.NoError(t, hub.Save(monitors[0]))
	require.NoError(t, am.HandleNetworkMonitorAlerts(system, results))
	assert.Equal(t, count+1, hub.TestMailer.TotalSend())
	// Disable, then re-enable the same ID: its new failure must be detected.
	monitors[0].Set("enabled", false)
	require.NoError(t, hub.Save(monitors[0]))
	require.NoError(t, am.HandleNetworkMonitorAlerts(system, results))
	monitors[0].Set("enabled", true)
	require.NoError(t, hub.Save(monitors[0]))
	require.NoError(t, am.HandleNetworkMonitorAlerts(system, results))
	assert.Equal(t, count+2, hub.TestMailer.TotalSend())
	// A new monitor must also become eligible without restarting the hub.
	created, err := beszelTests.CreateRecord(hub, "network_monitors", map[string]any{
		"system": system.Id, "name": "new", "target": "new.example.com", "protocol": "icmp", "interval": 60, "enabled": true,
	})
	require.NoError(t, err)
	results[created.Id] = monitorResult(10)
	require.NoError(t, am.HandleNetworkMonitorAlerts(system, results))
	assert.Equal(t, count+3, hub.TestMailer.TotalSend())
	// Threshold changes refresh cached config and preserve the active incidents.
	alert, err = hub.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	alert.Set("value", 15)
	require.NoError(t, hub.Save(alert))
	require.NoError(t, am.HandleNetworkMonitorAlerts(system, results))
	assert.Equal(t, count+5, hub.TestMailer.TotalSend())
}

func TestNetworkMonitorAlertRevalidatesCandidate(t *testing.T) {
	for _, change := range []string{"threshold", "disable alert", "disable monitor", "down"} {
		t.Run(change, func(t *testing.T) {
			hub, system, alert, monitors := networkAlertSetup(t)
			counted := &monitorCountingHub{TestHub: hub}
			am := alerts.NewTestAlertManagerWithoutWorker(counted)
			results := map[string]monitor.Result{monitors[0].Id: monitorResult(0)}
			require.NoError(t, am.HandleNetworkMonitorAlerts(system, results))
			count := hub.TestMailer.TotalSend()
			// Change the DB after the cache predicts a transition, before its transaction.
			counted.beforeTransaction = func() {
				counted.beforeTransaction = nil
				switch change {
				case "threshold":
					alert.Set("value", 20)
					require.NoError(t, hub.Save(alert))
				case "disable alert":
					require.NoError(t, hub.Delete(alert))
				case "disable monitor":
					monitors[0].Set("enabled", false)
					require.NoError(t, hub.Save(monitors[0]))
				case "down":
					_, err := hub.DB().Update("systems", dbx.Params{"status": "down"}, dbx.HashExp{"id": system.Id}).Execute()
					require.NoError(t, err)
				}
			}
			results[monitors[0].Id] = monitorResult(10)
			require.NoError(t, am.HandleNetworkMonitorAlerts(system, results))
			assert.EqualValues(t, 1, counted.transactions.Load())
			assert.Equal(t, count, hub.TestMailer.TotalSend())
			histories, err := hub.CountRecords("alerts_history")
			require.NoError(t, err)
			assert.Zero(t, histories)
		})
	}
}

func TestNetworkMonitorAlertConcurrentEvaluations(t *testing.T) {
	hub, system, _, monitors := networkAlertSetup(t)
	am := alerts.NewTestAlertManagerWithoutWorker(hub)
	results := map[string]monitor.Result{monitors[0].Id: monitorResult(10)}
	count := hub.TestMailer.TotalSend()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() { errs <- am.HandleNetworkMonitorAlerts(system, results) })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	assert.Equal(t, count+1, hub.TestMailer.TotalSend())
}

func TestNetworkMonitorAlertCacheAfterRollback(t *testing.T) {
	hub, system, _, monitors := networkAlertSetup(t)
	am := alerts.NewTestAlertManagerWithoutWorker(hub)
	results := map[string]monitor.Result{monitors[0].Id: monitorResult(0)}
	require.NoError(t, am.HandleNetworkMonitorAlerts(system, results))
	_, err := hub.DB().NewQuery(`CREATE TRIGGER fail_alert BEFORE UPDATE ON alerts BEGIN SELECT RAISE(ABORT, 'test rollback'); END`).Execute()
	require.NoError(t, err)
	count := hub.TestMailer.TotalSend()
	results[monitors[0].Id] = monitorResult(10)
	require.Error(t, am.HandleNetworkMonitorAlerts(system, results))
	assert.Equal(t, count, hub.TestMailer.TotalSend())
	histories, err := hub.CountRecords("alerts_history")
	require.NoError(t, err)
	assert.Zero(t, histories)
	_, err = hub.DB().NewQuery("DROP TRIGGER fail_alert").Execute()
	require.NoError(t, err)
	// A failed transition must not be published to the cache and mask the retry.
	require.NoError(t, am.HandleNetworkMonitorAlerts(system, results))
	assert.Equal(t, count+1, hub.TestMailer.TotalSend())
}
