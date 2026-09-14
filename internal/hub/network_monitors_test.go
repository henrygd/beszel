package hub

import (
	"testing"

	"github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	oldRecord := core.NewRecord(collection)
	oldRecord.Load(map[string]any{
		"system":   "sys123",
		"name":     "Example",
		"target":   "https://example.com",
		"protocol": "http",
		"port":     443,
		"interval": 60,
		"enabled":  true,
		"res":      1200,
		"resAvg1h": 1300,
		"resMin1h": 900,
		"resMax1h": 1600,
		"loss1h":   5,
		"updated":  "2026-04-29 12:00:00.000Z",
	})

	newRecord := copyMonitorToNewRecord(oldRecord, "next12345")

	assert.Equal(t, "next12345", newRecord.Id)
	assert.Equal(t, "Example", newRecord.GetString("name"))
	assert.Equal(t, "https://example.com", newRecord.GetString("target"))
	assert.Equal(t, "http", newRecord.GetString("protocol"))
	assert.Equal(t, 443, newRecord.GetInt("port"))
	assert.True(t, newRecord.GetBool("enabled"))
	assert.Zero(t, newRecord.GetFloat("res"))
	assert.Zero(t, newRecord.GetFloat("resAvg1h"))
	assert.Zero(t, newRecord.GetFloat("resMin1h"))
	assert.Zero(t, newRecord.GetFloat("resMax1h"))
	assert.Zero(t, newRecord.GetFloat("loss1h"))
	assert.Equal(t, "", newRecord.GetString("updated"))
}
