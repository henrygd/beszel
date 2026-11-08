//go:build testing

package agent

import (
	"runtime"
	"testing"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/stretchr/testify/assert"
)

func TestGetAllBusy(t *testing.T) {
	times := cpu.TimesStat{
		User: 1, System: 2, Idle: 3, Nice: 4, Iowait: 5,
		Irq: 6, Softirq: 7, Steal: 8, Guest: 9, GuestNice: 10,
	}
	wantTotal, wantBusy := 55.0, 47.0
	if runtime.GOOS == "linux" {
		wantTotal, wantBusy = 36, 28
	}
	total, busy := getAllBusy(times)
	assert.Equal(t, wantTotal, total)
	assert.Equal(t, wantBusy, busy)
	assert.InDelta(t, wantBusy/wantTotal*100, calculateBusy(cpu.TimesStat{}, times), 1e-10)
	assert.Zero(t, calculateBusy(times, times))
	assert.Zero(t, calculateBusy(times, cpu.TimesStat{}))
}

func TestCpuMetricsFromTimesRecoversAfterCounterRollback(t *testing.T) {
	const cacheTimeMs uint16 = 1234
	saved := lastCpuTimes
	t.Cleanup(func() { lastCpuTimes = saved })

	for _, tt := range []struct {
		name     string
		previous cpu.TimesStat
		current  cpu.TimesStat
		next     cpu.TimesStat
	}{
		{
			name:     "all counters reset",
			previous: cpu.TimesStat{User: 1000, System: 500, Idle: 8500},
			current:  cpu.TimesStat{User: 10, System: 5, Idle: 85},
			next:     cpu.TimesStat{User: 20, System: 10, Idle: 170},
		},
		{
			// TimesStat uses seconds: a 32-bit counter at 100 Hz wraps at 42949672.96.
			name:     "idle counter wraps",
			previous: cpu.TimesStat{User: 100, System: 50, Idle: 42949672.90},
			current:  cpu.TimesStat{User: 110, System: 55, Idle: 0.50},
			next:     cpu.TimesStat{User: 120, System: 60, Idle: 85.50},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			lastCpuTimes = map[uint16]cpu.TimesStat{cacheTimeMs: tt.previous}
			assert.Equal(t, CpuMetrics{}, cpuMetricsFromTimes(cacheTimeMs, tt.current))
			assert.Equal(t, tt.current, lastCpuTimes[cacheTimeMs], "a rollback must establish a new baseline")
			metrics := cpuMetricsFromTimes(cacheTimeMs, tt.next)
			assert.InDelta(t, 15, metrics.Total, 1e-10)
			assert.InDelta(t, 10, metrics.User, 1e-10)
			assert.InDelta(t, 5, metrics.System, 1e-10)
			assert.InDelta(t, 85, metrics.Idle, 1e-10)
		})
	}
}

func TestCpuMetricsFromTimesCacheIntervals(t *testing.T) {
	saved := lastCpuTimes
	t.Cleanup(func() { lastCpuTimes = saved })
	baseline := cpu.TimesStat{User: 10, System: 5, Idle: 85}
	lastCpuTimes = map[uint16]cpu.TimesStat{60000: baseline}

	// A new cache interval uses the default interval's baseline.
	current := cpu.TimesStat{User: 20, System: 10, Idle: 170}
	metrics := cpuMetricsFromTimes(1234, current)
	assert.InDelta(t, 15, metrics.Total, 1e-10)
	assert.Equal(t, baseline, lastCpuTimes[60000], "intervals must keep independent baselines")
	assert.Equal(t, current, lastCpuTimes[1234])

	// An unchanged sample must not divide by zero or stop later sampling.
	assert.Equal(t, CpuMetrics{}, cpuMetricsFromTimes(1234, current))
	metrics = cpuMetricsFromTimes(1234, cpu.TimesStat{User: 30, System: 15, Idle: 255})
	assert.InDelta(t, 15, metrics.Total, 1e-10)
}
