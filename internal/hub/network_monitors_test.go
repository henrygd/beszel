//go:build testing

package hub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateNetworkMonitorsOnPausedSystem(t *testing.T) {
	for _, batch := range []bool{false, true} {
		name := "single"
		if batch {
			name = "batch"
		}
		t.Run(name, func(t *testing.T) {
			hub, testApp, err := createTestHub(t)
			require.NoError(t, err)
			defer cleanupTestHub(hub, testApp)
			bindNetworkMonitorsEvents(hub)

			user, err := createTestUser(hub)
			require.NoError(t, err)
			system, err := createTestRecord(hub, "systems", map[string]any{
				"name": "Paused", "host": "localhost", "port": "45876",
				"status": "paused", "users": []string{user.Id},
			})
			require.NoError(t, err)
			// Paused systems are not loaded into the manager at startup.
			_, err = hub.sm.GetSystem(system.Id)
			require.Error(t, err)

			payload := func(target string) map[string]any {
				return map[string]any{
					"system": system.Id, "target": target, "protocol": "icmp",
					"interval": 60, "enabled": true,
				}
			}
			url := "/api/collections/network_monitors/records"
			var body any = payload("1.1.1.1")
			count := 1
			if batch {
				body = map[string]any{"requests": []map[string]any{
					{"method": "POST", "url": url, "body": payload("1.1.1.1")},
					{"method": "POST", "url": url, "body": payload("8.8.8.8")},
				}}
				url = "/api/batch"
				count = 2
			}
			data, err := json.Marshal(body)
			require.NoError(t, err)
			token, err := user.NewAuthToken()
			require.NoError(t, err)
			router, err := apis.NewRouter(hub)
			require.NoError(t, err)
			handler, err := router.BuildMux()
			require.NoError(t, err)
			request := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(data))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", token)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			assert.Equal(t, http.StatusOK, response.Code, response.Body.String())

			records, err := hub.FindAllRecords("network_monitors")
			require.NoError(t, err)
			require.Len(t, records, count)
			for _, record := range records {
				assert.Equal(t, system.Id, record.GetString("system"))
				assert.True(t, record.GetBool("enabled"))
			}
		})
	}
}

func TestGenerateMonitorID(t *testing.T) {
	tests := []struct {
		name     string
		systemID string
		config   monitor.Config
		expected string
	}{
		{
			name:     "Short hash meets the record ID minimum",
			systemID: "000000000012369",
			config:   monitor.Config{Protocol: "icmp", Target: "gateway.example.com"},
			expected: "0b3fd2",
		},
		{
			name:     "Existing six-character ID is unchanged",
			systemID: "000000000000274",
			config:   monitor.Config{Protocol: "icmp", Target: "gateway.example.com"},
			expected: "77a996",
		},
		{
			name:     "HTTP monitor on example.com",
			systemID: "sys123",
			config: monitor.Config{
				Protocol: "http",
				Target:   "example.com",
				Port:     0,
				Interval: 60,
			},
			expected: "a20a5827",
		},
		{
			name:     "HTTP monitor on example.com with different port",
			systemID: "sys123",
			config: monitor.Config{
				Protocol: "http",
				Target:   "example.com",
				Port:     8080,
				Interval: 60,
			},
			expected: "a20a5827",
		},
		{
			name:     "HTTP monitor on example.com with different system ID",
			systemID: "sys1234",
			config: monitor.Config{
				Protocol: "http",
				Target:   "example.com",
				Port:     80,
				Interval: 60,
			},
			expected: "ab602ae7",
		},
		{
			name:     "Same monitor, different interval",
			systemID: "sys1234",
			config: monitor.Config{
				Protocol: "http",
				Target:   "example.com",
				Port:     80,
				Interval: 120,
			},
			expected: "ab602ae7",
		},
		{
			name:     "ICMP monitor on 1.1.1.1",
			systemID: "sys456",
			config: monitor.Config{
				Protocol: "icmp",
				Target:   "1.1.1.1",
				Port:     0,
				Interval: 10,
			},
			expected: "6d13a4a4",
		}, {
			name:     "ICMP monitor on 1.1.1.1 with different system ID",
			systemID: "sys4567",
			config: monitor.Config{
				Protocol: "icmp",
				Target:   "1.1.1.1",
				Port:     0,
				Interval: 10,
			},
			expected: "ddd6c81",
		},
		{
			name:     "TCP monitor on example.com with port 443",
			systemID: "sys789",
			config: monitor.Config{
				Protocol: "tcp",
				Target:   "example.com",
				Port:     443,
				Interval: 30,
			},
			expected: "677b991",
		},
		{
			name:     "TCP monitor on example.com with port 8443",
			systemID: "sys789",
			config: monitor.Config{
				Protocol: "tcp",
				Target:   "example.com",
				Port:     8443,
				Interval: 30,
			},
			expected: "84167969",
		},
		{
			name:     "DNS monitor on example.com with server 1.1.1.1",
			systemID: "sys999",
			config: monitor.Config{
				Protocol: "dns",
				Target:   "example.com",
				Server:   "1.1.1.1",
				Interval: 30,
			},
			expected: "2175898b",
		},
		{
			name:     "DNS monitor on example.com with different server",
			systemID: "sys999",
			config: monitor.Config{
				Protocol: "dns",
				Target:   "example.com",
				Server:   "8.8.8.8",
				Interval: 30,
			},
			expected: "ebcd8b33",
		},
		{
			name:     "DNS monitor on example.com with no server (system resolver)",
			systemID: "sys999",
			config: monitor.Config{
				Protocol: "dns",
				Target:   "example.com",
				Server:   "",
				Interval: 30,
			},
			expected: "19476a7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateMonitorID(tt.systemID, tt.config)
			assert.Equal(t, tt.expected, got, "generateMonitorID() = %v, want %v", got, tt.expected)
		})
	}
}

func TestGenerateMonitorIDContentChecks(t *testing.T) {
	dns := monitor.Config{Protocol: "dns", Target: "example.com", Server: "1.1.1.1"}
	// the default lookup keeps the ID that existing monitors already have
	assert.Equal(t, "2175898b", generateMonitorID("sys999", dns))

	mx, txt := dns, dns
	mx.RecordType = "MX"
	txt.RecordType = "TXT"
	assert.NotEqual(t, generateMonitorID("sys999", dns), generateMonitorID("sys999", mx))
	assert.NotEqual(t, generateMonitorID("sys999", mx), generateMonitorID("sys999", txt))

	// the expected content is a probe option and must not change the ID
	withKeyword := mx
	withKeyword.Keyword = "mail.example.com"
	assert.Equal(t, generateMonitorID("sys999", mx), generateMonitorID("sys999", withKeyword))

	httpConfig := monitor.Config{Protocol: "http", Target: "https://example.com"}
	withJSON := httpConfig
	withJSON.Keyword = "UP"
	withJSON.JSONPath = "status"
	// the record type only applies to DNS monitors
	withJSON.RecordType = "MX"
	assert.Equal(t, generateMonitorID("sys999", httpConfig), generateMonitorID("sys999", withJSON))
}

func TestNetworkMonitorShortIDRequests(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			hub, testApp, err := createTestHub(t)
			require.NoError(t, err)
			defer cleanupTestHub(hub, testApp)
			bindNetworkMonitorsEvents(hub)
			user, err := createTestUser(hub)
			require.NoError(t, err)
			system, err := createTestRecord(hub, "systems", map[string]any{
				"id": "000000000012369", "name": "Paused", "host": "localhost", "port": "45876",
				"status": "paused", "users": []string{user.Id},
			})
			require.NoError(t, err)
			payload := map[string]any{
				"system": system.Id, "target": "gateway.example.com", "protocol": "icmp",
				"interval": 60, "enabled": true,
			}
			url := "/api/collections/network_monitors/records"
			var oldID string
			if method == http.MethodPatch {
				previous, err := createTestRecord(hub, "network_monitors", map[string]any{
					"system": system.Id, "target": "website.example.com", "protocol": "icmp", "interval": 60,
				})
				require.NoError(t, err)
				oldID = previous.Id
				url += "/" + oldID
			}
			data, err := json.Marshal(payload)
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
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			record, err := hub.FindRecordById("network_monitors", "0b3fd2")
			require.NoError(t, err)
			assert.Equal(t, "gateway.example.com", record.GetString("target"))
			if oldID != "" {
				_, err = hub.FindRecordById("network_monitors", oldID)
				require.Error(t, err)
			}
		})
	}
}

func TestCopyMonitorToNewRecordDropsResultFields(t *testing.T) {
	hub, testApp, err := createTestHub(t)
	require.NoError(t, err)
	defer cleanupTestHub(hub, testApp)

	collection, err := hub.FindCachedCollectionByNameOrId("network_monitors")
	require.NoError(t, err)
	assert.Nil(t, collection.Fields.GetByName("name"))

	oldRecord := core.NewRecord(collection)
	oldRecord.Load(map[string]any{
		"system":        "sys123",
		"target":        "https://example.com",
		"protocol":      "http",
		"port":          443,
		"server":        "1.1.1.1",
		"interval":      60,
		"enabled":       true,
		"skipTlsVerify": true,
		"keyword":       "UP",
		"jsonPath":      "status",
		"recordType":    "MX",
		"res":           1200,
		"resAvg1h":      1300,
		"resMin1h":      900,
		"resMax1h":      1600,
		"loss1h":        5,
		"certInfo":      map[string]any{"expires": 1800000000000},
		"updated":       "2026-04-29 12:00:00.000Z",
	})

	newRecord := copyMonitorToNewRecord(oldRecord, "next12345")

	assert.Equal(t, "next12345", newRecord.Id)
	assert.Equal(t, "https://example.com", newRecord.GetString("target"))
	assert.Equal(t, "http", newRecord.GetString("protocol"))
	assert.Equal(t, 443, newRecord.GetInt("port"))
	assert.Equal(t, "1.1.1.1", newRecord.GetString("server"))
	assert.True(t, newRecord.GetBool("enabled"))
	assert.True(t, newRecord.GetBool("skipTlsVerify"))
	assert.Equal(t, "UP", newRecord.GetString("keyword"))
	assert.Equal(t, "status", newRecord.GetString("jsonPath"))
	assert.Equal(t, "MX", newRecord.GetString("recordType"))
	assert.Contains(t, []string{"", "null"}, newRecord.GetString("certInfo"))
	assert.Zero(t, newRecord.GetFloat("res"))
	assert.Zero(t, newRecord.GetFloat("resAvg1h"))
	assert.Zero(t, newRecord.GetFloat("resMin1h"))
	assert.Zero(t, newRecord.GetFloat("resMax1h"))
	assert.Zero(t, newRecord.GetFloat("loss1h"))
	assert.Equal(t, "", newRecord.GetString("updated"))
}

func TestUpdateNetworkMonitorSystemAuthorization(t *testing.T) {
	hub, testApp, err := createTestHub(t)
	require.NoError(t, err)
	defer cleanupTestHub(hub, testApp)
	require.NoError(t, setCollectionAuthSettings(hub))
	bindNetworkMonitorsEvents(hub)

	alice, err := createTestUser(hub)
	require.NoError(t, err)
	bob, err := createTestRecord(hub, "users", map[string]any{
		"email":    "bob@example.com",
		"password": "password123",
	})
	require.NoError(t, err)

	aliceSystemA, err := createTestRecord(hub, "systems", map[string]any{
		"name": "AliceSystemA", "host": "localhost", "port": "45876",
		"status": "paused", "users": []string{alice.Id},
	})
	require.NoError(t, err)
	aliceSystemC, err := createTestRecord(hub, "systems", map[string]any{
		"name": "AliceSystemC", "host": "localhost", "port": "45877",
		"status": "paused", "users": []string{alice.Id},
	})
	require.NoError(t, err)
	bobSystemB, err := createTestRecord(hub, "systems", map[string]any{
		"name": "BobSystemB", "host": "localhost", "port": "45878",
		"status": "paused", "users": []string{bob.Id},
	})
	require.NoError(t, err)

	monitorRecord, err := createTestRecord(hub, "network_monitors", map[string]any{
		"system": aliceSystemA.Id, "target": "1.1.1.1", "protocol": "icmp",
		"interval": 60, "enabled": true,
	})
	require.NoError(t, err)

	aliceToken, err := alice.NewAuthToken()
	require.NoError(t, err)
	router, err := apis.NewRouter(hub)
	require.NoError(t, err)
	handler, err := router.BuildMux()
	require.NoError(t, err)

	// 1. Alice cannot move her monitor to Bob's system B
	patchBobBody, err := json.Marshal(map[string]any{
		"system":   bobSystemB.Id,
		"target":   "http://127.0.0.1:12345/probe",
		"protocol": "http",
		"interval": 60,
		"enabled":  true,
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPatch, "/api/collections/network_monitors/records/"+monitorRecord.Id, bytes.NewReader(patchBobBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", aliceToken)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusForbidden, resp.Code, resp.Body.String())

	records, err := hub.FindAllRecords("network_monitors")
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, monitorRecord.Id, records[0].Id)
	assert.Equal(t, aliceSystemA.Id, records[0].GetString("system"))

	// 2. Alice can move her monitor to her own system C
	patchAliceBody, err := json.Marshal(map[string]any{
		"system":   aliceSystemC.Id,
		"target":   "1.1.1.1",
		"protocol": "icmp",
		"interval": 60,
		"enabled":  true,
	})
	require.NoError(t, err)
	req2 := httptest.NewRequest(http.MethodPatch, "/api/collections/network_monitors/records/"+monitorRecord.Id, bytes.NewReader(patchAliceBody))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", aliceToken)
	resp2 := httptest.NewRecorder()
	handler.ServeHTTP(resp2, req2)
	assert.Equal(t, http.StatusOK, resp2.Code, resp2.Body.String())

	records, err = hub.FindAllRecords("network_monitors")
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, aliceSystemC.Id, records[0].GetString("system"))
}

func TestNetworkMonitorRecordTypeValidation(t *testing.T) {
	hub, testApp, err := createTestHub(t)
	require.NoError(t, err)
	defer cleanupTestHub(hub, testApp)

	user, err := createTestUser(hub)
	require.NoError(t, err)
	system, err := createTestRecord(hub, "systems", map[string]any{
		"name": "Record types", "host": "localhost", "port": "45876",
		"status": "paused", "users": []string{user.Id},
	})
	require.NoError(t, err)
	collection, err := hub.FindCachedCollectionByNameOrId("network_monitors")
	require.NoError(t, err)

	tests := map[string]bool{"": true, "A": true, "AAAA": true, "CNAME": true, "MX": true, "NS": true, "TXT": true, "mx": false, "SRV": false}
	for recordType, valid := range tests {
		record := core.NewRecord(collection)
		record.Load(map[string]any{
			"system": system.Id, "target": "example.com", "protocol": "dns", "server": recordType,
			"interval": 60, "recordType": recordType, "keyword": "mail.example.com",
		})
		err := hub.Save(record)
		if valid {
			assert.NoError(t, err, "record type %q", recordType)
		} else {
			assert.Error(t, err, "record type %q", recordType)
		}
	}
}
