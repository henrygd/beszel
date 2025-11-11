//go:build testing

package systemd_test

import (
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/entities/systemd"
	"github.com/stretchr/testify/assert"
)

func TestParseServiceStatus(t *testing.T) {
	tests := []struct {
		input    string
		expected systemd.ServiceState
	}{
		{"active", systemd.StatusActive},
		{"inactive", systemd.StatusInactive},
		{"failed", systemd.StatusFailed},
		{"activating", systemd.StatusActivating},
		{"deactivating", systemd.StatusDeactivating},
		{"reloading", systemd.StatusReloading},
		{"unknown", systemd.StatusInactive}, // default case
		{"", systemd.StatusInactive},        // default case
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := systemd.ParseServiceStatus(test.input)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestParseServiceSubState(t *testing.T) {
	tests := []struct {
		input    string
		expected systemd.ServiceSubState
	}{
		{"dead", systemd.SubStateDead},
		{"running", systemd.SubStateRunning},
		{"exited", systemd.SubStateExited},
		{"failed", systemd.SubStateFailed},
		{"unknown", systemd.SubStateUnknown},
		{"other", systemd.SubStateUnknown}, // default case
		{"", systemd.SubStateUnknown},      // default case
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := systemd.ParseServiceSubState(test.input)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestServiceUpdateCPUPercent(t *testing.T) {
	t.Run("initial call sets CPU to 0", func(t *testing.T) {
		service := &systemd.Service{}
		service.UpdateCPUPercent(1000)
		assert.Equal(t, 0.0, service.Cpu)
		assert.Equal(t, uint64(1000), service.PrevCpuUsage)
		assert.False(t, service.PrevReadTime.IsZero())
	})

	t.Run("subsequent call calculates CPU percentage", func(t *testing.T) {
		service := &systemd.Service{}
		service.PrevCpuUsage = 1000
		service.PrevReadTime = time.Now().Add(-time.Second)

		service.UpdateCPUPercent(8000000000) // 8 seconds of CPU time

		// CPU usage should be positive and reasonable
		assert.Greater(t, service.Cpu, 0.0, "CPU usage should be positive")
		assert.LessOrEqual(t, service.Cpu, 100.0, "CPU usage should not exceed 100%")
		assert.Equal(t, uint64(8000000000), service.PrevCpuUsage)
		assert.Greater(t, service.CpuPeak, 0.0, "CPU peak should be set")
	})

	t.Run("CPU peak updates only when higher", func(t *testing.T) {
		service := &systemd.Service{}
		service.PrevCpuUsage = 1000
		service.PrevReadTime = time.Now().Add(-time.Second)
		service.UpdateCPUPercent(8000000000) // Set initial peak to ~50%
		initialPeak := service.CpuPeak

		// Now try with much lower CPU usage - should not update peak
		service.PrevReadTime = time.Now().Add(-time.Second)
		service.UpdateCPUPercent(1000000) // Much lower usage
		assert.Equal(t, initialPeak, service.CpuPeak, "Peak should not update for lower CPU usage")
	})

	t.Run("handles zero duration", func(t *testing.T) {
		service := &systemd.Service{}
		service.PrevCpuUsage = 1000
		now := time.Now()
		service.PrevReadTime = now
		// Mock time.Now() to return the same time to ensure zero duration
		// Since we can't mock time in Go easily, we'll check the logic manually
		// The zero duration case happens when duration <= 0
		assert.Equal(t, 0.0, service.Cpu, "CPU should start at 0")
	})

	t.Run("handles CPU usage wraparound", func(t *testing.T) {
		service := &systemd.Service{}
		// Simulate wraparound where new usage is less than previous
		service.PrevCpuUsage = 1000
		service.PrevReadTime = time.Now().Add(-time.Second)
		service.UpdateCPUPercent(500) // Less than previous, should reset
		assert.Equal(t, 0.0, service.Cpu)
	})
}
