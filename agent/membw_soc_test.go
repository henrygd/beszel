//go:build testing

package agent

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gpuMetricsV3(size int, format, content byte, read, write uint16) []byte {
	data := make([]byte, size)
	binary.LittleEndian.PutUint16(data[0:2], uint16(size))
	data[2], data[3] = format, content
	binary.LittleEndian.PutUint16(data[amdGPUMetricsV3DramReadOffset:], read)
	binary.LittleEndian.PutUint16(data[amdGPUMetricsV3DramWriteOffset:], write)
	return data
}

func TestParseAMDGPUMetricsV3Dram(t *testing.T) {
	raw, err := parseAMDGPUMetricsV3Dram(gpuMetricsV3(264, 3, 0, 4242, 1717))
	require.NoError(t, err)
	assert.Equal(t, [2]uint16{4242, 1717}, raw)

	for name, data := range map[string][]byte{
		"discrete v1.3": gpuMetricsV3(264, 1, 3, 1, 1),
		"apu v2.4":      gpuMetricsV3(264, 2, 4, 1, 1),
		"v3.1":          gpuMetricsV3(264, 3, 1, 1, 1),
		"short":         gpuMetricsV3(264, 3, 0, 1, 1)[:97],
	} {
		_, err := parseAMDGPUMetricsV3Dram(data)
		assert.Error(t, err, name)
	}
	bad := gpuMetricsV3(264, 3, 0, 1, 1)
	binary.LittleEndian.PutUint16(bad[0:2], 300)
	_, err = parseAMDGPUMetricsV3Dram(bad)
	assert.Error(t, err, "declared size beyond data")
}

func TestMemBwU16Unwrapper(t *testing.T) {
	var u memBwU16Unwrapper
	// Observed on Strix Halo: reads climb past 65535 and wrap, then fall back.
	steps := []struct {
		raw  uint16
		want int64
	}{
		{60000, 60000},
		{65513, 65513},
		{37, 65573},
		{9000, 74536},
		{65000, 65000}, // falls back below the wrap point
		{161, 65697},
		{64597, 64597},
		{60000, 60000},
	}
	for i, s := range steps {
		assert.Equal(t, s.want, u.next(s.raw), "step %d raw %d", i, s.raw)
	}
}

func TestMemBwU16UnwrapperStartedWrapped(t *testing.T) {
	// Started while true traffic was above 65.5 GB/s: under-reports until the
	// value crosses back down, then recovers without going negative.
	var u memBwU16Unwrapper
	assert.Equal(t, int64(10000), u.next(10000)) // true 75536
	assert.Equal(t, int64(100), u.next(100))     // true 65636
	assert.Equal(t, int64(65400), u.next(65400)) // true 65400
	assert.Equal(t, int64(65500), u.next(65500))
	assert.Equal(t, int64(65536+20), u.next(20))
}

func TestFindAMDAPUGPUMetrics(t *testing.T) {
	root := t.TempDir()
	write := func(card string, data []byte) {
		dir := filepath.Join(root, card, "device")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "gpu_metrics"), data, 0o644))
	}
	write("card0", gpuMetricsV3(264, 1, 3, 100, 100))       // discrete: VRAM, not system memory
	write("card1", gpuMetricsV3(264, 3, 0, 0xffff, 0xffff)) // fields unsupported
	_, err := findAMDAPUGPUMetrics(root)
	assert.Error(t, err)

	write("card2", gpuMetricsV3(264, 3, 0, 4000, 2000))
	path, err := findAMDAPUGPUMetrics(root)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "card2", "device", "gpu_metrics"), path)

	src := &amdAPUDramSource{path: path}
	bps, err := src.sample()
	require.NoError(t, err)
	assert.Equal(t, [2]float64{4e9, 2e9}, bps)
}

func TestTegraActmonSource(t *testing.T) {
	dir := t.TempDir()
	compat := filepath.Join(dir, "compatible")
	debugfs := filepath.Join(dir, "debug")
	require.NoError(t, os.MkdirAll(filepath.Join(debugfs, "cactmon"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(debugfs, "tegra_mc_utils"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(debugfs, tegraActmonMCAll), []byte("1252111\n"), 0o644))

	require.NoError(t, os.WriteFile(compat, []byte("nvidia,p3737-0000\x00nvidia,tegra194\x00"), 0o644))
	_, err := newTegraActmonSource(compat, debugfs)
	assert.Error(t, err, "Xavier is not calibrated")

	require.NoError(t, os.WriteFile(compat, []byte("nvidia,p3737-0000+p3701-0005\x00nvidia,p3701-0005\x00nvidia,tegra234\x00"), 0o644))
	_, err = newTegraActmonSource(compat, debugfs)
	assert.Error(t, err, "channel count missing")

	for _, bad := range []string{"0", "abc", "300"} {
		require.NoError(t, os.WriteFile(filepath.Join(debugfs, tegraMCNumChannel), []byte(bad), 0o644))
		_, err = newTegraActmonSource(compat, debugfs)
		assert.Error(t, err, bad)
	}

	// AGX Orin: 16 channels. 1252111 busy kHz measured at 39% EMC with an
	// 80.5 GB/s copy load.
	require.NoError(t, os.WriteFile(filepath.Join(debugfs, tegraMCNumChannel), []byte("16\n"), 0o644))
	src, err := newTegraActmonSource(compat, debugfs)
	require.NoError(t, err)
	bps, err := src.sample()
	require.NoError(t, err)
	assert.InDelta(t, 80.14e9, bps[memBwRead], 0.01e9)
	assert.Zero(t, bps[memBwWrite])

	// Orin Nano: 8 channels. 1149914 busy kHz with a 38.2 GB/s copy load.
	require.NoError(t, os.WriteFile(filepath.Join(debugfs, tegraMCNumChannel), []byte("8"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(debugfs, tegraActmonMCAll), []byte("1149914"), 0o644))
	src, err = newTegraActmonSource(compat, debugfs)
	require.NoError(t, err)
	bps, err = src.sample()
	require.NoError(t, err)
	assert.InDelta(t, 36.8e9, bps[memBwRead], 0.01e9)

	// no read/write split: the hub must label the reading as total traffic
	assert.True(t, src.manager().totalOnly)
}

func TestAMDAPUDramSourceReseedsAfterLostContinuity(t *testing.T) {
	for _, gap := range []string{"read error", "parse error", "long gap"} {
		t.Run(gap, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "gpu_metrics")
			write := func(read uint16) {
				require.NoError(t, os.WriteFile(path, gpuMetricsV3(264, 3, 0, read, 2000), 0o644))
			}
			src := &amdAPUDramSource{path: path}
			write(60000)
			_, err := src.sample()
			require.NoError(t, err)
			switch gap {
			case "read error":
				require.NoError(t, os.Remove(path))
				_, err = src.sample()
				require.Error(t, err)
			case "parse error":
				require.NoError(t, os.WriteFile(path, []byte("unavailable"), 0o644))
				_, err = src.sample()
				require.Error(t, err)
			case "long gap":
				src.lastAt = time.Now().Add(-2 * memBwMaxSampleGap)
			}
			// Traffic fell while samples were unavailable; this is not a wrap.
			write(10000)
			bps, err := src.sample()
			require.NoError(t, err)
			assert.Equal(t, [2]float64{10e9, 2e9}, bps)
		})
	}
}

func TestParseBandwidthLabel(t *testing.T) {
	for label, want := range map[string]float64{
		"   4GB/s": 4e9, "128GB/s": 128e9, "512MB/s": 512e6, "1.5 GB/s": 1.5e9, "64KB/s": 64e3,
	} {
		got, ok := parseBandwidthLabel(label)
		assert.True(t, ok, label)
		assert.Equal(t, want, got, label)
	}
	for _, label := range []string{"", "GB/s", "4 Gb/s", "-1GB/s", "idle"} {
		_, ok := parseBandwidthLabel(label)
		assert.False(t, ok, label)
	}
}

func TestMemBwRateSampler(t *testing.T) {
	rates := [][2]float64{{10e9, 4e9}, {30e9, 4e9}, {30e9, 8e9}}
	var errAt = -1
	i := 0
	s := &memBwRateSampler{sample: func() ([2]float64, error) {
		defer func() { i++ }()
		if i == errAt {
			return [2]float64{}, errors.New("read failed")
		}
		return rates[min(i, len(rates)-1)], nil
	}}
	t0 := time.Unix(1000, 0)
	s.tick(t0)
	s.tick(t0.Add(time.Second))     // trapezoid (10+30)/2, (4+4)/2
	s.tick(t0.Add(2 * time.Second)) // (30+30)/2, (4+8)/2
	assert.Equal(t, [2]uint64{50e9, 10e9}, s.bytes)
	assert.Equal(t, uint64(2*time.Second), s.ns)

	// A gap longer than memBwMaxSampleGap is not integrated.
	s.tick(t0.Add(10 * time.Second))
	assert.Equal(t, [2]uint64{50e9, 10e9}, s.bytes)
	assert.Equal(t, uint64(2*time.Second), s.ns)

	// A failed sample breaks the chain: the next good sample only re-seeds.
	errAt = i
	s.tick(t0.Add(10100 * time.Millisecond))
	s.tick(t0.Add(10200 * time.Millisecond))
	assert.Equal(t, [2]uint64{50e9, 10e9}, s.bytes)
	s.tick(t0.Add(10300 * time.Millisecond))
	assert.Equal(t, [2]uint64{53e9, 10.8e9}, s.bytes)
}

func TestMemBwRateSamplerClampsNegative(t *testing.T) {
	s := &memBwRateSampler{sample: func() ([2]float64, error) { return [2]float64{-5e9, 1e9}, nil }}
	t0 := time.Unix(1000, 0)
	s.tick(t0)
	s.tick(t0.Add(time.Second))
	assert.Equal(t, [2]uint64{0, 1e9}, s.bytes)
}

func TestMemBwRateSamplerIgnoresQueuedTicks(t *testing.T) {
	calls := 0
	s := &memBwRateSampler{sample: func() ([2]float64, error) {
		calls++
		return [2]float64{20e9, 5e9}, nil
	}}
	t0 := time.Unix(1000, 0)
	s.tick(t0)
	s.readings(t0.Add(150 * time.Millisecond))
	// The 100 ms timer was queued while readings took the sampler lock.
	s.tick(t0.Add(100 * time.Millisecond))
	s.tick(t0.Add(150 * time.Millisecond))
	s.tick(t0.Add(200 * time.Millisecond))
	assert.Equal(t, 3, calls, "stale ticks must not advance the source either")
	assert.Equal(t, [2]uint64{4e9, 1e9}, s.bytes)
	assert.Equal(t, uint64(200*time.Millisecond), s.ns)
}

func TestMemBwSamplerFeedsRates(t *testing.T) {
	s := &memBwRateSampler{sample: func() ([2]float64, error) { return [2]float64{20e9, 5e9}, nil }}
	t0 := time.Unix(1000, 0)
	prev := memBwSample{counts: s.readings(t0), at: t0}
	for k := 1; k <= 10; k++ {
		s.tick(t0.Add(time.Duration(k) * memBwSampleInterval))
	}
	cur := memBwSample{counts: s.readings(t0.Add(time.Second)), at: t0.Add(time.Second)}
	bw, ok := memBwRates(memBwByteCounters("test"), prev, cur)
	require.True(t, ok)
	assert.Equal(t, [2]float64{20, 5}, bw)

	// No samples in the interval: the reading is dropped, not reported as 0.
	same := memBwSample{counts: cur.counts, at: cur.at.Add(time.Second)}
	_, ok = memBwRates(memBwByteCounters("test"), cur, same)
	assert.False(t, ok)
}

func TestProbeMemBwCounters(t *testing.T) {
	stuck := func() ([]memBwReading, error) { return []memBwReading{{value: 7}, {value: 9}}, nil }
	assert.Error(t, probeMemBwCounters(stuck, 0), "PMU that opens but never counts")

	n := uint64(0)
	moving := func() ([]memBwReading, error) { n += 64; return []memBwReading{{value: 7}, {value: n}}, nil }
	assert.NoError(t, probeMemBwCounters(moving, 0))

	failing := func() ([]memBwReading, error) { return nil, errors.New("read") }
	assert.Error(t, probeMemBwCounters(failing, 0))
}
