//go:build testing

package hub_test

import (
	"net/http"
	"testing"

	beszelTests "github.com/henrygd/beszel/internal/tests"
	pbTests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetworkMonitorPush(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()
	userToken, err := user.NewAuthToken()
	require.NoError(t, err)
	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name": "test-system", "users": []string{user.Id}, "host": "127.0.0.1", "status": "paused",
	})
	require.NoError(t, err)

	monitor, err := beszelTests.CreateRecord(hub, "network_monitors", map[string]any{
		"system": system.Id, "target": "nightly backup", "protocol": "push", "interval": 86400, "enabled": true,
	})
	require.NoError(t, err)
	token := monitor.GetString("pushToken")
	require.Len(t, token, 32, "push monitors get a generated token")
	disabled, err := beszelTests.CreateRecord(hub, "network_monitors", map[string]any{
		"system": system.Id, "target": "paused job", "protocol": "push", "interval": 60, "enabled": false,
	})
	require.NoError(t, err)

	testAppFactory := func(t testing.TB) *pbTests.TestApp { return hub.TestApp }
	expectLastPush := func(id string, set bool) func(testing.TB, *pbTests.TestApp, *http.Response) {
		return func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
			record, err := app.FindRecordById("network_monitors", id)
			require.NoError(t, err)
			assert.Equal(t, set, record.GetInt("lastPush") > 0)
		}
	}
	createBody := func(protocol string, interval int) map[string]any {
		return map[string]any{"system": system.Id, "target": "created-" + protocol, "protocol": protocol, "interval": interval, "enabled": true}
	}

	scenarios := []beszelTests.ApiScenario{
		{
			Name: "GET /push/{token} - unknown token", Method: http.MethodGet, URL: "/api/beszel/push/unknown",
			ExpectedStatus: 404, ExpectedContent: []string{"\"data\":{}"}, TestAppFactory: testAppFactory,
		},
		{
			Name: "GET /push/{token} - disabled monitor", Method: http.MethodGet, URL: "/api/beszel/push/" + disabled.GetString("pushToken"),
			ExpectedStatus: 404, ExpectedContent: []string{"\"data\":{}"}, TestAppFactory: testAppFactory,
			AfterTestFunc: expectLastPush(disabled.Id, false),
		},
		{
			Name: "GET /push/{token} - records heartbeat without auth", Method: http.MethodGet, URL: "/api/beszel/push/" + token,
			ExpectedStatus: 204, TestAppFactory: testAppFactory, AfterTestFunc: expectLastPush(monitor.Id, true),
		},
		{
			Name: "POST /push/{token} - records heartbeat", Method: http.MethodPost, URL: "/api/beszel/push/" + token,
			ExpectedStatus: 204, TestAppFactory: testAppFactory, AfterTestFunc: expectLastPush(monitor.Id, true),
		},
		{
			Name: "create icmp monitor above probe interval limit", Method: http.MethodPost, URL: "/api/collections/network_monitors/records",
			Body: jsonReader(createBody("icmp", 7200)), Headers: map[string]string{"Authorization": userToken},
			ExpectedStatus: 400, ExpectedContent: []string{"3600"}, TestAppFactory: testAppFactory,
		},
		{
			Name: "create push monitor above probe interval limit", Method: http.MethodPost, URL: "/api/collections/network_monitors/records",
			Body: jsonReader(createBody("push", 7200)), Headers: map[string]string{"Authorization": userToken},
			ExpectedStatus: 200, ExpectedContent: []string{"\"pushToken\""}, TestAppFactory: testAppFactory,
		},
	}
	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}
