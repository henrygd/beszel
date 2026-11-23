package agent

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/henrygd/beszel/agent/utils"
)

// System memory bandwidth from memory controller PMUs (perf uncore events).
//
// Event discovery reads only sysfs (/sys/bus/event_source/devices). Each PMU
// publishes its perf type, the CPUs to open counters on (cpumask), the bit
// layout of its config fields (format/*), and usually named events with a
// scale and unit (events/*). Counters are opened once at startup; each stats
// request only reads the counters and converts the delta to GB/s.

const (
	memBwRead  = 0
	memBwWrite = 1

	// memBwMaxPlausibleBps drops samples whose computed rate is absurd, which
	// happens if a counter goes backwards (reset) instead of wrapping.
	memBwMaxPlausibleBps = 100e12
)

// memBwEventCandidates lists sysfs event names per direction, in preference order.
//
// Known gap: on Intel Xeon servers (Skylake-SP, Ice Lake-SP, Sapphire Rapids)
// the uncore_imc_N PMUs may publish no events/ directory; the CAS encodings
// live only in the perf tool's pmu-events JSON. Those PMUs are skipped rather
// than programmed with encodings that are not published in sysfs.
var memBwEventCandidates = [2][]string{
	memBwRead:  {"cas_count_read", "data_read", "data_reads", "read", "umc_cas_cmd.rd"},
	memBwWrite: {"cas_count_write", "data_write", "data_writes", "write", "umc_cas_cmd.wr"},
}

// amdUmcCasEvent is the UMC "CAS commands" event for AMD Zen 4/Zen 5 memory
// controllers, as defined in the AMD PPR and in the kernel perf tool's
// pmu-events (amdzen4/amdzen5 memory-controller.json: umc_cas_cmd.rd is
// event=0xa,rdwrmask=1 and umc_cas_cmd.wr is event=0xa,rdwrmask=2). The kernel
// does not publish named UMC events in sysfs, only the format fields, so this
// is used only when the PMU's format/ directory has both "event" and
// "rdwrmask". Each CAS command transfers one 64-byte cache line.
const (
	amdUmcCasEvent      = 0x0a
	amdUmcBytesPerCAS   = 64
	amdUmcRdWrMaskRead  = 1
	amdUmcRdWrMaskWrite = 2
)

// memBwCounter is one perf counter to open: a PMU event on a single CPU.
type memBwCounter struct {
	pmu           string
	pmuType       uint32
	cpu           int
	config        [3]uint64 // config, config1, config2
	bytesPerCount float64
	dir           int
}

// memBwReading is one counter read with PERF_FORMAT_TOTAL_TIME_ENABLED and
// PERF_FORMAT_TOTAL_TIME_RUNNING: the raw count plus the nanoseconds the event
// was enabled and actually counting (they differ if the PMU multiplexes).
type memBwReading struct {
	value   uint64
	enabled uint64
	running uint64
}

// memBwSample is a snapshot of all counter readings.
type memBwSample struct {
	counts []memBwReading
	at     time.Time
}

// memBandwidthManager tracks counter snapshots per cache interval.
type memBandwidthManager struct {
	counters []memBwCounter
	read     func() ([]memBwReading, error)
	now      func() time.Time
	prev     map[uint16]memBwSample
	latest   memBwSample
	// totalOnly marks sources with no read/write split: bw[0] is all traffic.
	totalOnly bool
}

// get returns [read, write] GB/s since the previous call for cacheKey.
// ok is false on the first call (no baseline) or when the read fails.
func (m *memBandwidthManager) get(cacheKey uint16) (bw [2]float64, ok bool) {
	counts, err := m.read()
	if err != nil {
		return bw, false
	}
	cur := memBwSample{counts: counts, at: m.now()}
	prev, found := m.prev[cacheKey]
	if !found {
		// Seed a new interval from the newest snapshot of any interval so the
		// first request for a new cache key can still report a rate.
		prev, found = m.latest, m.latest.counts != nil
	}
	m.prev[cacheKey] = cur
	m.latest = cur
	if !found {
		return bw, false
	}
	return memBwRates(m.counters, prev, cur)
}

// probeMemBwCounters fails when the counters do not advance over delay.
func probeMemBwCounters(read func() ([]memBwReading, error), delay time.Duration) error {
	before, err := read()
	if err != nil {
		return err
	}
	time.Sleep(delay)
	after, err := read()
	if err != nil {
		return err
	}
	for i := range after {
		if i < len(before) && after[i].value != before[i].value {
			return nil
		}
	}
	return fmt.Errorf("counters did not advance")
}

// memBwRates converts two counter snapshots into [read, write] GB/s (1e9 bytes).
// Counter deltas use modular uint64 arithmetic, so a wrap past 2^64 is
// handled; a counter that went backwards (reset) yields an implausible rate
// and the sample is dropped. Each delta is scaled by delta-enabled /
// delta-running to correct for multiplexing; a counter that did not run at all
// in the interval makes the sample unusable.
func memBwRates(counters []memBwCounter, prev, cur memBwSample) (bw [2]float64, ok bool) {
	if len(prev.counts) != len(counters) || len(cur.counts) != len(counters) {
		return bw, false
	}
	elapsed := cur.at.Sub(prev.at).Seconds()
	if elapsed <= 0 {
		return bw, false
	}
	var bytes [2]float64
	for i, c := range counters {
		p, q := prev.counts[i], cur.counts[i]
		running := q.running - p.running
		if running == 0 {
			return bw, false
		}
		enabled := q.enabled - p.enabled
		delta := float64(q.value-p.value) * float64(enabled) / float64(running)
		bytes[c.dir] += delta * c.bytesPerCount
	}
	// A measured [0, 0] is later dropped by omitzero and reads as no data.
	// That is acceptable: idle memory traffic is far above the 0.01 GB/s
	// rounding step, so a real all-zero reading does not occur in practice.
	for dir := range bytes {
		bps := bytes[dir] / elapsed
		if bps > memBwMaxPlausibleBps || math.IsNaN(bps) {
			return [2]float64{}, false
		}
		bw[dir] = utils.TwoDecimals(bps / 1e9)
	}
	return bw, true
}

// discoverMemBwCounters resolves memory controller read/write events from a
// perf event_source devices directory (normally /sys/bus/event_source/devices).
func discoverMemBwCounters(root string) ([]memBwCounter, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var intelImc, intelFreeRunning, amdUmc []memBwCounter
	for _, entry := range entries {
		name := entry.Name()
		dir := filepath.Join(root, name)
		var target *[]memBwCounter
		switch {
		case strings.HasPrefix(name, "uncore_imc_free_running"):
			target = &intelFreeRunning
		case strings.HasPrefix(name, "uncore_imc"):
			target = &intelImc
		case strings.HasPrefix(name, "amd_umc_"):
			target = &amdUmc
		default:
			continue
		}
		counters, err := memBwPMUCounters(dir, name)
		if err != nil {
			continue
		}
		*target = append(*target, counters...)
	}
	// Free-running IMC counters observe the same traffic as the programmable
	// IMC PMUs, so use them only when the programmable ones have no CAS events.
	var counters []memBwCounter
	switch {
	case len(intelImc) > 0:
		counters = intelImc
	case len(intelFreeRunning) > 0:
		counters = intelFreeRunning
	default:
		counters = amdUmc
	}
	if len(counters) == 0 {
		return nil, fmt.Errorf("no memory controller PMU events found")
	}
	return counters, nil
}

// memBwPMUCounters returns read and write counters for one PMU, one per CPU in
// its cpumask. Both directions must resolve or the PMU is skipped.
func memBwPMUCounters(dir, name string) ([]memBwCounter, error) {
	pmuType, err := readUintFile(filepath.Join(dir, "type"))
	if err != nil {
		return nil, err
	}
	cpus, err := readCPUList(filepath.Join(dir, "cpumask"))
	if err != nil || len(cpus) == 0 {
		return nil, fmt.Errorf("%s: no cpumask", name)
	}
	formats, err := readPMUFormats(filepath.Join(dir, "format"))
	if err != nil {
		return nil, err
	}

	var resolved [2]struct {
		config        [3]uint64
		bytesPerCount float64
	}
	for dirIdx, candidates := range memBwEventCandidates {
		found := false
		for _, eventName := range candidates {
			config, bytesPerCount, err := readPMUEvent(filepath.Join(dir, "events"), eventName, formats)
			if err != nil {
				continue
			}
			resolved[dirIdx].config, resolved[dirIdx].bytesPerCount = config, bytesPerCount
			found = true
			break
		}
		if !found && strings.HasPrefix(name, "amd_umc_") {
			config, err := amdUmcCasConfig(formats, dirIdx)
			if err == nil {
				resolved[dirIdx].config, resolved[dirIdx].bytesPerCount = config, amdUmcBytesPerCAS
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("%s: no %s event", name, [2]string{"read", "write"}[dirIdx])
		}
	}

	counters := make([]memBwCounter, 0, len(cpus)*2)
	for _, cpu := range cpus {
		for dirIdx := range resolved {
			counters = append(counters, memBwCounter{
				pmu:           name,
				pmuType:       uint32(pmuType),
				cpu:           cpu,
				config:        resolved[dirIdx].config,
				bytesPerCount: resolved[dirIdx].bytesPerCount,
				dir:           dirIdx,
			})
		}
	}
	return counters, nil
}

func amdUmcCasConfig(formats map[string]pmuFormat, dir int) ([3]uint64, error) {
	mask := uint64(amdUmcRdWrMaskRead)
	if dir == memBwWrite {
		mask = amdUmcRdWrMaskWrite
	}
	return encodePMUTerms(map[string]uint64{"event": amdUmcCasEvent, "rdwrmask": mask}, formats)
}

// pmuFormat describes where a named term's bits live: which config field
// (0=config, 1=config1, 2=config2) and which bit ranges, low bits first.
type pmuFormat struct {
	field  int
	ranges [][2]uint // inclusive [lo, hi]
}

// readPMUFormats parses format/* files such as "config:0-7" or "config1:0-3,8".
func readPMUFormats(dir string) (map[string]pmuFormat, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	formats := make(map[string]pmuFormat, len(entries))
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		if f, err := parsePMUFormat(strings.TrimSpace(string(data))); err == nil {
			formats[entry.Name()] = f
		}
	}
	return formats, nil
}

func parsePMUFormat(spec string) (pmuFormat, error) {
	var f pmuFormat
	fieldName, bits, ok := strings.Cut(spec, ":")
	if !ok {
		return f, fmt.Errorf("invalid format %q", spec)
	}
	switch fieldName {
	case "config":
		f.field = 0
	case "config1":
		f.field = 1
	case "config2":
		f.field = 2
	default:
		return f, fmt.Errorf("unsupported format field %q", fieldName)
	}
	for part := range strings.SplitSeq(bits, ",") {
		loStr, hiStr, isRange := strings.Cut(part, "-")
		lo, err := strconv.ParseUint(loStr, 10, 8)
		if err != nil {
			return f, err
		}
		hi := lo
		if isRange {
			if hi, err = strconv.ParseUint(hiStr, 10, 8); err != nil {
				return f, err
			}
		}
		if hi < lo || hi > 63 {
			return f, fmt.Errorf("invalid bit range %q", part)
		}
		f.ranges = append(f.ranges, [2]uint{uint(lo), uint(hi)})
	}
	return f, nil
}

// encodePMUTerms packs term values into config fields using the PMU formats.
func encodePMUTerms(terms map[string]uint64, formats map[string]pmuFormat) ([3]uint64, error) {
	var config [3]uint64
	for name, value := range terms {
		f, ok := formats[name]
		if !ok {
			return config, fmt.Errorf("unknown term %q", name)
		}
		remaining := value
		for _, r := range f.ranges {
			width := r[1] - r[0] + 1
			mask := uint64(1)<<width - 1
			if width == 64 {
				mask = math.MaxUint64
			}
			config[f.field] |= (remaining & mask) << r[0]
			if width == 64 {
				remaining = 0
			} else {
				remaining >>= width
			}
		}
		if remaining != 0 {
			return config, fmt.Errorf("value %#x does not fit term %q", value, name)
		}
	}
	return config, nil
}

// readPMUEvent parses events/<name> ("event=0x04,umask=0x03") plus its
// .scale and .unit files and returns the encoded config and bytes per count.
func readPMUEvent(dir, name string, formats map[string]pmuFormat) ([3]uint64, float64, error) {
	var config [3]uint64
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return config, 0, err
	}
	terms := make(map[string]uint64)
	for part := range strings.SplitSeq(strings.TrimSpace(string(data)), ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, valStr, hasVal := strings.Cut(part, "=")
		value := uint64(1)
		if hasVal {
			if value, err = strconv.ParseUint(valStr, 0, 64); err != nil {
				return config, 0, fmt.Errorf("event %s: %w", name, err)
			}
		}
		terms[key] = value
	}
	if config, err = encodePMUTerms(terms, formats); err != nil {
		return config, 0, err
	}

	scaleData, err := os.ReadFile(filepath.Join(dir, name+".scale"))
	if err != nil {
		return config, 0, fmt.Errorf("event %s: no scale", name)
	}
	scale, err := strconv.ParseFloat(strings.TrimSpace(string(scaleData)), 64)
	if err != nil || scale <= 0 {
		return config, 0, fmt.Errorf("event %s: invalid scale", name)
	}
	unitData, err := os.ReadFile(filepath.Join(dir, name+".unit"))
	if err != nil {
		return config, 0, fmt.Errorf("event %s: no unit", name)
	}
	unitBytes, ok := memBwUnitBytes(strings.TrimSpace(string(unitData)))
	if !ok {
		return config, 0, fmt.Errorf("event %s: unsupported unit", name)
	}
	return config, scale * unitBytes, nil
}

func memBwUnitBytes(unit string) (float64, bool) {
	switch unit {
	case "B", "Bytes", "bytes":
		return 1, true
	case "KiB":
		return 1 << 10, true
	case "MiB":
		return 1 << 20, true
	case "GiB":
		return 1 << 30, true
	case "KB":
		return 1e3, true
	case "MB":
		return 1e6, true
	case "GB":
		return 1e9, true
	}
	return 0, false
}

// readCPUList parses a kernel cpulist such as "0", "0,64" or "0-3,8".
func readCPUList(path string) ([]int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseCPUList(strings.TrimSpace(string(data)))
}

func parseCPUList(list string) ([]int, error) {
	var cpus []int
	if list == "" {
		return nil, nil
	}
	for part := range strings.SplitSeq(list, ",") {
		loStr, hiStr, isRange := strings.Cut(part, "-")
		lo, err := strconv.Atoi(loStr)
		if err != nil {
			return nil, err
		}
		hi := lo
		if isRange {
			if hi, err = strconv.Atoi(hiStr); err != nil {
				return nil, err
			}
		}
		if hi < lo {
			return nil, fmt.Errorf("invalid cpu range %q", part)
		}
		for cpu := lo; cpu <= hi; cpu++ {
			cpus = append(cpus, cpu)
		}
	}
	slices.Sort(cpus)
	return slices.Compact(cpus), nil
}

func readUintFile(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(data)), 10, 32)
}
