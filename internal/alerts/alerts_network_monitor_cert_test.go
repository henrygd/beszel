//go:build testing

package alerts_test

import (
	"strconv"
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

// certAlertSetup creates a 30 day certificate alert and an HTTPS monitor without certificate info.
func certAlertSetup(t *testing.T) (*beszelTests.TestHub, *core.Record, *core.Record, *core.Record) {
	t.Helper()
	hub, system, alert := systemdTestSetup(t, false)
	t.Cleanup(hub.Cleanup)
	alert.Set("name", "NetworkMonitorCert")
	alert.Set("value", 30)
	require.NoError(t, hub.Save(alert))
	m, err := beszelTests.CreateRecord(hub, "network_monitors", map[string]any{
		"system": system.Id, "target": "https://example.com", "protocol": "http", "interval": 60, "enabled": true,
	})
	require.NoError(t, err)
	// Avoid starting a system update worker in tests.
	_, err = hub.DB().Update("systems", dbx.Params{"status": "up"}, dbx.HashExp{"id": system.Id}).Execute()
	require.NoError(t, err)
	system.Set("status", "up")
	return hub, system, alert, m
}

// certExpiringIn returns certificate info for a certificate with the given days
// left, plus an hour so that the day count is stable during the test.
func certExpiringIn(days int) *monitor.CertInfo {
	return &monitor.CertInfo{Expires: time.Now().Add(time.Duration(days)*24*time.Hour + time.Hour).UnixMilli()}
}

func setMonitorCert(t *testing.T, hub *beszelTests.TestHub, m *core.Record, cert *monitor.CertInfo) {
	t.Helper()
	m.Set("certInfo", cert)
	require.NoError(t, hub.Save(m))
}

func TestNetworkMonitorCertAlertLifecycle(t *testing.T) {
	hub, system, alert, m := certAlertSetup(t)
	am := alerts.NewTestAlertManagerWithoutWorker(hub)
	count := hub.TestMailer.TotalSend()
	check := func(active bool, open, sent int) {
		t.Helper()
		require.NoError(t, am.HandleNetworkMonitorAlerts(system, nil))
		record, err := hub.FindRecordById("alerts", alert.Id)
		require.NoError(t, err)
		assert.Equal(t, active, record.GetBool("triggered"))
		total, err := hub.CountRecords("alerts_history", dbx.HashExp{"alert_id": alert.Id, "resolved": ""})
		require.NoError(t, err)
		assert.EqualValues(t, open, total)
		assert.Equal(t, count+sent, hub.TestMailer.TotalSend())
	}
	lastMessage := func() string {
		return hub.TestMailer.Messages()[hub.TestMailer.TotalSend()-1].Text
	}
	// Unknown expiry never triggers.
	check(false, 0, 0)
	setMonitorCert(t, hub, m, certExpiringIn(40))
	check(false, 0, 0)
	// Expiry is checked against the clock, without needing a new result.
	cert := certExpiringIn(25)
	setMonitorCert(t, hub, m, cert)
	check(true, 1, 1)
	assert.Contains(t, lastMessage(), "https://example.com")
	assert.Contains(t, lastMessage(), "expires in 25 days ("+time.UnixMilli(cert.Expires).UTC().Format(time.DateOnly)+")")
	assert.Contains(t, lastMessage(), "within the 30-day threshold")
	// Repeated checks and a hub restart do not notify again.
	check(true, 1, 1)
	am = alerts.NewTestAlertManagerWithoutWorker(hub)
	check(true, 1, 1)
	// A renewed certificate resolves the incident.
	setMonitorCert(t, hub, m, certExpiringIn(90))
	check(false, 0, 2)
	assert.Contains(t, lastMessage(), "expires in 90 days")
	assert.Contains(t, hub.TestMailer.Messages()[hub.TestMailer.TotalSend()-1].Subject, "Certificate expiry recovered")
	assert.Contains(t, lastMessage(), "beyond the 30-day threshold")
	// A new incident opens when expiry nears again.
	setMonitorCert(t, hub, m, certExpiringIn(20))
	check(true, 1, 3)
	histories, err := hub.FindAllRecords("alerts_history", dbx.HashExp{"alert_id": alert.Id})
	require.NoError(t, err)
	require.Len(t, histories, 2)
	for _, history := range histories {
		assert.Equal(t, "NetworkMonitorCert", history.GetString("name"))
		assert.Equal(t, "https://example.com", history.GetString("monitor_name"))
	}
}

func TestNetworkMonitorCertAlertThresholdBoundary(t *testing.T) {
	for _, tc := range []struct {
		name      string
		offset    time.Duration
		triggered bool
	}{
		{name: "a minute beyond", offset: time.Minute, triggered: false},
		{name: "23 hours beyond", offset: 23 * time.Hour, triggered: false},
		{name: "a minute within", offset: -time.Minute, triggered: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hub, system, alert, m := certAlertSetup(t)
			am := alerts.NewTestAlertManagerWithoutWorker(hub)
			// Expiry beyond 30 days still displays as 30 days left, but must not trigger a 30 day threshold.
			setMonitorCert(t, hub, m, &monitor.CertInfo{Expires: time.Now().Add(30*24*time.Hour + tc.offset).UnixMilli()})
			require.NoError(t, am.HandleNetworkMonitorAlerts(system, nil))
			alert, err := hub.FindRecordById("alerts", alert.Id)
			require.NoError(t, err)
			assert.Equal(t, tc.triggered, alert.GetBool("triggered"))
		})
	}
}

func TestNetworkMonitorCertAlertExpired(t *testing.T) {
	hub, system, _, m := certAlertSetup(t)
	am := alerts.NewTestAlertManagerWithoutWorker(hub)
	count := hub.TestMailer.TotalSend()
	cert := &monitor.CertInfo{Expires: time.Now().Add(-72 * time.Hour).UnixMilli()}
	setMonitorCert(t, hub, m, cert)
	require.NoError(t, am.HandleNetworkMonitorAlerts(system, nil))
	require.Equal(t, count+1, hub.TestMailer.TotalSend())
	message := hub.TestMailer.Messages()[count]
	assert.Contains(t, message.Subject, "Certificate expired")
	assert.Contains(t, message.Text, "certificate expired on "+time.UnixMilli(cert.Expires).UTC().Format(time.DateOnly))
	histories, err := hub.FindAllRecords("alerts_history")
	require.NoError(t, err)
	require.Len(t, histories, 1)
	assert.Negative(t, histories[0].GetFloat("value"))
}

func TestNetworkMonitorCertAlertIgnoresMonitorsWithoutCert(t *testing.T) {
	hub, system, alert, _ := certAlertSetup(t)
	alert.Set("value", 365)
	require.NoError(t, hub.Save(alert))
	_, err := beszelTests.CreateRecord(hub, "network_monitors", map[string]any{
		"system": system.Id, "target": "gateway.example.com", "protocol": "icmp", "interval": 60, "enabled": true,
	})
	require.NoError(t, err)
	am := alerts.NewTestAlertManagerWithoutWorker(hub)
	count := hub.TestMailer.TotalSend()
	require.NoError(t, am.HandleNetworkMonitorAlerts(system, nil))
	assert.Equal(t, count, hub.TestMailer.TotalSend())
	alert, err = hub.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	assert.False(t, alert.GetBool("triggered"))
}

// The hub stores certificate info with direct SQL, which bypasses record hooks.
func TestNetworkMonitorCertAlertSQLUpdatedCert(t *testing.T) {
	hub, system, alert, m := certAlertSetup(t)
	am := alerts.NewTestAlertManagerWithoutWorker(hub)
	setMonitorCert(t, hub, m, certExpiringIn(60))
	require.NoError(t, am.HandleNetworkMonitorAlerts(system, nil))
	count := hub.TestMailer.TotalSend()
	cert := certExpiringIn(10)
	_, err := hub.DB().Update("network_monitors", dbx.Params{"certInfo": `{"expires":` + strconv.FormatInt(cert.Expires, 10) + `}`}, dbx.HashExp{"id": m.Id}).Execute()
	require.NoError(t, err)
	require.NoError(t, am.HandleNetworkMonitorAlerts(system, map[string]monitor.Result{m.Id: {Cert: cert}}))
	assert.Equal(t, count+1, hub.TestMailer.TotalSend())
	alert, err = hub.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	assert.True(t, alert.GetBool("triggered"))
}

func TestNetworkMonitorCertAlertSteadyStateNoDatabaseWork(t *testing.T) {
	hub, system, _, m := certAlertSetup(t)
	counted := &monitorCountingHub{TestHub: hub}
	am := alerts.NewTestAlertManagerWithoutWorker(counted)
	sql := monitorSQLCounter(t, hub)
	cert := certExpiringIn(10)
	setMonitorCert(t, hub, m, cert)
	// The agent resends unchanged certificate info for certificates about to expire.
	result := monitorResult(0)
	result.Cert = cert
	results := map[string]monitor.Result{m.Id: result}
	require.NoError(t, am.HandleNetworkMonitorAlerts(system, results))
	sql.Store(0)
	counted.transactions.Store(0)
	for range 100 {
		require.NoError(t, am.HandleNetworkMonitorAlerts(system, results))
	}
	assert.Zero(t, sql.Load(), "steady state must not issue SQL")
	assert.Zero(t, counted.transactions.Load(), "steady state must not open transactions")
}

func TestNetworkMonitorCertAlertIndependentOfLossAlert(t *testing.T) {
	hub, system, certAlert, m := certAlertSetup(t)
	lossAlert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name": "NetworkMonitorLoss", "system": system.Id, "user": certAlert.GetString("user"), "value": 5,
	})
	require.NoError(t, err)
	am := alerts.NewTestAlertManagerWithoutWorker(hub)
	setMonitorCert(t, hub, m, certExpiringIn(10))
	results := map[string]monitor.Result{m.Id: monitorResult(0)}
	require.NoError(t, am.HandleNetworkMonitorAlerts(system, results))
	results[m.Id] = monitorResult(50)
	require.NoError(t, am.HandleNetworkMonitorAlerts(system, results))
	// Recovering loss must leave the certificate incident open.
	results[m.Id] = monitorResult(0)
	require.NoError(t, am.HandleNetworkMonitorAlerts(system, results))
	for id, triggered := range map[string]bool{certAlert.Id: true, lossAlert.Id: false} {
		record, err := hub.FindRecordById("alerts", id)
		require.NoError(t, err)
		assert.Equal(t, triggered, record.GetBool("triggered"), record.GetString("name"))
	}
	open, err := hub.CountRecords("alerts_history", dbx.HashExp{"resolved": ""})
	require.NoError(t, err)
	assert.EqualValues(t, 1, open)
}

func TestNetworkMonitorCertAlertCleanup(t *testing.T) {
	for _, scenario := range []string{"disable monitor", "delete monitor", "down"} {
		t.Run(scenario, func(t *testing.T) {
			hub, system, alert, m := certAlertSetup(t)
			am := alerts.NewTestAlertManagerWithoutWorker(hub)
			setMonitorCert(t, hub, m, certExpiringIn(10))
			require.NoError(t, am.HandleNetworkMonitorAlerts(system, nil))
			count := hub.TestMailer.TotalSend()
			switch scenario {
			case "disable monitor":
				m.Set("enabled", false)
				require.NoError(t, hub.Save(m))
			case "delete monitor":
				require.NoError(t, hub.Delete(m))
			case "down":
				_, err := hub.DB().Update("systems", dbx.Params{"status": "down"}, dbx.HashExp{"id": system.Id}).Execute()
				require.NoError(t, err)
				system.Set("status", "down")
			}
			require.NoError(t, am.HandleNetworkMonitorAlerts(system, nil))
			assert.Equal(t, count, hub.TestMailer.TotalSend())
			record, err := hub.FindRecordById("alerts", alert.Id)
			require.NoError(t, err)
			open, err := hub.CountRecords("alerts_history", dbx.HashExp{"alert_id": alert.Id, "resolved": ""})
			require.NoError(t, err)
			// A disconnected system keeps its incident; removed monitors close it silently.
			assert.Equal(t, scenario == "down", record.GetBool("triggered"))
			assert.Equal(t, scenario == "down", open == 1)
		})
	}
}

func TestNetworkMonitorCertAlertAPI(t *testing.T) {
	for _, tc := range []struct {
		name   string
		value  float64
		status int
	}{
		{name: "valid threshold", value: 14, status: 200},
		{name: "maximum threshold", value: 365, status: 200},
		{name: "zero threshold", value: 0, status: 400},
		{name: "fractional threshold", value: 14.5, status: 400},
		{name: "too large threshold", value: 366, status: 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hub, user := beszelTests.GetHubWithUser(t)
			defer hub.Cleanup()
			systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "paused")
			require.NoError(t, err)
			token, err := user.NewAuthToken()
			require.NoError(t, err)
			for _, direct := range []bool{false, true} {
				body := map[string]any{"name": "NetworkMonitorCert", "value": tc.value, "min": 10, "systems": []string{systems[0].Id}, "overwrite": true}
				url := "/api/beszel/user-alerts"
				if direct {
					url = "/api/collections/alerts/records"
					body["system"], body["user"] = systems[0].Id, user.Id
					existing, err := hub.FindAllRecords("alerts")
					require.NoError(t, err)
					for _, record := range existing {
						require.NoError(t, hub.Delete(record))
					}
				}
				content := `"status":400`
				if tc.status == 200 {
					content = `"NetworkMonitorCert"`
					if !direct {
						content = `"success":true`
					}
				}
				scenario := beszelTests.ApiScenario{
					Name: tc.name, Method: "POST", URL: url, Body: jsonReader(body),
					Headers: map[string]string{"Authorization": token}, ExpectedStatus: tc.status, ExpectedContent: []string{content},
					TestAppFactory: func(testing.TB) *pbTests.TestApp { return hub.TestApp },
				}
				scenario.Test(t)
				records, err := hub.FindAllRecords("alerts")
				require.NoError(t, err)
				if tc.status != 200 {
					assert.Empty(t, records)
					continue
				}
				require.Len(t, records, 1)
				assert.Equal(t, tc.value, records[0].GetFloat("value"))
				assert.Zero(t, records[0].GetInt("min"))
			}
		})
	}
}
