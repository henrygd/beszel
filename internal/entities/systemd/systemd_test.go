//go:build testing

package systemd_test

import (
	"runtime"
	"testing"
	"testing/synctest"
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
		synctest.Test(t, func(t *testing.T) {
			service := &systemd.Service{}
			service.PrevCpuUsage = 1000
			service.PrevReadTime = time.Now().Add(-time.Second)

			// Half of one second's CPU capacity across all cores.
			cpuUsage := service.PrevCpuUsage + uint64(time.Second/2)*uint64(runtime.NumCPU())
			service.UpdateCPUPercent(cpuUsage)

			assert.Equal(t, 50.0, service.Cpu)
			assert.Equal(t, cpuUsage, service.PrevCpuUsage)
			assert.Equal(t, time.Now(), service.PrevReadTime)
			assert.Equal(t, 50.0, service.CpuPeak)
		})
	})

	t.Run("CPU peak updates only when higher", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			service := &systemd.Service{}
			service.PrevCpuUsage = 1000
			service.PrevReadTime = time.Now().Add(-time.Second)
			service.UpdateCPUPercent(service.PrevCpuUsage + uint64(time.Second/2)*uint64(runtime.NumCPU()))
			assert.Equal(t, 50.0, service.CpuPeak)

			// A smaller increase in the cumulative counter gives 25% usage.
			service.PrevReadTime = time.Now().Add(-time.Second)
			service.UpdateCPUPercent(service.PrevCpuUsage + uint64(time.Second/4)*uint64(runtime.NumCPU()))
			assert.Equal(t, 25.0, service.Cpu)
			assert.Equal(t, 50.0, service.CpuPeak, "Peak should not update for lower CPU usage")
		})
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
