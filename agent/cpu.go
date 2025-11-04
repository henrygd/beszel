package agent

import (
	"math"
	"path/filepath"
	"runtime"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/process"
)

var lastCpuTimes = make(map[uint16]cpu.TimesStat)
var lastPerCoreCpuTimes = make(map[uint16][]cpu.TimesStat)
var lastProcessCpuTimes = make(map[uint16]map[int32]float64)
var lastProcessCpuSampleTime = make(map[uint16]time.Time)

// init initializes the CPU monitoring by storing the initial CPU times
// for the default 60-second cache interval.
func init() {
	if times, err := cpu.Times(false); err == nil {
		lastCpuTimes[60000] = times[0]
	}
	if perCoreTimes, err := cpu.Times(true); err == nil {
		lastPerCoreCpuTimes[60000] = perCoreTimes
	}
	if processes, err := process.Processes(); err == nil {
		snapshot := make(map[int32]float64, len(processes))
		for _, proc := range processes {
			if times, err := proc.Times(); err == nil {
				snapshot[proc.Pid] = times.Total()
			}
		}
		lastProcessCpuTimes[60000] = snapshot
		lastProcessCpuSampleTime[60000] = time.Now()
	}
}

// CpuMetrics contains detailed CPU usage breakdown
type CpuMetrics struct {
	Total  float64
	User   float64
	System float64
	Iowait float64
	Steal  float64
	Idle   float64
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

	t1All, _ := getAllBusy(t1)
	t2All, _ := getAllBusy(t2)

	totalDelta := t2All - t1All
	if totalDelta <= 0 {
		return CpuMetrics{}, nil
	}

	metrics := CpuMetrics{
		Total:  calculateBusy(t1, t2),
		User:   clampPercent((t2.User - t1.User) / totalDelta * 100),
		System: clampPercent((t2.System - t1.System) / totalDelta * 100),
		Iowait: clampPercent((t2.Iowait - t1.Iowait) / totalDelta * 100),
		Steal:  clampPercent((t2.Steal - t1.Steal) / totalDelta * 100),
		Idle:   clampPercent((t2.Idle - t1.Idle) / totalDelta * 100),
	}

	lastCpuTimes[cacheTimeMs] = times[0]
	return metrics, nil
}

// clampPercent ensures the percentage is between 0 and 100
func clampPercent(value float64) float64 {
	return math.Min(100, math.Max(0, value))
}

// getPerCoreCpuUsage calculates per-core CPU busy usage as integer percentages (0-100).
// It uses cached previous measurements for the provided cache interval.
func getPerCoreCpuUsage(cacheTimeMs uint16) (system.Uint8Slice, error) {
	perCoreTimes, err := cpu.Times(true)
	if err != nil || len(perCoreTimes) == 0 {
		return nil, err
	}

	// Initialize cache if needed
	if _, ok := lastPerCoreCpuTimes[cacheTimeMs]; !ok {
		lastPerCoreCpuTimes[cacheTimeMs] = lastPerCoreCpuTimes[60000]
	}

	lastTimes := lastPerCoreCpuTimes[cacheTimeMs]

	// Limit to the number of cores available in both samples
	length := len(perCoreTimes)
	if len(lastTimes) < length {
		length = len(lastTimes)
	}

	usage := make([]uint8, length)
	for i := 0; i < length; i++ {
		t1 := lastTimes[i]
		t2 := perCoreTimes[i]
		usage[i] = uint8(math.Round(calculateBusy(t1, t2)))
	}

	lastPerCoreCpuTimes[cacheTimeMs] = perCoreTimes
	return usage, nil
}

// getTopCpuProcess returns the process with the highest CPU usage since the last run
// for the given cache interval. It returns nil if insufficient data is available.
func getTopCpuProcess(cacheTimeMs uint16) (*system.TopCpuProcess, error) {
	processes, err := process.Processes()
	if err != nil {
		return nil, err
	}

	now := time.Now()

	lastTimes, ok := lastProcessCpuTimes[cacheTimeMs]
	if !ok {
		if fallback := lastProcessCpuTimes[60000]; fallback != nil {
			copied := make(map[int32]float64, len(fallback))
			for pid, total := range fallback {
				copied[pid] = total
			}
			lastTimes = copied
			lastProcessCpuTimes[cacheTimeMs] = copied
		} else {
			lastTimes = make(map[int32]float64)
			lastProcessCpuTimes[cacheTimeMs] = lastTimes
		}
	}

	lastSample := lastProcessCpuSampleTime[cacheTimeMs]
	if lastSample.IsZero() {
		if fallback := lastProcessCpuSampleTime[60000]; !fallback.IsZero() {
			lastSample = fallback
			lastProcessCpuSampleTime[cacheTimeMs] = fallback
		}
	}

	elapsed := now.Sub(lastSample).Seconds()
	if lastSample.IsZero() || elapsed <= 0 {
		snapshot := make(map[int32]float64, len(processes))
		for _, proc := range processes {
			if times, err := proc.Times(); err == nil {
				snapshot[proc.Pid] = times.Total()
			}
		}
		lastProcessCpuTimes[cacheTimeMs] = snapshot
		lastProcessCpuSampleTime[cacheTimeMs] = now
		return nil, nil
	}

	cpuCount := float64(runtime.NumCPU())
	if cpuCount <= 0 {
		cpuCount = 1
	}

	snapshot := make(map[int32]float64, len(processes))
	var topName string
	var topPercent float64

	for _, proc := range processes {
		times, err := proc.Times()
		if err != nil {
			continue
		}

		total := times.Total()
		pid := proc.Pid
		snapshot[pid] = total

		lastTotal, ok := lastTimes[pid]
		if !ok || total <= lastTotal {
			continue
		}

		percent := clampPercent((total - lastTotal) / (elapsed * cpuCount) * 100)
		if percent <= 0 {
			continue
		}

		name, err := proc.Name()
		if err != nil || name == "" {
			if exe, exeErr := proc.Exe(); exeErr == nil && exe != "" {
				name = filepath.Base(exe)
			}
		}
		if name == "" {
			continue
		}

		if percent > topPercent {
			topPercent = percent
			topName = name
		}
	}

	lastProcessCpuTimes[cacheTimeMs] = snapshot
	lastProcessCpuSampleTime[cacheTimeMs] = now

	if topName == "" {
		return nil, nil
	}

	return &system.TopCpuProcess{
		Name:    topName,
		Percent: topPercent,
	}, nil
}

// calculateBusy calculates the CPU busy percentage between two time points.
// It computes the ratio of busy time to total time elapsed between t1 and t2,
// returning a percentage clamped between 0 and 100.
func calculateBusy(t1, t2 cpu.TimesStat) float64 {
	t1All, t1Busy := getAllBusy(t1)
	t2All, t2Busy := getAllBusy(t2)

	if t2All <= t1All || t2Busy <= t1Busy {
		return 0
	}
	return clampPercent((t2Busy - t1Busy) / (t2All - t1All) * 100)
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
