//go:build testing

package agent

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writePMUFixture creates a fake /sys/bus/event_source/devices/<name> tree.
func writePMUFixture(t *testing.T, root, name string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(root, name, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content+"\n"), 0o644))
	}
}

func intelImcFixture(pmuType string) map[string]string {
	return map[string]string{
		"type":                         pmuType,
		"cpumask":                      "0,28",
		"format/event":                 "config:0-7",
		"format/umask":                 "config:8-15",
		"events/cas_count_read":        "event=0x04,umask=0x03",
		"events/cas_count_read.scale":  "6.103515625e-5",
		"events/cas_count_read.unit":   "MiB",
		"events/cas_count_write":       "event=0x04,umask=0x0c",
		"events/cas_count_write.scale": "6.103515625e-5",
		"events/cas_count_write.unit":  "MiB",
		"events/clockticks":            "event=0x00,umask=0x00",
	}
}

func freeRunningFixture() map[string]string {
	return map[string]string{
		"type":                    "25",
		"cpumask":                 "0",
		"format/event":            "config:0-7",
		"format/umask":            "config:8-15",
		"events/data_read":        "event=0xff,umask=0x20",
		"events/data_read.scale":  "6.103515625e-5",
		"events/data_read.unit":   "MiB",
		"events/data_write":       "event=0xff,umask=0x21",
		"events/data_write.scale": "6.103515625e-5",
		"events/data_write.unit":  "MiB",
	}
}

func amdUmcFixture(pmuType string) map[string]string {
	return map[string]string{
		"type":            pmuType,
		"cpumask":         "0",
		"format/event":    "config:0-7",
		"format/rdwrmask": "config:8-9",
	}
}

func TestDiscoverMemBwCountersIntelImc(t *testing.T) {
	root := t.TempDir()
	writePMUFixture(t, root, "uncore_imc_0", intelImcFixture("17"))
	writePMUFixture(t, root, "uncore_imc_1", intelImcFixture("18"))
	// Must be ignored: same traffic as the programmable IMC PMUs.
	writePMUFixture(t, root, "uncore_imc_free_running_0", freeRunningFixture())
	writePMUFixture(t, root, "cpu", map[string]string{"type": "4"})

	counters, err := discoverMemBwCounters(root)
	require.NoError(t, err)
	// 2 PMUs x 2 sockets (cpumask 0,28) x read/write
	require.Len(t, counters, 8)
	for _, c := range counters {
		assert.Contains(t, []string{"uncore_imc_0", "uncore_imc_1"}, c.pmu)
		assert.Contains(t, []int{0, 28}, c.cpu)
		assert.InDelta(t, 64.0, c.bytesPerCount, 1e-9)
		if c.dir == memBwRead {
			assert.Equal(t, uint64(0x0304), c.config[0])
		} else {
			assert.Equal(t, uint64(0x0c04), c.config[0])
		}
	}
	assert.Equal(t, uint32(17), counters[0].pmuType)
}

func TestDiscoverMemBwCountersIntelFreeRunningOnly(t *testing.T) {
	root := t.TempDir()
	writePMUFixture(t, root, "uncore_imc_free_running_0", freeRunningFixture())
	// Client IMC PMU without CAS events must not block the free-running fallback.
	writePMUFixture(t, root, "uncore_imc_0", map[string]string{
		"type": "20", "cpumask": "0", "format/event": "config:0-7", "events/gt_requests": "event=0x01",
	})

	counters, err := discoverMemBwCounters(root)
	require.NoError(t, err)
	require.Len(t, counters, 2)
	assert.Equal(t, "uncore_imc_free_running_0", counters[0].pmu)
	assert.Equal(t, uint64(0x20ff), counters[0].config[0])
	assert.Equal(t, uint64(0x21ff), counters[1].config[0])
}

func TestDiscoverMemBwCountersAmdUmc(t *testing.T) {
	root := t.TempDir()
	writePMUFixture(t, root, "amd_umc_0", amdUmcFixture("30"))
	writePMUFixture(t, root, "amd_umc_1", amdUmcFixture("31"))
	writePMUFixture(t, root, "amd_iommu_0", map[string]string{"type": "12", "cpumask": "0"})

	counters, err := discoverMemBwCounters(root)
	require.NoError(t, err)
	require.Len(t, counters, 4)
	for _, c := range counters {
		assert.Equal(t, 0, c.cpu)
		assert.InDelta(t, 64.0, c.bytesPerCount, 1e-9)
		if c.dir == memBwRead {
			assert.Equal(t, uint64(0x10a), c.config[0])
		} else {
			assert.Equal(t, uint64(0x20a), c.config[0])
		}
	}
}

func TestDiscoverMemBwCountersAmdUmcPrefersSysfsEvents(t *testing.T) {
	root := t.TempDir()
	files := amdUmcFixture("30")
	files["events/umc_cas_cmd.rd"] = "event=0x0a,rdwrmask=1"
	files["events/umc_cas_cmd.rd.scale"] = "64"
	files["events/umc_cas_cmd.rd.unit"] = "Bytes"
	writePMUFixture(t, root, "amd_umc_0", files)

	counters, err := discoverMemBwCounters(root)
	require.NoError(t, err)
	require.Len(t, counters, 2)
	assert.Equal(t, uint64(0x10a), counters[0].config[0])
	assert.Equal(t, uint64(0x20a), counters[1].config[0])
}

func TestDiscoverMemBwCountersUnsupported(t *testing.T) {
	t.Run("no memory controller PMU (this AMD host without amd_uncore)", func(t *testing.T) {
		root := t.TempDir()
		for _, name := range []string{"amd_iommu_0", "cpu", "power", "msr", "ibs_op"} {
			writePMUFixture(t, root, name, map[string]string{"type": "4", "cpumask": "0"})
		}
		_, err := discoverMemBwCounters(root)
		assert.Error(t, err)
	})
	t.Run("amd umc without rdwrmask format", func(t *testing.T) {
		root := t.TempDir()
		files := amdUmcFixture("30")
		delete(files, "format/rdwrmask")
		writePMUFixture(t, root, "amd_umc_0", files)
		_, err := discoverMemBwCounters(root)
		assert.Error(t, err)
	})
	t.Run("intel event without scale", func(t *testing.T) {
		root := t.TempDir()
		files := intelImcFixture("17")
		delete(files, "events/cas_count_write.scale")
		writePMUFixture(t, root, "uncore_imc_0", files)
		_, err := discoverMemBwCounters(root)
		assert.Error(t, err)
	})
	t.Run("empty cpumask", func(t *testing.T) {
		root := t.TempDir()
		files := amdUmcFixture("30")
		files["cpumask"] = ""
		writePMUFixture(t, root, "amd_umc_0", files)
		_, err := discoverMemBwCounters(root)
		assert.Error(t, err)
	})
	t.Run("missing root", func(t *testing.T) {
		_, err := discoverMemBwCounters(filepath.Join(t.TempDir(), "nope"))
		assert.Error(t, err)
	})
}

func TestParsePMUFormatAndEncode(t *testing.T) {
	f, err := parsePMUFormat("config:0-7,32-35")
	require.NoError(t, err)
	assert.Equal(t, 0, f.field)
	assert.Equal(t, [][2]uint{{0, 7}, {32, 35}}, f.ranges)

	f1, err := parsePMUFormat("config1:3")
	require.NoError(t, err)
	assert.Equal(t, 1, f1.field)

	formats := map[string]pmuFormat{"event": f, "edge": f1}
	config, err := encodePMUTerms(map[string]uint64{"event": 0xabc, "edge": 1}, formats)
	require.NoError(t, err)
	assert.Equal(t, uint64(0xbc)|uint64(0xa)<<32, config[0])
	assert.Equal(t, uint64(1)<<3, config[1])

	_, err = encodePMUTerms(map[string]uint64{"event": 0x1000}, formats)
	assert.Error(t, err, "value wider than the format must fail")
	_, err = encodePMUTerms(map[string]uint64{"unknown": 1}, formats)
	assert.Error(t, err)

	for _, bad := range []string{"config", "config3:0-7", "config:7-0", "config:0-64", "config:x"} {
		_, err := parsePMUFormat(bad)
		assert.Error(t, err, bad)
	}
}

func TestParseCPUList(t *testing.T) {
	cpus, err := parseCPUList("0-2,8,64")
	require.NoError(t, err)
	assert.Equal(t, []int{0, 1, 2, 8, 64}, cpus)
	cpus, err = parseCPUList("")
	require.NoError(t, err)
	assert.Empty(t, cpus)
	_, err = parseCPUList("3-1")
	assert.Error(t, err)
}

func testMemBwCounters() []memBwCounter {
	return []memBwCounter{
		{dir: memBwRead, bytesPerCount: 64},
		{dir: memBwWrite, bytesPerCount: 64},
		{dir: memBwRead, bytesPerCount: 64},
	}
}

// readings builds fully scheduled counter readings (enabled == running == ns).
func readings(ns uint64, values ...uint64) []memBwReading {
	r := make([]memBwReading, len(values))
	for i, v := range values {
		r[i] = memBwReading{value: v, enabled: ns, running: ns}
	}
	return r
}

func TestMemBwRates(t *testing.T) {
	counters := testMemBwCounters()
	t0 := time.Unix(1000, 0)

	t.Run("rate", func(t *testing.T) {
		prev := memBwSample{counts: readings(0, 0, 0, 0), at: t0}
		// 2 s: read 20e9 bytes over two controllers, write 5e9 bytes
		cur := memBwSample{counts: readings(2e9, 10e9/64, 5e9/64, 10e9/64), at: t0.Add(2 * time.Second)}
		bw, ok := memBwRates(counters, prev, cur)
		require.True(t, ok)
		assert.Equal(t, [2]float64{10, 2.5}, bw)
	})

	t.Run("uint64 wrap", func(t *testing.T) {
		prev := memBwSample{counts: readings(0, math.MaxUint64-99, 0, 0), at: t0}
		cur := memBwSample{counts: readings(1e9, 100, 0, 0), at: t0.Add(time.Second)}
		bw, ok := memBwRates(counters, prev, cur)
		require.True(t, ok)
		assert.InDelta(t, 200*64/1e9, bw[0], 0.01)
	})

	t.Run("counter reset is dropped", func(t *testing.T) {
		prev := memBwSample{counts: readings(0, 1e15, 0, 0), at: t0}
		cur := memBwSample{counts: readings(1e9, 5, 0, 0), at: t0.Add(time.Second)}
		_, ok := memBwRates(counters, prev, cur)
		assert.False(t, ok)
	})

	t.Run("no elapsed time", func(t *testing.T) {
		s := memBwSample{counts: readings(1e9, 1, 1, 1), at: t0}
		_, ok := memBwRates(counters, s, s)
		assert.False(t, ok)
	})

	t.Run("length mismatch", func(t *testing.T) {
		_, ok := memBwRates(counters, memBwSample{counts: readings(0, 1), at: t0}, memBwSample{counts: readings(1e9, 1, 2, 3), at: t0.Add(time.Second)})
		assert.False(t, ok)
	})

	t.Run("multiplexed counter is scaled", func(t *testing.T) {
		prev := memBwSample{counts: []memBwReading{
			{value: 0, enabled: 0, running: 0},
			{value: 0, enabled: 0, running: 0},
			{value: 0, enabled: 0, running: 0},
		}, at: t0}
		// 1 s: counter 0 only ran a quarter of the time, so its 1 GB is
		// scaled to 4 GB; counter 2 ran half the time, 1 GB -> 2 GB.
		cur := memBwSample{counts: []memBwReading{
			{value: 1e9 / 64, enabled: 1e9, running: 0.25e9},
			{value: 1e9 / 64, enabled: 1e9, running: 1e9},
			{value: 1e9 / 64, enabled: 1e9, running: 0.5e9},
		}, at: t0.Add(time.Second)}
		bw, ok := memBwRates(counters, prev, cur)
		require.True(t, ok)
		assert.Equal(t, [2]float64{6, 1}, bw)
	})

	t.Run("counter that never ran drops the sample", func(t *testing.T) {
		prev := memBwSample{counts: []memBwReading{
			{value: 0, enabled: 0, running: 5e8},
			{value: 0, enabled: 0, running: 0},
			{value: 0, enabled: 0, running: 0},
		}, at: t0}
		cur := memBwSample{counts: []memBwReading{
			{value: 0, enabled: 1e9, running: 5e8},
			{value: 1e9 / 64, enabled: 1e9, running: 1e9},
			{value: 1e9 / 64, enabled: 1e9, running: 1e9},
		}, at: t0.Add(time.Second)}
		_, ok := memBwRates(counters, prev, cur)
		assert.False(t, ok)
	})
}

func TestMemBandwidthManagerGet(t *testing.T) {
	now := time.Unix(1000, 0)
	counts := readings(0, 0, 0, 0)
	var readErr error
	m := &memBandwidthManager{
		counters: testMemBwCounters(),
		read: func() ([]memBwReading, error) {
			if readErr != nil {
				return nil, readErr
			}
			return append([]memBwReading(nil), counts...), nil
		},
		now:  func() time.Time { return now },
		prev: make(map[uint16]memBwSample),
	}

	// first tick: no baseline, no value
	_, ok := m.get(60000)
	assert.False(t, ok)

	now = now.Add(time.Second)
	counts = readings(1e9, 1e9/64, 1e9/64, 0)
	bw, ok := m.get(60000)
	require.True(t, ok)
	assert.Equal(t, [2]float64{1, 1}, bw)

	// a new cache key seeds from the latest snapshot of any key
	now = now.Add(time.Second)
	counts = readings(2e9, 2e9/64, 1e9/64, 0)
	bw, ok = m.get(1000)
	require.True(t, ok)
	assert.Equal(t, [2]float64{1, 0}, bw)

	// the original key measures from its own previous snapshot (2 s ago)
	now = now.Add(time.Second)
	counts = readings(3e9, 2e9/64, 1e9/64, 0)
	bw, ok = m.get(60000)
	require.True(t, ok)
	assert.Equal(t, [2]float64{0.5, 0}, bw)

	readErr = errors.New("boom")
	_, ok = m.get(60000)
	assert.False(t, ok)
}
