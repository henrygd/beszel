//go:build testing

package hub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/henrygd/beszel/internal/entities/speedtest"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSpeedtestID(t *testing.T) {
	auto := generateSpeedtestID("sys1", speedtest.Config{})
	assert.Equal(t, auto, generateSpeedtestID("sys1", speedtest.Config{}), "IDs must be stable")
	assert.NotEqual(t, auto, generateSpeedtestID("sys2", speedtest.Config{}))
	assert.NotEqual(t, auto, generateSpeedtestID("sys1", speedtest.Config{ServerID: 42}))
	assert.NotEqual(t, auto, generateSpeedtestID("sys1", speedtest.Config{Interface: "eth1"}))
	assert.NotEqual(t, generateSpeedtestID("sys1", speedtest.Config{Interface: "eth1"}), generateSpeedtestID("sys1", speedtest.Config{Interface: "eth2"}))
	assert.Regexp(t, "^[a-z0-9]{1,10}$", auto)
}

func TestSpeedtestServerChangeReplacesRecord(t *testing.T) {
	hub, testApp, err := createTestHub(t)
	require.NoError(t, err)
	defer cleanupTestHub(hub, testApp)
	bindSpeedtestsEvents(hub)

	user, err := createTestUser(hub)
	require.NoError(t, err)
	system, err := createTestRecord(hub, "systems", map[string]any{
		"name": "Paused", "host": "localhost", "port": "45876",
		"status": "paused", "users": []string{user.Id},
	})
	require.NoError(t, err)
	record, err := createTestRecord(hub, "speedtests", map[string]any{
		"system": system.Id, "interval": 60, "enabled": true,
	})
	require.NoError(t, err)
	assert.Equal(t, generateSpeedtestID(system.Id, speedtest.Config{}), record.Id)

	status, body := speedtestAPIRequest(t, hub, user, http.MethodPatch, "/api/collections/speedtests/records/"+record.Id, map[string]any{"server_id": 42, "server_name": "Odido", "server_location": "Amsterdam, Netherlands"})
	assert.Equal(t, http.StatusOK, status, body)

	records, err := hub.FindAllRecords("speedtests")
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, generateSpeedtestID(system.Id, speedtest.Config{ServerID: 42}), records[0].Id)
	assert.Equal(t, 42, records[0].GetInt("server_id"))
	assert.Equal(t, "Odido", records[0].GetString("server_name"))
	assert.Equal(t, "Amsterdam, Netherlands", records[0].GetString("server_location"))
	assert.Equal(t, 60, records[0].GetInt("interval"))
	assert.True(t, records[0].GetBool("enabled"))
}

// Speedtest IDs are unpadded hashes, so the schema must accept IDs shorter than six characters.
func TestSpeedtestShortIDRequests(t *testing.T) {
	const systemID, shortID = "000000000001739", "a0116"
	require.Equal(t, shortID, generateSpeedtestID(systemID, speedtest.Config{}))
	for _, method := range []string{http.MethodPost, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			hub, testApp, err := createTestHub(t)
			require.NoError(t, err)
			defer cleanupTestHub(hub, testApp)
			bindSpeedtestsEvents(hub)
			user, err := createTestUser(hub)
			require.NoError(t, err)
			system, err := createTestRecord(hub, "systems", map[string]any{
				"id": systemID, "name": "Paused", "host": "localhost", "port": "45876",
				"status": "paused", "users": []string{user.Id},
			})
			require.NoError(t, err)
			url := "/api/collections/speedtests/records"
			payload := map[string]any{"system": system.Id, "server_id": 0, "interval": 60, "enabled": true}
			var oldID string
			if method == http.MethodPatch {
				previous, err := createTestRecord(hub, "speedtests", map[string]any{
					"system": system.Id, "server_id": 42, "interval": 60,
				})
				require.NoError(t, err)
				oldID = previous.Id
				url += "/" + oldID
				payload = map[string]any{"server_id": 0}
			}
			status, body := speedtestAPIRequest(t, hub, user, method, url, payload)
			require.Equal(t, http.StatusOK, status, body)
			_, err = hub.FindRecordById("speedtests", shortID)
			require.NoError(t, err)
			if oldID != "" {
				_, err = hub.FindRecordById("speedtests", oldID)
				require.Error(t, err)
			}
		})
	}
}

func TestDuplicateSpeedtestIsRejected(t *testing.T) {
	hub, testApp, err := createTestHub(t)
	require.NoError(t, err)
	defer cleanupTestHub(hub, testApp)
	bindSpeedtestsEvents(hub)

	user, err := createTestUser(hub)
	require.NoError(t, err)
	system, err := createTestRecord(hub, "systems", map[string]any{
		"name": "Paused", "host": "localhost", "port": "45876",
		"status": "paused", "users": []string{user.Id},
	})
	require.NoError(t, err)
	create := func(serverID int) (int, string) {
		return speedtestAPIRequest(t, hub, user, http.MethodPost, "/api/collections/speedtests/records", map[string]any{
			"system": system.Id, "server_id": serverID, "interval": 60, "enabled": true,
		})
	}

	status, body := create(0)
	require.Equal(t, http.StatusOK, status, body)
	status, body = create(42)
	require.Equal(t, http.StatusOK, status, body)

	// Adding the same server to the same system again.
	status, body = create(42)
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Contains(t, body, errDuplicateSpeedtest)

	// Switching a speedtest to a server the system already tests against.
	status, body = speedtestAPIRequest(t, hub, user, http.MethodPatch,
		"/api/collections/speedtests/records/"+generateSpeedtestID(system.Id, speedtest.Config{}), map[string]any{"server_id": 42})
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Contains(t, body, errDuplicateSpeedtest)

	records, err := hub.FindAllRecords("speedtests")
	require.NoError(t, err)
	assert.Len(t, records, 2, "the rejected edit must not delete the original speedtest")
}

func TestSpeedtestInterfaces(t *testing.T) {
	hub, testApp, err := createTestHub(t)
	require.NoError(t, err)
	defer cleanupTestHub(hub, testApp)
	bindSpeedtestsEvents(hub)

	user, err := createTestUser(hub)
	require.NoError(t, err)
	system, err := createTestRecord(hub, "systems", map[string]any{
		"name": "Paused", "host": "localhost", "port": "45876",
		"status": "paused", "users": []string{user.Id},
	})
	require.NoError(t, err)
	create := func(iface string) (int, string) {
		return speedtestAPIRequest(t, hub, user, http.MethodPost, "/api/collections/speedtests/records", map[string]any{
			"system": system.Id, "server_id": 42, "interface": iface, "interval": 60, "enabled": true,
		})
	}

	// The same server can be tested once per interface.
	for _, iface := range []string{"", "eth1"} {
		status, body := create(iface)
		require.Equal(t, http.StatusOK, status, body)
	}
	// Surrounding whitespace is trimmed, so this duplicates eth1.
	status, body := create(" eth1 ")
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Contains(t, body, errDuplicateSpeedtest)

	eth1ID := generateSpeedtestID(system.Id, speedtest.Config{ServerID: 42, Interface: "eth1"})
	record, err := hub.FindRecordById("speedtests", eth1ID)
	require.NoError(t, err)
	assert.Equal(t, "eth1", record.GetString("interface"))

	// Changing the interface replaces the record.
	status, body = speedtestAPIRequest(t, hub, user, http.MethodPatch, "/api/collections/speedtests/records/"+eth1ID, map[string]any{"interface": "eth2 "})
	require.Equal(t, http.StatusOK, status, body)
	_, err = hub.FindRecordById("speedtests", eth1ID)
	assert.Error(t, err)
	record, err = hub.FindRecordById("speedtests", generateSpeedtestID(system.Id, speedtest.Config{ServerID: 42, Interface: "eth2"}))
	require.NoError(t, err)
	assert.Equal(t, "eth2", record.GetString("interface"))
	assert.Equal(t, 60, record.GetInt("interval"))

	configs, err := hub.sm.GetSpeedtestConfigsForSystem(system.Id)
	require.NoError(t, err)
	interfaces := make([]string, len(configs))
	for i, config := range configs {
		interfaces[i] = config.Interface
	}
	assert.ElementsMatch(t, []string{"", "eth2"}, interfaces)
}

func TestRunSpeedtest(t *testing.T) {
	hub, testApp, err := createTestHub(t)
	require.NoError(t, err)
	defer cleanupTestHub(hub, testApp)

	user, err := createTestUser(hub)
	require.NoError(t, err)
	readonly, err := createTestRecord(hub, "users", map[string]any{
		"email": "readonly@test.com", "password": "testtesttest", "role": "readonly",
	})
	require.NoError(t, err)
	other, err := createTestRecord(hub, "users", map[string]any{"email": "other@test.com", "password": "testtesttest"})
	require.NoError(t, err)

	system, err := createTestRecord(hub, "systems", map[string]any{
		"name": "Down", "host": "127.0.0.1", "port": "1",
		"status": "down", "users": []string{user.Id, readonly.Id},
	})
	require.NoError(t, err)
	require.NoError(t, hub.sm.AddRecord(system, nil))
	enabled, err := createTestRecord(hub, "speedtests", map[string]any{"system": system.Id, "interval": 60, "enabled": true})
	require.NoError(t, err)
	paused, err := createTestRecord(hub, "speedtests", map[string]any{"system": system.Id, "server_id": 42, "interval": 60, "enabled": false})
	require.NoError(t, err)

	handler := speedtestServersMux(t, hub)
	run := func(user *core.Record, id string) int {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/api/beszel/speedtest/run?id="+id, nil)
		if user != nil {
			token, err := user.NewAuthToken()
			require.NoError(t, err)
			request.Header.Set("Authorization", token)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response.Code
	}

	assert.Equal(t, http.StatusUnauthorized, run(nil, enabled.Id))
	assert.Equal(t, http.StatusForbidden, run(readonly, enabled.Id))
	assert.Equal(t, http.StatusBadRequest, run(user, ""))
	assert.Equal(t, http.StatusNotFound, run(user, "missing"))
	assert.Equal(t, http.StatusNotFound, run(other, enabled.Id), "users without access to the system")
	assert.Equal(t, http.StatusBadRequest, run(user, paused.Id), "paused speedtests")
	assert.Equal(t, http.StatusBadRequest, run(user, enabled.Id), "systems that are not up")
}

func speedtestAPIRequest(t *testing.T, hub *Hub, user *core.Record, method, url string, body any) (int, string) {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	token, err := user.NewAuthToken()
	require.NoError(t, err)
	router, err := apis.NewRouter(hub)
	require.NoError(t, err)
	handler, err := router.BuildMux()
	require.NoError(t, err)
	request := httptest.NewRequest(method, url, bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response.Code, response.Body.String()
}
