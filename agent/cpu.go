package agent

import (
	"math"
	"runtime"

	"github.com/shirou/gopsutil/v4/cpu"
)

var lastCpuTimes = make(map[uint16]cpu.TimesStat)

// init initializes the CPU monitoring by storing the initial CPU times
// for the default 60-second cache interval.
func init() {
	if times, err := cpu.Times(false); err == nil {
		lastCpuTimes[60000] = times[0]
	}
}

// CpuMetrics contains detailed CPU usage breakdown
type CpuMetrics struct {
	Total  float64
	User   float64
	System float64
	Iowait float64
	Steal  float64
}

// getCpuPercent calculates the CPU usage percentage using cached previous measurements.
// It uses the specified cache time interval to determine the time window for calculation.
// Returns the CPU usage percentage (0-100) and any error encountered.
func getCpuPercent(cacheTimeMs uint16) (float64, error) {
	times, err := cpu.Times(false)
	if err != nil || len(times) == 0 {
		return 0, err
	}
	// if cacheTimeMs is not in lastCpuTimes, use 60000 as fallback lastCpuTime
	if _, ok := lastCpuTimes[cacheTimeMs]; !ok {
		lastCpuTimes[cacheTimeMs] = lastCpuTimes[60000]
	}
	delta := calculateBusy(lastCpuTimes[cacheTimeMs], times[0])
	lastCpuTimes[cacheTimeMs] = times[0]
	return delta, nil
}

// getCpuMetrics calculates detailed CPU usage metrics using cached previous measurements.
// It returns percentages for total, user, system, iowait, and steal time.
func getCpuMetrics(cacheTimeMs uint16) (CpuMetrics, error) {
	times, err := cpu.Times(false)
	if err != nil || len(times) == 0 {
		return CpuMetrics{}, err
	}
	// if cacheTimeMs is not in lastCpuTimes, use 60000 as fallback lastCpuTime
	if _, ok := lastCpuTimes[cacheTimeMs]; !ok {
		lastCpuTimes[cacheTimeMs] = lastCpuTimes[60000]
	}

	t1 := lastCpuTimes[cacheTimeMs]
	t2 := times[0]

	t1All, t1Busy := getAllBusy(t1)
	t2All, t2Busy := getAllBusy(t2)

	totalDelta := t2All - t1All
	if totalDelta <= 0 {
		return CpuMetrics{}, nil
	}

	metrics := CpuMetrics{
		Total:  clampPercent((t2Busy - t1Busy) / totalDelta * 100),
		User:   clampPercent((t2.User - t1.User) / totalDelta * 100),
		System: clampPercent((t2.System - t1.System) / totalDelta * 100),
		Iowait: clampPercent((t2.Iowait - t1.Iowait) / totalDelta * 100),
		Steal:  clampPercent((t2.Steal - t1.Steal) / totalDelta * 100),
	}

	lastCpuTimes[cacheTimeMs] = times[0]
	return metrics, nil
}

// clampPercent ensures the percentage is between 0 and 100
func clampPercent(value float64) float64 {
	return math.Min(100, math.Max(0, value))
}

// calculateBusy calculates the CPU busy percentage between two time points.
// It computes the ratio of busy time to total time elapsed between t1 and t2,
// returning a percentage clamped between 0 and 100.
func calculateBusy(t1, t2 cpu.TimesStat) float64 {
	t1All, t1Busy := getAllBusy(t1)
	t2All, t2Busy := getAllBusy(t2)

	if t2Busy <= t1Busy {
		return 0
	}
	if t2All <= t1All {
		return 100
	}
	return math.Min(100, math.Max(0, (t2Busy-t1Busy)/(t2All-t1All)*100))
}

// getAllBusy calculates the total CPU time and busy CPU time from CPU times statistics.
// On Linux, it excludes guest and guest_nice time from the total to match kernel behavior.
// Returns total CPU time and busy CPU time (total minus idle and I/O wait time).
func getAllBusy(t cpu.TimesStat) (float64, float64) {
	tot := t.Total()
	if runtime.GOOS == "linux" {
		tot -= t.Guest     // Linux 2.6.24+
		tot -= t.GuestNice // Linux 3.2.0+
	}

	busy := tot - t.Idle - t.Iowait

	return tot, busy
}
