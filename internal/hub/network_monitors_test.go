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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateMonitorID(tt.systemID, tt.config)
			assert.Equal(t, tt.expected, got, "generateMonitorID() = %v, want %v", got, tt.expected)
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
		"system":    "sys123",
		"target":    "https://example.com",
		"protocol":  "http",
		"port":      443,
		"interval":  60,
		"enabled":   true,
		"res":       1200,
		"resAvg1h":  1300,
		"resMin1h":  900,
		"resMax1h":  1600,
		"loss1h":    5,
		"checkCert": true,
		"certInfo":  map[string]any{"expires": 1800000000000},
		"updated":   "2026-04-29 12:00:00.000Z",
	})

	newRecord := copyMonitorToNewRecord(oldRecord, "next12345")

	assert.Equal(t, "next12345", newRecord.Id)
	assert.Equal(t, "https://example.com", newRecord.GetString("target"))
	assert.Equal(t, "http", newRecord.GetString("protocol"))
	assert.Equal(t, 443, newRecord.GetInt("port"))
	assert.True(t, newRecord.GetBool("enabled"))
	assert.True(t, newRecord.GetBool("checkCert"))
	assert.Contains(t, []string{"", "null"}, newRecord.GetString("certInfo"))
	assert.Zero(t, newRecord.GetFloat("res"))
	assert.Zero(t, newRecord.GetFloat("resAvg1h"))
	assert.Zero(t, newRecord.GetFloat("resMin1h"))
	assert.Zero(t, newRecord.GetFloat("resMax1h"))
	assert.Zero(t, newRecord.GetFloat("loss1h"))
	assert.Equal(t, "", newRecord.GetString("updated"))
}

func TestNormalizeCertCheck(t *testing.T) {
	hub, testApp, err := createTestHub(t)
	require.NoError(t, err)
	defer cleanupTestHub(hub, testApp)

	collection, err := hub.FindCachedCollectionByNameOrId("network_monitors")
	require.NoError(t, err)
	cert := map[string]any{"expires": 1800000000000}
	for _, tc := range []struct {
		name, protocol, target string
		checkCert, wantCheck   bool
	}{
		{"https", "http", "https://example.com", true, true},
		{"uppercase scheme", "http", "HTTPS://example.com", true, true},
		{"plain http", "http", "http://example.com", true, false},
		{"tcp", "tcp", "example.com", true, false},
		{"disabled", "http", "https://example.com", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := core.NewRecord(collection)
			record.Load(map[string]any{"protocol": tc.protocol, "target": tc.target, "checkCert": tc.checkCert, "certInfo": cert})
			normalizeCertCheck(record)
			assert.Equal(t, tc.wantCheck, record.GetBool("checkCert"))
			assert.Equal(t, tc.wantCheck, monitorConfigFromRecord(record).CheckCert)
			assert.Equal(t, tc.wantCheck, record.GetString("certInfo") != "null")
		})
	}
}
