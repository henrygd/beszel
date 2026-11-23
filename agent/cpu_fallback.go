package agent

// Android fallback CPU accounting for unrooted devices (Termux), where
// SELinux denies untrusted-app reads of /proc/stat and /proc/loadavg
// (henrygd/beszel#2346). gopsutil's cpu.Times / load.Avg therefore fail and
// the agent reports 0% CPU.
//
// Two data sources are tried, in order of quality:
//
//  1. Process ticks: sum utime+stime of every readable /proc/[pid]/stat,
//     converted to seconds via USER_HZ. These are genuine busy seconds with
//     no unit guessing — the same estimation trick Android's toybox `top`
//     uses when /proc/stat is unreadable. SELinux hides other UID's
//     processes from an unprivileged app, but for a Termux agent that is
//     exactly the workload being monitored (the agent itself, sshd,
//     tailscaled, ...).
//
//  2. cpufreq residency ratios: /sys/devices/system/cpu/cpuN/cpufreq/stats/
//     time_in_state, with busy ≈ 1 - (residency at the lowest frequency /
//     total residency). Only a fallback: kernels that clock-gate idle cores
//     stop accumulating time_in_state while the core sleeps, so all
//     powered-on time counts as busy and the display pegs near 100%.
//
// Both results are synthesized into cumulative cpu.TimesStat values so the
// existing delta logic (cpuMetricsFromTimes / calculateBusy) works unchanged.

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
)

// Hooks so tests can point at fixture paths / fake clocks.
var (
	androidProcStatPath  = "/proc/stat"
	androidSysCpuRoot    = "/sys/devices/system/cpu"
	androidProcPidRoot   = "/proc"
	androidClockNow      = time.Now
	androidClockTickRate = 100.0 // USER_HZ jiffies per second in /proc/*/stat
)

// androidNoProcStat is cached: true when /proc/stat is unreadable in this
// environment (unrooted Android), meaning gopsutil's default source can
// never work and the fallback should be used.
var androidNoProcStat = sync.OnceValue(func() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	_, statErr := os.ReadFile(androidProcStatPath)
	return statErr != nil
})

// androidCpuState keeps cumulative counters between samples so each call can
// produce monotonically increasing TimesStat values, matching gopsutil.
type androidCpuState struct {
	cumBusy float64 // cumulative busy seconds (all cores)
	cumWall float64 // cumulative wall-clock seconds × core count
	at      time.Time
}

var (
	androidMu      sync.Mutex
	androidTotal   androidCpuState   // for cpu.Times(false)
	androidPerCore []androidCpuState // per core, for cpu.Times(true)
)

// androidCpuTimes returns cumulative CPU-time counters synthesized from
// Android-readable sources, as a drop-in replacement for cpu.Times.
// perCore mirrors gopsutil semantics: true returns one TimesStat per core.
// Returns an empty slice on the first call (no delta yet), matching what
// beszel's baseline-seeding logic expects, and a non-nil error only when no
// data source works at all.
func androidCpuTimes(perCore bool) ([]cpu.TimesStat, error) {
	androidMu.Lock()
	defer androidMu.Unlock()

	now := androidClockNow()
	cores := runtime.NumCPU()
	if cores <= 0 {
		cores = 1
	}

	// Sample busy time per core. cpufreq data is per core; the process-tick
	// strategy yields one aggregate that we split evenly across cores.
	perCoreBusy := androidSampleBusyPerCore(now, cores)
	if len(perCoreBusy) == 0 {
		return nil, errNoAndroidCpuSource
	}

	if perCore {
		if len(perCoreBusy) != cores {
			// Align the per-core baseline with whatever the source exposes
			// (e.g. some cores lack cpufreq stats).
			if len(perCoreBusy) == 0 {
				return nil, errNoAndroidCpuSource
			}
			cores = len(perCoreBusy)
		}
		return androidBuildPerCore(perCoreBusy, now, cores), nil
	}
	return androidBuildTotal(perCoreBusy, now, cores), nil
}

// errNoAndroidCpuSource is returned when neither cpufreq nor process ticks
// are readable.
var errNoAndroidCpuSource = errors.New("no readable CPU data source (cpufreq/time_in_state, /proc/*/stat)")

// androidBuildTotal converts the per-core busy snapshot into a single
// aggregated cumulative TimesStat.
func androidBuildTotal(perCoreBusy []float64, now time.Time, cores int) []cpu.TimesStat {
	var busySum float64
	for _, b := range perCoreBusy {
		busySum += b
	}

	// Baseline per-core list must exist to seed wall time on first call.
	prevTotal := androidTotal
	if prevTotal.at.IsZero() {
		androidTotal = androidCpuState{cumBusy: busySum, cumWall: 0, at: now}
		return []cpu.TimesStat{}
	}

	elapsed := now.Sub(prevTotal.at).Seconds()
	if elapsed <= 0 {
		return []cpu.TimesStat{}
	}
	wallDelta := elapsed * float64(cores)
	busyDelta := clampAndroid(busySum-prevTotal.cumBusy, wallDelta)

	androidTotal = androidCpuState{
		cumBusy: prevTotal.cumBusy + busyDelta,
		cumWall: prevTotal.cumWall + wallDelta,
		at:      now,
	}
	cur := androidTotal
	return []cpu.TimesStat{{
		User: cur.cumBusy,
		Idle: cur.cumWall - cur.cumBusy,
	}}
}

// androidBuildPerCore converts the busy snapshot into per-core cumulative
// TimesStat entries. Cores without cpufreq data get the average busy share.
func androidBuildPerCore(perCoreBusy []float64, now time.Time, cores int) []cpu.TimesStat {
	if len(androidPerCore) != cores {
		// Core count changed (hotplug) or first call: reset baseline.
		androidPerCore = make([]androidCpuState, cores)
		for i := range androidPerCore {
			androidPerCore[i].at = now
			androidPerCore[i].cumBusy = perCoreBusy[i]
		}
		return []cpu.TimesStat{}
	}

	elapsed := now.Sub(androidPerCore[0].at).Seconds()
	if elapsed <= 0 {
		return []cpu.TimesStat{}
	}

	out := make([]cpu.TimesStat, cores)
	for i := 0; i < cores; i++ {
		prev := androidPerCore[i]
		wallDelta := elapsed
		busyDelta := clampAndroid(perCoreBusy[i]-prev.cumBusy, wallDelta)
		androidPerCore[i] = androidCpuState{
			cumBusy: prev.cumBusy + busyDelta,
			cumWall: prev.cumWall + wallDelta,
			at:      now,
		}
		cur := androidPerCore[i]
		out[i] = cpu.TimesStat{
			User: cur.cumBusy,
			Idle: cur.cumWall - cur.cumBusy,
		}
	}
	return out
}

// clampAndroid clamps a delta to [0, max].
func clampAndroid(delta, max float64) float64 {
	if delta < 0 {
		return 0
	}
	if delta > max {
		return max
	}
	return delta
}

// androidLastRaw keeps the previous raw cpufreq counters per core so busy
// time can be derived from unit-independent ratios instead of absolute
// counts (whose units and idle-accounting semantics vary by kernel).
type androidRawSamples struct {
	total []float64 // cumulative residency across all frequencies
	min   []float64 // cumulative residency at the lowest frequency
	ok    bool
	at    time.Time
}

var (
	androidLastRaw androidRawSamples
	androidCumBusy []float64 // accumulated busy seconds per core
)

// androidSourceLogged ensures the active fallback source is reported once.
var androidSourceLogged sync.Once

// androidSampleBusyPerCore reads cumulative busy seconds per core.
// Returns nil when no source is readable at all.
func androidSampleBusyPerCore(now time.Time, cores int) []float64 {
	// Strategy 1: process ticks, aggregated then split evenly. Genuine
	// busy seconds (utime/stime at USER_HZ), no unit guessing — matches
	// what toybox top reports in the same environment.
	if busy, found := androidProcessTicks(); found {
		androidSourceLogged.Do(func() {
			slog.Info("android CPU fallback: using process ticks (/proc/[pid]/stat)")
		})
		samples := make([]float64, cores)
		for i := range samples {
			samples[i] = busy / float64(cores)
		}
		return samples
	}

	// Strategy 2: cpufreq time_in_state residency ratios, per core.
	dirs := androidCpuDirs()
	if len(dirs) > 0 {
		totals := make([]float64, len(dirs))
		mins := make([]float64, len(dirs))
		ok := 0
		for i, dir := range dirs {
			t, m, got := androidTimeInStateBusy(dir)
			totals[i], mins[i] = t, m
			if got {
				ok++
			}
		}
		if ok > 0 {
			androidSourceLogged.Do(func() {
				slog.Info("android CPU fallback: using cpufreq residency ratios")
			})
			return androidAccumulateBusy(totals, mins, now)
		}
	}
	return nil
}

// androidAccumulateBusy converts raw cpufreq residency counters into
// cumulative busy seconds per core.
//
// Absolute time_in_state counts cannot be trusted as seconds: the unit is
// kernel-dependent (10 ms jiffies on most SoCs, other scales elsewhere) and
// many kernels accumulate idle time into the current frequency's bucket, so
// the raw total can equal or exceed wall-clock time (which made the first
// attempt peg the CPU display at 100%).
//
// Instead we use the standard Android frequency-ratio heuristic:
//
//	idle fraction ≈ residency at the LOWEST frequency / total residency
//	busy fraction = 1 - idle fraction
//
// The governor parks an idle core at its lowest frequency and raises the
// frequency under load, so the ratio tracks utilization regardless of the
// counter unit. Busy seconds = busy fraction × elapsed wall time.
func androidAccumulateBusy(totals, mins []float64, now time.Time) []float64 {
	sameShape := androidLastRaw.ok && len(androidLastRaw.total) == len(totals)
	elapsed := 0.0
	if sameShape {
		elapsed = now.Sub(androidLastRaw.at).Seconds()
		if elapsed <= 0 {
			// Clock went backwards or sampled too fast; keep prior cumulative.
			androidLastRaw.total, androidLastRaw.min, androidLastRaw.at = totals, mins, now
			return androidCumBusy
		}
	}
	if len(androidCumBusy) != len(totals) {
		androidCumBusy = make([]float64, len(totals))
	}
	if sameShape {
		for i := range totals {
			dTotal := totals[i] - androidLastRaw.total[i]
			dMin := mins[i] - androidLastRaw.min[i]
			usage := androidUsageRatio(dTotal, dMin)
			androidCumBusy[i] += usage * elapsed
		}
	}
	// First sample: baseline only; cumulative stays where it is and real
	// deltas begin on the next call, matching the seeding behavior of the
	// rest of the fallback.
	androidLastRaw.total, androidLastRaw.min, androidLastRaw.ok, androidLastRaw.at = totals, mins, true, now
	return androidCumBusy
}

// androidUsageRatio computes the busy fraction of one core from residency
// deltas: (total - lowest-freq) / total, clamped to [0,1]. A zero or
// negative total delta (idle-saturated counter or hotplug gap) reports 0.
func androidUsageRatio(dTotal, dMin float64) float64 {
	if dTotal <= 0 {
		return 0
	}
	idle := dMin / dTotal
	if idle < 0 {
		idle = 0
	}
	if idle > 1 {
		idle = 1
	}
	return 1 - idle
}

// androidCpuDirs lists /sys/devices/system/cpu/cpuN directories, sorted.
func androidCpuDirs() []string {
	entries, err := os.ReadDir(androidSysCpuRoot)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "cpu") {
			continue
		}
		if _, err := strconv.Atoi(strings.TrimPrefix(name, "cpu")); err != nil {
			continue
		}
		dirs = append(dirs, filepath.Join(androidSysCpuRoot, name))
	}
	sort.Strings(dirs)
	return dirs
}

// androidTimeInStateBusy reads one core's time_in_state file and returns
// (cumulative residency across all frequencies, cumulative residency at the
// lowest frequency, found). Units are kernel-internal and deliberately NOT
// converted to seconds — only ratios of these values are ever used.
func androidTimeInStateBusy(cpuDir string) (float64, float64, bool) {
	data, err := os.ReadFile(filepath.Join(cpuDir, "cpufreq", "stats", "time_in_state"))
	if err != nil {
		return 0, 0, false
	}
	var total, minFreq, minCount float64
	minFreq = -1
	found := false
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		freq, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			continue
		}
		count, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			continue
		}
		total += count
		if minFreq < 0 || freq < minFreq {
			minFreq = freq
			minCount = count
		}
		found = true
	}
	return total, minCount, found
}

// androidProcessTicks sums utime+stime (fields 14, 15) across every readable
// /proc/[pid]/stat, converted to seconds via USER_HZ.
func androidProcessTicks() (float64, bool) {
	entries, err := os.ReadDir(androidProcPidRoot)
	if err != nil {
		return 0, false
	}
	var total float64
	found := false
	for _, e := range entries {
		if !isPidName(e.Name()) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(androidProcPidRoot, e.Name(), "stat"))
		if err != nil {
			continue
		}
		// The comm field is parenthesized and may contain spaces; split after it.
		close := strings.LastIndexByte(string(data), ')')
		if close < 0 {
			continue
		}
		fields := strings.Fields(string(data)[close+1:])
		// After ')' the first field is state; utime/stime are fields 11/12
		// (0-based) of the remainder.
		if len(fields) < 13 {
			continue
		}
		utime, err1 := strconv.ParseFloat(fields[11], 64)
		stime, err2 := strconv.ParseFloat(fields[12], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		total += utime + stime
		found = true
	}
	return total / androidClockTickRate, found
}

// isPidName reports whether a /proc entry name is a numeric PID.
func isPidName(name string) bool {
	_, err := strconv.Atoi(name)
	return err == nil
}

// androidLoadAvg synthesizes load averages from the sysinfo(2) syscall when
// /proc/loadavg is unreadable. Kernel loads are fixed-point with
// SI_LOAD_SHIFT = 16 bits of fraction.
func androidLoadAvg() ([3]float64, bool) {
	var si syscall.Sysinfo_t
	if err := syscall.Sysinfo(&si); err != nil {
		return [3]float64{}, false
	}
	const siLoadShift = 16
	loads := [3]float64{}
	for i, raw := range si.Loads {
		if i > 2 {
			break
		}
		loads[i] = float64(uint64(raw)) / float64(uint64(1)<<siLoadShift)
	}
	slog.Debug("androidLoadAvg fallback", "load1", loads[0], "load5", loads[1])
	return loads, true
}
