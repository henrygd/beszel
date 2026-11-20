package agent

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// System memory bandwidth from SoC telemetry, for hosts without usable memory
// controller PMUs:
//   - AMD APUs: amdgpu gpu_metrics v3.0 (Strix Point, Strix Halo) reports
//     average DRAM read and write bandwidth for the whole SoC, CPU included.
//   - NVIDIA Jetson Orin (tegra234): the central activity monitor (cactmon)
//     reports EMC busy cycles for all memory clients, without a read/write split.
//   - Apple silicon: IOReport PMP bandwidth histograms (membw_darwin.go).
//
// The Linux sources report a rate rather than a byte counter, so a background
// sampler integrates them into byte counters that plug into memBwRates.

const (
	// memBwSampleInterval is how often rate sources are polled. The amdgpu
	// DRAM fields are 16-bit MB/s values that wrap above 65.5 GB/s, and a wrap
	// is only detectable while the value moves less than half the range
	// (32.8 GB/s) between samples. Measured on Strix Halo under a burst copy
	// load: the write rate rose 36 GB/s within 1 s, so a 1 s interval misreads
	// a fast rise as a wrap, while the largest 100 ms step was 3.9 GB/s. The
	// cost is negligible: a gpu_metrics read takes about 8 us (served from the
	// driver's cached table), so 10 Hz is roughly 0.008% of one core. The
	// Jetson source does not wrap but shares this interval; its read is just
	// as cheap, so one interval keeps the sampler simple.
	memBwSampleInterval = 100 * time.Millisecond
	// memBwMaxSampleGap skips integration across longer gaps (suspend, stalls)
	// instead of extrapolating one rate over the whole gap.
	memBwMaxSampleGap = 2 * time.Second
)

// memBwRateSampler integrates an instantaneous [read, write] bytes/s source
// into cumulative byte counters using the trapezoid rule.
type memBwRateSampler struct {
	mu       sync.Mutex
	sample   func() ([2]float64, error)
	bytes    [2]uint64
	ns       uint64
	last     time.Time
	lastRate [2]float64
	have     bool
}

func (s *memBwRateSampler) tick(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// A queued ticker timestamp can precede a synchronous readings call.
	// Never rewind the baseline and integrate the same interval twice.
	if !now.After(s.last) {
		return
	}
	rate, err := s.sample()
	if err != nil {
		s.have = false
		return
	}
	for d := range rate {
		rate[d] = max(rate[d], 0)
	}
	if s.have {
		dt := now.Sub(s.last)
		if dt > 0 && dt <= memBwMaxSampleGap {
			for d := range rate {
				s.bytes[d] += uint64((rate[d] + s.lastRate[d]) / 2 * dt.Seconds())
			}
			s.ns += uint64(dt)
		}
	}
	s.last, s.lastRate, s.have = now, rate, true
}

// readings brings the integration up to now and returns the read and write
// byte counters. Enabled and running time are equal (no multiplexing); a
// running delta of zero means no samples landed in the interval.
func (s *memBwRateSampler) readings(now time.Time) []memBwReading {
	s.tick(now)
	s.mu.Lock()
	defer s.mu.Unlock()
	return []memBwReading{
		{value: s.bytes[memBwRead], enabled: s.ns, running: s.ns},
		{value: s.bytes[memBwWrite], enabled: s.ns, running: s.ns},
	}
}

// newMemBwSamplerManager wraps a rate source in a manager and starts polling it.
func newMemBwSamplerManager(name string, sample func() ([2]float64, error)) *memBandwidthManager {
	s := &memBwRateSampler{sample: sample}
	s.tick(time.Now())
	go func() {
		ticker := time.NewTicker(memBwSampleInterval)
		for now := range ticker.C {
			s.tick(now)
		}
	}()
	return &memBandwidthManager{
		counters: memBwByteCounters(name),
		read:     func() ([]memBwReading, error) { return s.readings(time.Now()), nil },
		now:      time.Now,
		prev:     make(map[uint16]memBwSample),
	}
}

// memBwByteCounters describes a source that exposes one read and one write
// byte counter.
func memBwByteCounters(name string) []memBwCounter {
	return []memBwCounter{
		{pmu: name, dir: memBwRead, bytesPerCount: 1},
		{pmu: name, dir: memBwWrite, bytesPerCount: 1},
	}
}

// AMD APU gpu_metrics (struct gpu_metrics_v3_0 in the kernel's
// kgd_pp_interface.h). The header is u16 structure_size, u8 format_revision,
// u8 content_revision; average_dram_reads and average_dram_writes follow 45
// u16 fields (temperatures, activities, energy accumulators are u16/u32
// aligned without padding) at byte offsets 94 and 96, in MB/s.
const (
	amdGPUMetricsV3DramReadOffset  = 94
	amdGPUMetricsV3DramWriteOffset = 96
	amdGPUMetricsV3MinSize         = amdGPUMetricsV3DramWriteOffset + 2
	amdGPUMetricsUnsupportedU16    = 0xffff
)

// parseAMDGPUMetricsV3Dram returns the raw DRAM [read, write] MB/s fields.
func parseAMDGPUMetricsV3Dram(data []byte) ([2]uint16, error) {
	var raw [2]uint16
	if len(data) < amdGPUMetricsV3MinSize {
		return raw, fmt.Errorf("gpu_metrics too short: %d bytes", len(data))
	}
	size := int(binary.LittleEndian.Uint16(data[0:2]))
	if size < amdGPUMetricsV3MinSize || size > len(data) {
		return raw, fmt.Errorf("gpu_metrics size %d invalid for %d bytes", size, len(data))
	}
	if data[2] != 3 || data[3] != 0 {
		return raw, fmt.Errorf("gpu_metrics v%d.%d unsupported", data[2], data[3])
	}
	raw[memBwRead] = binary.LittleEndian.Uint16(data[amdGPUMetricsV3DramReadOffset:])
	raw[memBwWrite] = binary.LittleEndian.Uint16(data[amdGPUMetricsV3DramWriteOffset:])
	return raw, nil
}

// memBwU16Unwrapper extends a 16-bit value that wraps modulo 65536. It
// assumes the true value changes by less than half the range between samples.
// The offset never goes negative, so an agent started while the value is
// already wrapped under-reports until the value next crosses the wrap point.
type memBwU16Unwrapper struct {
	prev   uint16
	offset int64
	have   bool
}

func (u *memBwU16Unwrapper) next(raw uint16) int64 {
	if u.have {
		switch delta := int64(raw) - int64(u.prev); {
		case delta < -1<<15:
			u.offset += 1 << 16
		case delta > 1<<15:
			u.offset -= 1 << 16
		}
	}
	u.prev, u.have = raw, true
	if u.offset < 0 {
		u.offset = 0
	}
	return int64(raw) + u.offset
}

// amdAPUDramSource reads SoC DRAM bandwidth from an APU's gpu_metrics file.
type amdAPUDramSource struct {
	path   string
	unwrap [2]memBwU16Unwrapper
	lastAt time.Time
}

func (a *amdAPUDramSource) sample() ([2]float64, error) {
	var bps [2]float64
	now := time.Now()
	if now.Sub(a.lastAt) > memBwMaxSampleGap {
		// A missed crossing cannot be distinguished from a load change. Seed
		// again instead of carrying a potentially false wrap offset forever.
		a.unwrap = [2]memBwU16Unwrapper{}
	}
	// Any read/parse failure also breaks continuity for the next sample.
	a.lastAt = time.Time{}
	data, err := os.ReadFile(a.path)
	if err != nil {
		return bps, err
	}
	raw, err := parseAMDGPUMetricsV3Dram(data)
	if err != nil {
		return bps, err
	}
	for d := range raw {
		bps[d] = float64(a.unwrap[d].next(raw[d])) * 1e6
	}
	a.lastAt = now
	return bps, nil
}

// findAMDAPUGPUMetrics returns the first gpu_metrics file under drmRoot
// (normally /sys/class/drm) in the APU v3.0 layout with populated DRAM fields.
// Discrete GPUs use other layouts whose DRAM fields describe VRAM, so the
// format check also keeps this source to system memory.
func findAMDAPUGPUMetrics(drmRoot string) (string, error) {
	paths, _ := filepath.Glob(filepath.Join(drmRoot, "card*", "device", "gpu_metrics"))
	seen := make(map[string]bool)
	for _, path := range paths {
		if real, err := filepath.EvalSymlinks(path); err == nil {
			if seen[real] {
				continue
			}
			seen[real] = true
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		raw, err := parseAMDGPUMetricsV3Dram(data)
		if err != nil || (raw[memBwRead] == amdGPUMetricsUnsupportedU16 && raw[memBwWrite] == amdGPUMetricsUnsupportedU16) {
			continue
		}
		return path, nil
	}
	return "", fmt.Errorf("no AMD APU gpu_metrics v3.0")
}

// parseBandwidthLabel parses an IOReport histogram bucket name such as
// "  4GB/s" or "512MB/s" into bytes/s.
func parseBandwidthLabel(label string) (float64, bool) {
	label = strings.TrimSpace(label)
	units := []struct {
		suffix string
		scale  float64
	}{{"GB/s", 1e9}, {"MB/s", 1e6}, {"KB/s", 1e3}}
	for _, u := range units {
		if num, ok := strings.CutSuffix(label, u.suffix); ok {
			v, err := strconv.ParseFloat(strings.TrimSpace(num), 64)
			if err != nil || v < 0 {
				return 0, false
			}
			return v * u.scale, true
		}
	}
	return 0, false
}

// Jetson Orin (tegra234) central actmon. cactmon/mc_all reports EMC busy
// cycles per millisecond (kHz of busy clock: busy fraction times the EMC clock
// in kHz). Each EMC clock moves two LPDDR5 transfers (double data rate) on
// every 16-bit channel, so bytes/s = mc_all * 1000 * 2 * channels * 2.
// Calibrated on AGX Orin (16 channels) and Orin Nano (8 channels) against
// memory copy loads and tegrastats EMC utilization.
const (
	tegraActmonMCAll       = "cactmon/mc_all"
	tegraMCNumChannel      = "tegra_mc_utils/num_channel"
	tegraBytesPerChanCycle = 2 * 2 // DDR transfers per clock * bytes per 16-bit channel
)

type tegraActmonSource struct {
	path          string
	bytesPerCount float64
}

func (t *tegraActmonSource) sample() ([2]float64, error) {
	data, err := os.ReadFile(t.path)
	if err != nil {
		return [2]float64{}, err
	}
	busyKHz, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return [2]float64{}, err
	}
	// No read/write split is available: the total is reported as read.
	return [2]float64{float64(busyKHz) * t.bytesPerCount, 0}, nil
}

// manager samples the source and marks its readings as total-only.
func (t *tegraActmonSource) manager() *memBandwidthManager {
	m := newMemBwSamplerManager("tegra_cactmon", t.sample)
	m.totalOnly = true
	return m
}

// newTegraActmonSource resolves the actmon source from a debugfs root
// (normally /sys/kernel/debug) on tegra234 devices listed in compatiblePath.
// The debugfs root is 0700, but the two files read here are world-readable,
// so a non-root agent only needs those directories bind-mounted into an
// otherwise empty /sys/kernel/debug, for example with a unit drop-in:
//
//	[Service]
//	TemporaryFileSystem=/sys/kernel/debug:ro,mode=0755
//	BindReadOnlyPaths=/sys/kernel/debug/cactmon /sys/kernel/debug/tegra_mc_utils
func newTegraActmonSource(compatiblePath, debugfsRoot string) (*tegraActmonSource, error) {
	compat, err := os.ReadFile(compatiblePath)
	if err != nil {
		return nil, err
	}
	if !bytes.Contains(compat, []byte("nvidia,tegra234")) {
		return nil, fmt.Errorf("not tegra234")
	}
	data, err := os.ReadFile(filepath.Join(debugfsRoot, tegraMCNumChannel))
	if err != nil {
		return nil, err
	}
	channels, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 8)
	if err != nil || channels == 0 || channels > 64 {
		return nil, fmt.Errorf("invalid memory channel count %q", strings.TrimSpace(string(data)))
	}
	t := &tegraActmonSource{
		path:          filepath.Join(debugfsRoot, tegraActmonMCAll),
		bytesPerCount: 1000 * tegraBytesPerChanCycle * float64(channels),
	}
	if _, err := t.sample(); err != nil {
		return nil, err
	}
	return t, nil
}
