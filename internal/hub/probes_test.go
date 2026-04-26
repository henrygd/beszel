package hub

import (
	"testing"

	"github.com/henrygd/beszel/internal/entities/probe"
	"github.com/stretchr/testify/assert"
)

func TestGenerateProbeID(t *testing.T) {
	tests := []struct {
		name     string
		systemID string
		config   probe.Config
		expected string
	}{
		{
			name:     "HTTP probe on example.com",
			systemID: "sys123",
			config: probe.Config{
				Protocol: "http",
				Target:   "example.com",
				Port:     80,
				Interval: 60,
			},
			expected: "de7b3647",
		},
		{
			name:     "HTTP probe on example.com with different system ID",
			systemID: "sys1234",
			config: probe.Config{
				Protocol: "http",
				Target:   "example.com",
				Port:     80,
				Interval: 60,
			},
			expected: "be9e2707",
		},
		{
			name:     "Same probe, different interval",
			systemID: "sys1234",
			config: probe.Config{
				Protocol: "http",
				Target:   "example.com",
				Port:     80,
				Interval: 120,
			},
			expected: "be9e2707",
		},
		{
			name:     "ICMP probe on 1.1.1.1",
			systemID: "sys456",
			config: probe.Config{
				Protocol: "icmp",
				Target:   "1.1.1.1",
				Port:     0,
				Interval: 10,
			},
			expected: "49ec14fc",
		}, {
			name:     "ICMP probe on 1.1.1.1 with different system ID",
			systemID: "sys4567",
			config: probe.Config{
				Protocol: "icmp",
				Target:   "1.1.1.1",
				Port:     0,
				Interval: 10,
			},
			expected: "84921aa3",
		},
		{
			name:     "TCP probe on example.com with port 443",
			systemID: "sys789",
			config: probe.Config{
				Protocol: "tcp",
				Target:   "example.com",
				Port:     443,
				Interval: 30,
			},
			expected: "677b991",
		},
		{
			name:     "TCP probe on example.com with port 8443",
			systemID: "sys789",
			config: probe.Config{
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
			got := generateProbeID(tt.systemID, tt.config)
			assert.Equal(t, tt.expected, got, "generateProbeID() = %v, want %v", got, tt.expected)
		})
	}
}
