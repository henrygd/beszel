//go:build testing

package alerts_test

import (
	"fmt"
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

// setMonitorAlert changes the setup alert's type and threshold.
func setMonitorAlert(t *testing.T, hub core.App, alert *core.Record, name string, value float64) {
	t.Helper()
	alert.Set("name", name)
	alert.Set("value", value)
	require.NoError(t, hub.Save(alert))
}

func streakResult(streak int64) monitor.Result {
	return monitor.Result{LastProbeAt: time.Now().UnixMilli(), SampleCount: 1, Streak: streak}
}

func openIncidents(t *testing.T, hub core.App, alertID string) int64 {
	t.Helper()
	total, err := hub.CountRecords("alerts_history", dbx.HashExp{"alert_id": alertID, "resolved": ""})
	require.NoError(t, err)
	return total
}

func TestNetworkMonitorDownAlertLifecycle(t *testing.T) {
	hub, system, alert, monitors := networkAlertSetup(t)
	setMonitorAlert(t, hub, alert, "NetworkMonitorDown", 3)
	am := alerts.NewTestAlertManagerWithoutWorker(hub)
	count := hub.TestMailer.TotalSend()
	id := monitors[0].Id
	handle := func(streak int64) {
		t.Helper()
		require.NoError(t, am.HandleNetworkMonitorAlerts(system, map[string]monitor.Result{id: streakResult(streak)}))
	}

	// Below the threshold nothing happens, even with no warm-up history.
	handle(-2)
	assert.Zero(t, openIncidents(t, hub, alert.Id))
	assert.Equal(t, count, hub.TestMailer.TotalSend())

	handle(-3)
	assert.EqualValues(t, 1, openIncidents(t, hub, alert.Id))
	require.Equal(t, count+1, hub.TestMailer.TotalSend())
	assert.Contains(t, hub.TestMailer.Messages()[count].Text, "the last 3 checks failed")
	history, err := hub.FindFirstRecordByFilter("alerts_history", "alert_id = {:id}", dbx.Params{"id": alert.Id})
	require.NoError(t, err)
	assert.Equal(t, 3.0, history.GetFloat("value"))

	// Further failures don't notify again.
	handle(-10)
	assert.Equal(t, count+1, hub.TestMailer.TotalSend())

	// The first success resolves the incident immediately.
	handle(1)
	assert.Zero(t, openIncidents(t, hub, alert.Id))
	require.Equal(t, count+2, hub.TestMailer.TotalSend())
	assert.Contains(t, hub.TestMailer.Messages()[count+1].Subject, "Network monitor up")
	record, err := hub.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	assert.False(t, record.GetBool("triggered"))
}

func TestNetworkMonitorDownAlertIgnoresUnknownResults(t *testing.T) {
	for name, result := range map[string]monitor.Result{
		// Agents without streak reporting send zero.
		"old agent": {LastProbeAt: time.Now().UnixMilli(), SampleCount: 60, PacketLoss1h: 100},
		"stale":     {LastProbeAt: time.Now().Add(-time.Hour).UnixMilli(), SampleCount: 60, Streak: -50},
		"no probes": {Streak: -5},
	} {
		t.Run(name, func(t *testing.T) {
			hub, system, alert, monitors := networkAlertSetup(t)
			setMonitorAlert(t, hub, alert, "NetworkMonitorDown", 1)
			am := alerts.NewTestAlertManagerWithoutWorker(hub)
			require.NoError(t, am.HandleNetworkMonitorAlerts(system, map[string]monitor.Result{monitors[0].Id: result}))
			assert.Zero(t, openIncidents(t, hub, alert.Id))
		})
	}
}

func TestNetworkMonitorLatencyAlertLifecycle(t *testing.T) {
	hub, system, alert, monitors := networkAlertSetup(t)
	setMonitorAlert(t, hub, alert, "NetworkMonitorLatency", 200)
	am := alerts.NewTestAlertManagerWithoutWorker(hub)
	count := hub.TestMailer.TotalSend()
	id := monitors[0].Id
	handle := func(avgUs int64, loss float64) {
		t.Helper()
		result := monitorResult(loss)
		result.AvgResponse1h = avgUs
		require.NoError(t, am.HandleNetworkMonitorAlerts(system, map[string]monitor.Result{id: result}))
	}

	handle(200_000, 0) // Equal to the threshold is not slow.
	assert.Zero(t, openIncidents(t, hub, alert.Id))

	handle(250_500, 0)
	assert.EqualValues(t, 1, openIncidents(t, hub, alert.Id))
	require.Equal(t, count+1, hub.TestMailer.TotalSend())
	assert.Contains(t, hub.TestMailer.Messages()[count].Text, "250.50 ms")

	// A full hour of loss has no response time, so it neither triggers nor resolves.
	handle(0, 100)
	assert.EqualValues(t, 1, openIncidents(t, hub, alert.Id))

	handle(150_000, 0)
	assert.Zero(t, openIncidents(t, hub, alert.Id))
	assert.Equal(t, count+2, hub.TestMailer.TotalSend())
}

// The API test also covers the migration that adds both names to the alerts collection.
func TestNetworkMonitorDownAndLatencyAlertAPI(t *testing.T) {
	for _, tc := range []struct {
		name   string
		value  float64
		status int
	}{
		{"NetworkMonitorDown", 1, 200},
		{"NetworkMonitorDown", 20, 200},
		{"NetworkMonitorDown", 0, 400},
		{"NetworkMonitorDown", 21, 400},
		{"NetworkMonitorDown", 2.5, 400},
		{"NetworkMonitorLatency", 1, 200},
		{"NetworkMonitorLatency", 10_000, 200},
		{"NetworkMonitorLatency", 0, 400},
		{"NetworkMonitorLatency", 10_001, 400},
	} {
		t.Run(fmt.Sprintf("%s %v", tc.name, tc.value), func(t *testing.T) {
			hub, user := beszelTests.GetHubWithUser(t)
			defer hub.Cleanup()
			systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "paused")
			require.NoError(t, err)
			token, err := user.NewAuthToken()
			require.NoError(t, err)
			content := `"status":400`
			if tc.status == 200 {
				content = `"success":true`
			}
			body := map[string]any{"name": tc.name, "value": tc.value, "min": 10, "systems": []string{systems[0].Id}, "overwrite": true}
			scenario := beszelTests.ApiScenario{
				Name: tc.name, Method: "POST", URL: "/api/beszel/user-alerts", Body: jsonReader(body),
				Headers: map[string]string{"Authorization": token}, ExpectedStatus: tc.status, ExpectedContent: []string{content},
				TestAppFactory: func(testing.TB) *pbTests.TestApp { return hub.TestApp },
			}
			scenario.Test(t)
			records, err := hub.FindAllRecords("alerts")
			require.NoError(t, err)
			if tc.status != 200 {
				assert.Empty(t, records)
				return
			}
			require.Len(t, records, 1)
			assert.Equal(t, tc.name, records[0].GetString("name"))
			assert.Equal(t, tc.value, records[0].GetFloat("value"))
			assert.Zero(t, records[0].GetInt("min"))
		})
	}
}
