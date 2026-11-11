//go:build testing

package custommetrics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/fxamacker/cbor/v2"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixture is a data directory holding config.yml and metric files.
type fixture struct {
	t        *testing.T
	dir      string
	c        *Collector
	now      time.Time // file mtimes default to this
	warnings *[]string
}

// newFixture writes config.yml, with {dir} replaced by the data directory,
// and returns a collector for it.
func newFixture(t *testing.T, config string) *fixture {
	t.Helper()
	t.Setenv("CONFIG", "")
	t.Setenv("BESZEL_AGENT_CONFIG", "")
	f := &fixture{t: t, dir: t.TempDir(), now: time.Now().Truncate(time.Second), warnings: captureWarnings(t)}
	f.config(config)
	f.c = NewCollector(f.dir)
	require.NotNil(t, f.c)
	return f
}

func (f *fixture) config(content string) {
	f.t.Helper()
	path := filepath.Join(f.dir, "config.yml")
	info, err := os.Stat(path)
	mtime := f.now.Add(-time.Hour)
	if err == nil {
		mtime = info.ModTime().Add(time.Second)
	}
	writeConfig(f.t, path, strings.ReplaceAll(content, "{dir}", f.dir), mtime)
}

// write creates or replaces a file under the data directory with the given age.
func (f *fixture) write(name, content string, age time.Duration) string {
	f.t.Helper()
	path := filepath.Join(f.dir, name)
	require.NoError(f.t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(f.t, os.WriteFile(path, []byte(content), 0o644))
	mtime := f.now.Add(-age)
	require.NoError(f.t, os.Chtimes(path, mtime, mtime))
	return path
}

// collect advances the clock by step and collects at the given interval.
func (f *fixture) collect(cacheTimeMs uint16, step time.Duration) (map[string]float64, map[string]system.CustomMetricMeta) {
	f.now = f.now.Add(step)
	return f.c.Collect(cacheTimeMs, f.now)
}

func (f *fixture) warned(substr string) int {
	n := 0
	for _, w := range *f.warnings {
		if strings.Contains(w, substr) {
			n++
		}
	}
	return n
}

func TestCollectorInertWithoutConfig(t *testing.T) {
	t.Setenv("CONFIG", "")
	t.Setenv("BESZEL_AGENT_CONFIG", "")
	captureWarnings(t)
	assert.Nil(t, NewCollector(""), "no data directory and no CONFIG")

	c := NewCollector(t.TempDir())
	require.NotNil(t, c)
	values, meta := c.Collect(60000, time.Now())
	assert.Nil(t, values)
	assert.Nil(t, meta)
}

func TestCollectorNoSeries(t *testing.T) {
	f := newFixture(t, "metrics:\n  sources:\n    - path: {dir}/metrics.d\n")
	require.NoError(t, os.Mkdir(filepath.Join(f.dir, "metrics.d"), 0o755))
	values, meta := f.collect(60000, 0)
	assert.Nil(t, values, "nil, so omitempty elides the field")
	assert.Nil(t, meta)
}

func TestCollectorGaugesAndMetadata(t *testing.T) {
	f := newFixture(t, `metrics:
  sources:
    - path: {dir}/power.prom
      display_names:
        pi_power_board_watts: Board power
`)
	longHelp := strings.Repeat("é", 250)
	f.write("power.prom", `# HELP pi_power_board_watts Board power, summed over all PMIC rails.
# TYPE pi_power_board_watts gauge
# UNIT pi_power_board_watts watts
pi_power_board_watts 1.84
# HELP pi_power_wall_estimate_watts `+longHelp+`
pi_power_wall_estimate_watts 2.668
disk_temp_celsius{device="sda"} 41
queue_depth 7
`, 0)

	values, meta := f.collect(60000, 0)
	assert.Equal(t, map[string]float64{
		"pi_power_board_watts":         1.84,
		"pi_power_wall_estimate_watts": 2.668,
		"disk_temp_celsius_sda":        41,
		"queue_depth":                  7,
	}, values)
	assert.Equal(t, map[string]system.CustomMetricMeta{
		"pi_power_board_watts": {
			Unit: "watts", Help: "Board power, summed over all PMIC rails.", DisplayName: "Board power", Chart: "Board power",
		},
		"pi_power_wall_estimate_watts": {
			Unit: "watts", Help: strings.Repeat("é", 200), DisplayName: "pi_power_wall_estimate_watts", Chart: "Pi power wall estimate watts",
		},
		"disk_temp_celsius_sda": {Unit: "celsius", DisplayName: "disk_temp_celsius_sda", Chart: "Disk temp celsius"},
		"queue_depth":           {DisplayName: "queue_depth", Chart: "Queue depth"},
	}, meta)
}

// Producers often write every # HELP and # TYPE line first and the values
// after. The metadata must still reach each metric.
func TestCollectorHeaderBlockMetadata(t *testing.T) {
	f := newFixture(t, "metrics:\n  sources:\n    - path: {dir}/m.prom\n")
	file := func(energy int) string {
		return fmt.Sprintf(`# HELP pi_power_board_watts Board power.
# TYPE pi_power_board_watts gauge
# HELP pi_energy_joules_total Energy used since boot.
# TYPE pi_energy_joules_total counter
# TYPE req_seconds histogram
pi_power_board_watts 1.84
pi_energy_joules_total %d
req_seconds_bucket{le="1"} 3
req_seconds_sum 2
req_seconds_count 3
`, energy)
	}
	f.write("m.prom", file(12345), 0)
	values, meta := f.collect(60000, 0)
	assert.Equal(t, map[string]float64{"pi_power_board_watts": 1.84}, values,
		"a counter's first observation has no rate, and histogram samples are skipped")
	assert.Equal(t, map[string]system.CustomMetricMeta{
		"pi_power_board_watts":   {Unit: "watts", Help: "Board power.", DisplayName: "pi_power_board_watts", Chart: "Pi power board watts"},
		"pi_energy_joules_total": {Unit: "joules/s", Help: "Energy used since boot.", DisplayName: "pi_energy_joules_total", Chart: "Pi energy joules"},
	}, meta)
	assert.Equal(t, 1, f.warned("Custom metric types not supported"))

	f.now = f.now.Add(time.Minute)
	f.write("m.prom", file(12405), 0)
	values, _ = f.collect(60000, 0)
	assert.Equal(t, map[string]float64{"pi_power_board_watts": 1.84, "pi_energy_joules_total": 1}, values)
}

func TestCollectorCounterRate(t *testing.T) {
	f := newFixture(t, "metrics:\n  sources:\n    - path: {dir}/c.prom\n")
	counter := func(v int) {
		f.write("c.prom", fmt.Sprintf("# TYPE jobs_total counter\njobs_total %d\n", v), 0)
	}

	counter(100)
	values, meta := f.collect(60000, 0)
	assert.NotContains(t, values, "jobs_total", "first observation emits nothing")
	assert.Equal(t, system.CustomMetricMeta{Unit: "/s", DisplayName: "jobs_total", Chart: "Jobs"}, meta["jobs_total"])

	f.now = f.now.Add(60 * time.Second)
	counter(220)
	values, _ = f.collect(60000, 0)
	assert.Equal(t, 2.0, values["jobs_total"], "120 over 60s")

	f.now = f.now.Add(60 * time.Second)
	counter(10)
	values, _ = f.collect(60000, 0)
	assert.NotContains(t, values, "jobs_total", "a reset emits nothing, never a negative spike")

	f.now = f.now.Add(30 * time.Second)
	counter(40)
	values, _ = f.collect(60000, 0)
	assert.Equal(t, 1.0, values["jobs_total"], "resumes after a reset")
}

// A producer that writes less often than the agent collects must read
// steadily, not 0 and then a spike: the rate is timed by its readings, and
// the result is repeated until it writes again or its reading goes stale.
func TestCollectorCounterSlowerThanCollection(t *testing.T) {
	tests := []struct {
		name        string
		cacheTimeMs uint16
		step        time.Duration // between collections
		every       int           // collections between producer writes
		increase    int           // per write
		maxAge      time.Duration
		rate, delta float64
	}{
		{"live view, written every 20s", 1000, time.Second, 20, 100, 2 * time.Minute, 5, 100},
		{"stored records, written every 5m", 60000, time.Minute, 5, 300, 15 * time.Minute, 1, 300},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, fmt.Sprintf(`metrics:
  max_age: %s
  sources:
    - path: {dir}/rate.prom
    - path: {dir}/delta.prom
      counters: delta
`, tt.maxAge))
			total, lastWrite := 1000, f.now
			for i := range 2*tt.every + 2 {
				if i > 0 {
					f.now = f.now.Add(tt.step)
				}
				if i%tt.every == 0 {
					if i > 0 {
						total += tt.increase
					}
					f.write("rate.prom", fmt.Sprintf("# TYPE jobs_total counter\njobs_total %d\n", total), 0)
					f.write("delta.prom", fmt.Sprintf("# TYPE sent_total counter\nsent_total %d\n", total), 0)
					lastWrite = f.now
				}
				values, _ := f.collect(tt.cacheTimeMs, 0)
				if i < tt.every {
					assert.Empty(t, values, "nothing until the second reading (collection %d)", i)
				} else {
					assert.Equal(t, map[string]float64{"jobs_total": tt.rate, "sent_total": tt.delta}, values, "collection %d", i)
				}
			}

			// The producer stops: the result holds until its last reading goes stale.
			f.now = lastWrite.Add(tt.maxAge)
			values, _ := f.collect(tt.cacheTimeMs, 0)
			assert.Equal(t, map[string]float64{"jobs_total": tt.rate, "sent_total": tt.delta}, values)
			values, _ = f.collect(tt.cacheTimeMs, time.Second)
			assert.Empty(t, values)
		})
	}
}

// A counter's readings are timed by their timestamps, else their file's mtime.
func TestCollectorCounterReadingTimes(t *testing.T) {
	f := newFixture(t, "metrics:\n  sources:\n    - path: {dir}/c.prom\n")
	start := f.now
	collect := func(value int, at time.Duration) (float64, bool) {
		t.Helper()
		f.write("c.prom", fmt.Sprintf("# TYPE n_total counter\nn_total %d %d\n", value, start.Add(at).UnixMilli()), 0)
		values, _ := f.collect(60000, 20*time.Second)
		rate, ok := values["n_total"]
		return rate, ok
	}

	_, ok := collect(100, 0)
	assert.False(t, ok, "first reading")
	rate, _ := collect(200, 20*time.Second)
	assert.Equal(t, 5.0, rate)

	// A new value with the same time, as on a filesystem with coarse mtimes,
	// is not a new reading: the result repeats, and the next rate is measured
	// from the reading before it.
	rate, _ = collect(250, 20*time.Second)
	assert.Equal(t, 5.0, rate)
	rate, _ = collect(400, 40*time.Second)
	assert.Equal(t, 10.0, rate, "200 to 400 over 20s")

	// A reading timed before the previous one starts over.
	_, ok = collect(500, 30*time.Second)
	assert.False(t, ok)
	rate, _ = collect(600, 50*time.Second)
	assert.Equal(t, 5.0, rate)
}

func TestCollectorCounterModes(t *testing.T) {
	f := newFixture(t, `metrics:
  counters: delta
  sources:
    - path: {dir}/delta.prom
    - path: {dir}/raw.prom
      counters: raw
    - path: {dir}/skip.prom
      counters: skip
    - path: {dir}/rate.prom
      counters: rate
`)
	write := func(v int) {
		for _, name := range []string{"delta", "raw", "skip", "rate"} {
			f.write(name+".prom", fmt.Sprintf("# TYPE %s_bytes_total counter\n%s_bytes_total %d\nplain_%s 1\n", name, name, v, name), 0)
		}
	}
	write(1000)
	values, _ := f.collect(60000, 0)
	assert.Equal(t, map[string]float64{"raw_bytes_total": 1000, "plain_delta": 1, "plain_raw": 1, "plain_skip": 1, "plain_rate": 1}, values)

	f.now = f.now.Add(10 * time.Second)
	write(1500)
	values, meta := f.collect(60000, 0)
	assert.Equal(t, 500.0, values["delta_bytes_total"])
	assert.Equal(t, 1500.0, values["raw_bytes_total"])
	assert.Equal(t, 50.0, values["rate_bytes_total"])
	assert.Equal(t, "bytes", meta["delta_bytes_total"].Unit)
	assert.Equal(t, "bytes", meta["raw_bytes_total"].Unit)
	assert.Equal(t, "bytes/s", meta["rate_bytes_total"].Unit)

	assert.NotContains(t, values, "skip_bytes_total")
	assert.NotContains(t, meta, "skip_bytes_total", "skip is like exclude: no metadata either")
	assert.Equal(t, 1.0, values["plain_skip"], "skip leaves gauges alone")
}

func TestCollectorKeyIgnoresFileName(t *testing.T) {
	f := newFixture(t, "metrics:\n  sources:\n    - path: {dir}/metrics.d\n")
	f.write("metrics.d/power.prom", "pi_power_board_watts 1.84\n", 0)
	before, _ := f.collect(60000, 0)

	require.NoError(t, os.Rename(filepath.Join(f.dir, "metrics.d/power.prom"), filepath.Join(f.dir, "metrics.d/pi5.prom")))
	after, _ := f.collect(60000, time.Second)
	assert.Equal(t, before, after)
	assert.Equal(t, map[string]float64{"pi_power_board_watts": 1.84}, after)
}

func TestCollectorCollisions(t *testing.T) {
	f := newFixture(t, `metrics:
  sources:
    - path: {dir}/second
    - path: {dir}/first
`)
	// Source order wins over file name order, then file order, then line order.
	f.write("first/a.prom", "shared 3\n", 0)
	f.write("second/z.prom", "shared 1\n", 0)
	f.write("second/a.prom", "shared 2\n", 0)
	f.write("second/b.prom", "disk{dev=\"sda_1\"} 10\ndisk{dev=\"sda\",part=\"1\"} 20\n", 0)

	for range 5 {
		values, _ := f.collect(60000, time.Second)
		assert.Equal(t, 2.0, values["shared"], "second/a.prom wins: first source, first file")
		assert.Equal(t, 10.0, values["disk_sda_1"], "the first line wins")
	}
	require.Equal(t, 3, f.warned("Custom metric series share keys with earlier series"), "warned once per file: %v", *f.warnings)
	second := filepath.Join(f.dir, "second")
	assert.Equal(t, 1, f.warned("file "+filepath.Join(second, "z.prom")+" series 1 first shared line 1 kept "+filepath.Join(second, "a.prom")+":1"))
	assert.Equal(t, 1, f.warned("file "+filepath.Join(second, "b.prom")+" series 1 first disk_sda_1 line 2 kept "+filepath.Join(second, "b.prom")+":1"))
	assert.Equal(t, 1, f.warned("file "+filepath.Join(f.dir, "first", "a.prom")+" series 1 first shared line 1"))
}

// A producer bug that writes every series twice loses half its lines, but
// logs one warning for the file, with a count, instead of one per line.
func TestCollectorCollisionsWarnOncePerFile(t *testing.T) {
	f := newFixture(t, "metrics:\n  max_series: 256\n  sources:\n    - path: {dir}/m.prom\n")
	var b strings.Builder
	for i := range 200 {
		fmt.Fprintf(&b, "q_%d 1\nq_%d 1\n", i, i)
	}
	path := f.write("m.prom", b.String(), 0)
	for range 3 {
		values, _ := f.collect(60000, time.Second)
		assert.Len(t, values, 200)
	}
	require.Len(t, *f.warnings, 1, "%v", *f.warnings)
	assert.Equal(t, "Custom metric series share keys with earlier series, keeping the first file "+path+
		" series 200 first q_0 line 2 kept "+path+":1", (*f.warnings)[0])
}

func TestCollectorPrefixResolvesCollision(t *testing.T) {
	f := newFixture(t, `metrics:
  sources:
    - path: {dir}/a.prom
      prefix: lab
    - path: {dir}/b.prom
      prefix: rack-2
`)
	f.write("a.prom", "temp_celsius 20\n", 0)
	f.write("b.prom", "temp_celsius 30\n", 0)
	values, meta := f.collect(60000, 0)
	assert.Equal(t, map[string]float64{"lab_temp_celsius": 20, "rack_2_temp_celsius": 30}, values)
	assert.Equal(t, "lab_temp_celsius", meta["lab_temp_celsius"].DisplayName)
	assert.Equal(t, "rack_2_temp_celsius", meta["rack_2_temp_celsius"].DisplayName)
	assert.Zero(t, f.warned("share keys"))
}

func TestCollectorStaleness(t *testing.T) {
	f := newFixture(t, `metrics:
  sources:
    - path: {dir}/fast.prom
      max_age: 60s
    - path: {dir}/slow.prom
      max_age: 10m
    - path: {dir}/stamped.prom
      max_age: 60s
`)
	f.write("fast.prom", "fast_value 1\n", 90*time.Second)
	f.write("slow.prom", "slow_value 2\n", 90*time.Second)
	f.write("stamped.prom", fmt.Sprintf("old_stamp 3 %d\nnew_stamp 4 %d\nno_stamp 5\n",
		f.now.Add(-2*time.Minute).UnixMilli(), f.now.Add(-time.Second).UnixMilli()), 0)
	require.NoError(t, os.Chtimes(filepath.Join(f.dir, "stamped.prom"), f.now, f.now))

	values, meta := f.collect(60000, 0)
	assert.Equal(t, map[string]float64{"slow_value": 2, "new_stamp": 4, "no_stamp": 5}, values,
		"a 60s source drops a 90s-old file a 10m source keeps; an explicit timestamp overrides a fresh mtime")
	assert.Contains(t, meta, "fast_value", "a stale series keeps its metadata")
	assert.Contains(t, meta, "old_stamp")

	// A fresh timestamp keeps a sample alive in an old file.
	f.write("fast.prom", fmt.Sprintf("fast_value 6 %d\n", f.now.UnixMilli()), time.Hour)
	values, _ = f.collect(60000, time.Second)
	assert.Equal(t, 6.0, values["fast_value"])

	// Removing the file drops the metadata too.
	require.NoError(t, os.Remove(filepath.Join(f.dir, "fast.prom")))
	_, meta = f.collect(60000, time.Second)
	assert.NotContains(t, meta, "fast_value")
}

// A timestamp more than a minute ahead of the agent's clock, such as one
// written in nanoseconds or by a producer whose clock runs fast, gives way to
// the file's mtime, so a stopped producer still goes stale.
func TestCollectorFutureTimestamps(t *testing.T) {
	f := newFixture(t, "metrics:\n  sources:\n    - path: {dir}/m.prom\n      max_age: 60s\n")
	path := f.write("m.prom", fmt.Sprintf("ns_stamp 1 %d\nskewed 2 %d\nnear 3 %d\n",
		f.now.UnixNano(), f.now.Add(time.Hour).UnixMilli(), f.now.Add(30*time.Second).UnixMilli()), 0)
	values, _ := f.collect(60000, 0)
	assert.Equal(t, map[string]float64{"ns_stamp": 1, "skewed": 2, "near": 3}, values)
	require.Len(t, *f.warnings, 1, "%v", *f.warnings)
	assert.Equal(t, "Custom metric timestamps ahead of the agent's clock, using the file's time; timestamps are in milliseconds file "+
		path+" series 2 first ns_stamp", (*f.warnings)[0])

	// The producer stops: the file and the near timestamp age, and every series goes.
	values, _ = f.collect(60000, 2*time.Minute)
	assert.Empty(t, values)
}

// A counter's rate is timed by the mtime in place of a timestamp far ahead,
// so it stays right: µs read as ms would make every rate 1,000 times too small.
func TestCollectorFutureTimestampCounterRate(t *testing.T) {
	f := newFixture(t, "metrics:\n  sources:\n    - path: {dir}/c.prom\n")
	write := func(v int) {
		f.write("c.prom", fmt.Sprintf("# TYPE jobs_total counter\njobs_total %d %d\n", v, f.now.UnixMicro()), 0)
	}
	write(100)
	f.collect(60000, 0)
	f.now = f.now.Add(20 * time.Second)
	write(200)
	values, _ := f.collect(60000, 0)
	assert.Equal(t, 5.0, values["jobs_total"])
}

func TestCollectorStaleCounterRestartsCleanly(t *testing.T) {
	f := newFixture(t, "metrics:\n  sources:\n    - path: {dir}/c.prom\n      max_age: 60s\n")
	f.write("c.prom", "# TYPE n_total counter\nn_total 100\n", 0)
	f.collect(60000, 0)
	f.now = f.now.Add(time.Minute)
	f.write("c.prom", "# TYPE n_total counter\nn_total 160\n", 0)
	values, _ := f.collect(60000, 0)
	require.Equal(t, 1.0, values["n_total"])

	// The producer stops: two stale collections.
	values, _ = f.collect(60000, 2*time.Minute)
	assert.NotContains(t, values, "n_total")
	values, _ = f.collect(60000, time.Minute)
	assert.NotContains(t, values, "n_total")

	// It returns with a much higher value: no rate spans the gap.
	f.write("c.prom", "# TYPE n_total counter\nn_total 5000\n", 0)
	values, _ = f.collect(60000, 0)
	assert.NotContains(t, values, "n_total", "first fresh collection after a gap emits nothing")
	f.now = f.now.Add(time.Minute)
	f.write("c.prom", "# TYPE n_total counter\nn_total 5060\n", 0)
	values, _ = f.collect(60000, 0)
	assert.Equal(t, 1.0, values["n_total"])
}

// Interleaved 1s live-view and 60s stored-record collections each compute
// rates over their own interval, as the network trackers do.
func TestCollectorCountersPerInterval(t *testing.T) {
	f := newFixture(t, "metrics:\n  sources:\n    - path: {dir}/c.prom\n")
	total := 0
	tick := func(seconds int) {
		total += 10 * seconds // 10 per second
		f.now = f.now.Add(time.Duration(seconds) * time.Second)
		f.write("c.prom", fmt.Sprintf("# TYPE n_total counter\nn_total %d\n", total), 0)
	}

	tick(0)
	f.collect(60000, 0)
	f.collect(1000, 0)
	for range 60 {
		tick(1)
		values, _ := f.collect(1000, 0)
		require.Equal(t, 10.0, values["n_total"])
	}
	values, _ := f.collect(60000, 0)
	assert.Equal(t, 10.0, values["n_total"], "the 60s rate is not computed from 1s deltas")
	tick(60)
	values, _ = f.collect(60000, 0)
	assert.Equal(t, 10.0, values["n_total"])
	tick(1)
	values, _ = f.collect(1000, 0)
	assert.Equal(t, 10.0, values["n_total"], "the 1s interval measures from its own previous collection, 61s ago")
}

func TestCollectorIncludeExclude(t *testing.T) {
	f := newFixture(t, `metrics:
  sources:
    - path: {dir}/app.prom
      include: [myapp_queue_*, myapp_jobs_*]
      exclude: [myapp_jobs_debug_*]
`)
	app := f.write("app.prom", `myapp_queue_depth{queue="mail"} 4
myapp_jobs_running 2
myapp_jobs_debug_flag 1
myapp_other 9
# TYPE myapp_queue_latency_seconds histogram
myapp_queue_latency_seconds_bucket{le="1"} 3
myapp_queue_latency_seconds_count 3
`, 0)
	values, meta := f.collect(60000, 0)
	assert.Equal(t, map[string]float64{"myapp_queue_depth_mail": 4, "myapp_jobs_running": 2}, values)
	assert.Len(t, meta, 2)
	assert.Equal(t, 1, f.warned("Custom metric types not supported, skipped file "+app+" metrics 1 first myapp_queue_latency_seconds"))

	f.config(`metrics:
  sources:
    - path: {dir}/app.prom
      include: [myapp_jobs_*]
`)
	f.collect(60000, time.Second)
	assert.Equal(t, 1, f.warned("not supported"), "excluded families do not warn")
}

func TestCollectorDropsNonFinite(t *testing.T) {
	f := newFixture(t, "metrics:\n  sources:\n    - path: {dir}/v.prom\n")
	f.write("v.prom", "a NaN\nb +Inf\nc -Inf\nd 1\n", 0)
	values, meta := f.collect(60000, 0)
	assert.Equal(t, map[string]float64{"d": 1}, values)
	assert.Len(t, meta, 4)
}

// Finite readings can still give a change that overflows to ±Inf, which would
// make the hub store the whole stats record as null. Such a change reports nothing.
func TestCollectorDropsOverflowingCounterChange(t *testing.T) {
	for _, mode := range []string{"rate", "delta"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t, "metrics:\n  counters: "+mode+"\n  sources:\n    - path: {dir}/c.prom\n")
			f.write("c.prom", "# TYPE c counter\nc_total -1e308\n", 0)
			f.collect(60000, 0)

			f.now = f.now.Add(time.Minute)
			f.write("c.prom", "# TYPE c counter\nc_total 1e308\n", 0)
			values, meta := f.collect(60000, 0)
			assert.Empty(t, values)
			assert.Contains(t, meta, "c_total", "the series keeps its chart")

			values, _ = f.collect(60000, time.Second)
			assert.Empty(t, values, "not repeated while the producer is silent")
		})
	}
}

func TestCollectorSkipsBadLinesAndLargeFiles(t *testing.T) {
	f := newFixture(t, "metrics:\n  sources:\n    - path: {dir}/m.d\n")
	f.write("m.d/ok.prom", "a 1\nnot a sample\nb 2\n", 0)
	f.write("m.d/big.prom", "big 1\n#"+strings.Repeat("x", maxFileSize)+"\n", 0)
	values, _ := f.collect(60000, 0)
	assert.Equal(t, map[string]float64{"a": 1, "b": 2}, values)
	assert.Equal(t, 1, f.warned("Unparseable lines in custom metrics file skipped"))
	assert.Equal(t, 1, f.warned("Custom metrics file too large, skipped"))

	f.collect(60000, time.Minute)
	assert.Equal(t, 1, f.warned("Unparseable lines"), "rate-limited")
}

func TestCollectorMaxSeries(t *testing.T) {
	f := newFixture(t, "metrics:\n  max_series: 3\n  sources:\n    - path: {dir}/m.prom\n")
	f.write("m.prom", "e 5\nb 2\nd 4\na 1\nc 3\n", 0)
	for range 10 {
		values, meta := f.collect(60000, time.Second)
		assert.Equal(t, map[string]float64{"a": 1, "b": 2, "c": 3}, values)
		assert.Len(t, meta, 3)
	}
	assert.Equal(t, 1, f.warned("Too many custom metric series"))
}

func TestCollectorMaxSeriesCountsStaleSeries(t *testing.T) {
	f := newFixture(t, "metrics:\n  max_series: 1\n  sources:\n    - path: {dir}/m.prom\n      max_age: 60s\n")
	f.write("m.prom", "a 1\nb 2\n", 5*time.Minute)
	values, meta := f.collect(60000, 0)
	assert.Nil(t, values)
	assert.Equal(t, []string{"a"}, keysOf(meta))
}

func keysOf[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func TestCollectorSources(t *testing.T) {
	f := newFixture(t, `metrics:
  sources:
    - path: {dir}/missing
    - path: {dir}/dir
    - path: {dir}/single.txt
    - path: {dir}/globbed/*.metrics
`)
	f.write("dir/one.prom", "from_dir 1\n", 0)
	f.write("dir/notes.txt", "ignored_txt 1\n", 0)
	f.write("dir/power.prom.tmp123", "ignored_tmp 1\n", 0)
	f.write("dir/nested/deep.prom", "ignored_nested 1\n", 0)
	f.write("single.txt", "from_file 2\n", 0)
	f.write("globbed/a.metrics", "from_glob_a 3\n", 0)
	f.write("globbed/b.metrics", "from_glob_b 4\n", 0)
	f.write("globbed/c.prom", "ignored_glob 5\n", 0)

	values, _ := f.collect(60000, 0)
	assert.Equal(t, map[string]float64{"from_dir": 1, "from_file": 2, "from_glob_a": 3, "from_glob_b": 4}, values)
	// The path as configured; on Windows it mixes separators, so not filepath.Join.
	assert.Equal(t, 1, f.warned("Custom metrics source not readable path "+f.dir+"/missing"))
}

// A source glob that cannot be parsed warns once an hour, and the other
// sources still report.
func TestCollectorInvalidGlobSource(t *testing.T) {
	f := newFixture(t, "metrics:\n  sources:\n    - path: {dir}/[\n    - path: {dir}/m.prom\n")
	f.write("m.prom", "m 1\n", 0)
	for range 2 {
		values, _ := f.collect(60000, time.Second)
		assert.Equal(t, map[string]float64{"m": 1}, values)
	}
	assert.Equal(t, 1, f.warned("Invalid custom metrics source glob"))
}

// One file read through two sources: its series are claimed by the first
// source, whose settings apply, with one warning per key. With disjoint
// include patterns, two sources can split a file between two charts.
func TestCollectorOverlappingSources(t *testing.T) {
	f := newFixture(t, `metrics:
  sources:
    - path: {dir}/m.d
      chart:
        title: First
    - path: {dir}/m.d/*.prom
      chart:
        title: Second
    - path: {dir}/split.prom
      include: [power_*]
      chart:
        title: Power
    - path: {dir}/split.prom
      include: [temp_*]
      chart:
        title: Temperature
`)
	f.write("m.d/a.prom", "a 1\nb 2\n", 0)
	f.write("split.prom", "power_watts 3\ntemp_celsius 4\n", 0)
	values, meta := f.collect(60000, 0)
	assert.Equal(t, map[string]float64{"a": 1, "b": 2, "power_watts": 3, "temp_celsius": 4}, values)
	assert.Equal(t, "First", meta["a"].Chart)
	assert.Equal(t, "First", meta["b"].Chart)
	assert.Equal(t, "Power", meta["power_watts"].Chart)
	assert.Equal(t, "Temperature", meta["temp_celsius"].Chart)
	assert.Equal(t, 1, f.warned("Custom metric series share keys with earlier series"), "one for the file read twice, none for the split file")
	assert.Equal(t, 1, f.warned("file "+filepath.Join(f.dir, "m.d", "a.prom")+" series 2 first a line 1"))
}

func TestCollectorFileCap(t *testing.T) {
	f := newFixture(t, "metrics:\n  max_series: 256\n  sources:\n    - path: {dir}/a\n    - path: {dir}/b\n")
	for i := range 100 {
		f.write(fmt.Sprintf("a/%03d.prom", i), fmt.Sprintf("a_%03d 1\n", i), 0)
		f.write(fmt.Sprintf("b/%03d.prom", i), fmt.Sprintf("b_%03d 1\n", i), 0)
	}
	values, _ := f.collect(60000, 0)
	assert.Len(t, values, maxFiles)
	assert.Contains(t, values, "a_099")
	assert.Contains(t, values, "b_027")
	assert.NotContains(t, values, "b_028")
	assert.Equal(t, 1, f.warned("Too many custom metrics files"))
}

// Collection stops once it has keyed maxSamples samples, so a source full of
// series costs no more than the budget. max_series then keeps the first by
// key among the series read.
func TestCollectorSampleBudget(t *testing.T) {
	f := newFixture(t, "metrics:\n  max_series: 256\n  sources:\n    - path: {dir}/many.prom\n    - path: {dir}/later.prom\n")
	var b strings.Builder
	for i := range maxSamples + 100 {
		fmt.Fprintf(&b, "z_%04d 1\n", i)
	}
	f.write("many.prom", b.String(), 0)
	f.write("later.prom", "a 1\n", 0)
	values, meta := f.collect(60000, 0)
	assert.Len(t, values, 256)
	assert.Len(t, meta, 256)
	assert.Contains(t, values, "z_0255")
	assert.NotContains(t, values, "a", "the source after the budget is not read, though its key sorts first")
	assert.Equal(t, 1, f.warned(fmt.Sprintf("Too many custom metric samples, skipping the rest max %d", maxSamples)))
	assert.Equal(t, 1, f.warned("Too many custom metric series"))
}

// Collection stops before parsing a file that would take it past maxBytes, so
// files full of lines that never become series stay cheap to collect.
func TestCollectorByteBudget(t *testing.T) {
	f := newFixture(t, "metrics:\n  sources:\n    - path: {dir}/m.d\n")
	comment := "# " + strings.Repeat("x", 60<<10) + "\n"
	for i := range 5 {
		f.write(fmt.Sprintf("m.d/%d.prom", i), fmt.Sprintf("%sm_%d 1\n", comment, i), 0)
	}
	values, _ := f.collect(60000, 0)
	assert.Equal(t, map[string]float64{"m_0": 1, "m_1": 1, "m_2": 1, "m_3": 1}, values)
	assert.Equal(t, 1, f.warned(fmt.Sprintf("Too much custom metrics data, skipping the rest max_kib %d", maxBytes>>10)))
}

// readRegular returns a file up to its limit, with the info of the file read,
// and refuses a larger one without reading it.
func TestReadRegularTooLarge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.prom")
	require.NoError(t, os.WriteFile(path, []byte("0123456789"), 0o644))
	data, info, err := readRegular(path, 10)
	require.NoError(t, err)
	assert.Equal(t, "0123456789", string(data))
	assert.Equal(t, int64(10), info.Size())

	_, _, err = readRegular(path, 9)
	assert.ErrorIs(t, err, errTooLarge)
}

// Label values written in other scripts keep their series apart. Kept to ASCII, all
// three rooms flattened to room_temp_celsius, and two of the three were dropped.
func TestCollectorLabelValuesInAnyScript(t *testing.T) {
	f := newFixture(t, "metrics:\n  sources:\n    - path: {dir}/m.prom\n")
	f.write("m.prom", `room_temp_celsius{room="Кухня"} 21
room_temp_celsius{room="Спальня"} 19
room_temp_celsius{room="キッチン"} 22
`, 0)
	values, _ := f.collect(60000, 0)
	assert.Equal(t, map[string]float64{
		"room_temp_celsius_Кухня":   21,
		"room_temp_celsius_Спальня": 19,
		"room_temp_celsius_キッチン":    22,
	}, values)
	assert.Empty(t, *f.warnings)
}

// A key is capped in bytes, which a non-Latin key reaches in fewer characters. The
// warning's example is cut at a character boundary, so the log line stays valid UTF-8.
func TestCollectorLongKeyInAnyScript(t *testing.T) {
	f := newFixture(t, "metrics:\n  sources:\n    - path: {dir}/m.prom\n")
	// "mm_" is 3 bytes and each ж 2, so the cut at maxKeyLen falls inside a character.
	f.write("m.prom", fmt.Sprintf("mm{x=%q} 1\n", strings.Repeat("ж", maxKeyLen)), 0)
	values, _ := f.collect(60000, 0)
	assert.Empty(t, values)
	require.Len(t, *f.warnings, 1, "%v", *f.warnings)
	assert.True(t, utf8.ValidString((*f.warnings)[0]), "%q", (*f.warnings)[0])
	assert.Contains(t, (*f.warnings)[0], "first mm_жжж")
}

// Over-long keys drop their series, with one warning for the file, not one per line.
func TestCollectorLongKey(t *testing.T) {
	f := newFixture(t, "metrics:\n  sources:\n    - path: {dir}/m.prom\n")
	var b strings.Builder
	for i := range 400 {
		fmt.Fprintf(&b, "m{x=\"%s%03d\"} 1\n", strings.Repeat("v", maxKeyLen), i)
	}
	b.WriteString("short 2\n")
	path := f.write("m.prom", b.String(), 0)
	values, meta := f.collect(60000, 0)
	assert.Equal(t, map[string]float64{"short": 2}, values)
	assert.Len(t, meta, 1)
	require.Len(t, *f.warnings, 1, "%v", *f.warnings)
	first := "m_" + strings.Repeat("v", maxKeyLen-2) + "…"
	assert.Equal(t, "Custom metric keys too long, skipped file "+path+" series 400 first "+first+" line 1", (*f.warnings)[0])
}

// worstCaseMetadata collects max_series series whose every text field is
// longer than its cap and is written with filler, and whose keys are as long
// as allowed.
func worstCaseMetadata(t *testing.T, filler string) map[string]system.CustomMetricMeta {
	t.Helper()
	over := func(limit int) string { return strings.Repeat(filler, limit+10) }
	var file, names strings.Builder
	file.WriteString("# HELP m " + over(maxDescriptionLength) + "\n# TYPE m counter\n# UNIT m " + over(maxUnitLength) + "\n")
	for i := range maxMaxSeries {
		value := fmt.Sprintf("%03d", i) + strings.Repeat("v", maxKeyLen-len("m_")-3)
		fmt.Fprintf(&file, "m{l=%q} %d\n", value, i)
		fmt.Fprintf(&names, "        m_%s: %q\n", value, over(maxDisplayNameLength))
	}
	f := newFixture(t, fmt.Sprintf(`metrics:
  max_series: %d
  sources:
    - path: {dir}/m.prom
      chart:
        title: %q
        description: %q
      display_names:
%s`, maxMaxSeries, over(maxDisplayNameLength), over(maxDescriptionLength), names.String()))
	f.write("m.prom", file.String(), 0)
	_, meta := f.collect(60000, 0)
	require.Len(t, meta, maxMaxSeries)
	return meta
}

// Every metadata field is capped, so one producer file cannot inflate the
// systems.info record pushed to every browser. The bounds are the measured
// worst cases at the caps.
func TestCollectorMetadataIsBounded(t *testing.T) {
	for _, tt := range []struct {
		name, filler string
		maxBytes     int // for both CBOR and JSON
	}{
		{"ASCII", "x", 250_000},
		{"4-byte UTF-8", "𝄞", 850_000},
		{"escaped in JSON", "<", 1_250_000}, // encoding/json writes < as <
	} {
		t.Run(tt.name, func(t *testing.T) {
			meta := worstCaseMetadata(t, tt.filler)
			for key, m := range meta {
				assert.LessOrEqual(t, len(key), maxKeyLen)
				assert.Equal(t, maxUnitLength+len("/s"), utf8.RuneCountInString(m.Unit))
				assert.Equal(t, maxDescriptionLength, utf8.RuneCountInString(m.Help))
				assert.Equal(t, maxDisplayNameLength, utf8.RuneCountInString(m.DisplayName))
				assert.Equal(t, maxDisplayNameLength, utf8.RuneCountInString(m.Chart))
				assert.Equal(t, maxDescriptionLength, utf8.RuneCountInString(m.ChartDescription))
			}
			info := system.Info{CustomMetricsMeta: meta}
			encoded, err := cbor.Marshal(info)
			require.NoError(t, err)
			asJSON, err := json.Marshal(info)
			require.NoError(t, err)
			assert.LessOrEqual(t, len(encoded), tt.maxBytes, "CBOR")
			assert.LessOrEqual(t, len(asJSON), tt.maxBytes, "JSON")
		})
	}
}

// gatherStats caches responses that share Info by value, so every call must
// return new maps.
func TestCollectorReturnsFreshMaps(t *testing.T) {
	f := newFixture(t, "metrics:\n  sources:\n    - path: {dir}/m.prom\n")
	f.write("m.prom", "a 1\n", 0)
	values, meta := f.collect(60000, 0)
	values["a"] = 99
	values["injected"] = 1
	meta["a"] = system.CustomMetricMeta{DisplayName: "changed"}

	values2, meta2 := f.collect(60000, time.Second)
	assert.Equal(t, map[string]float64{"a": 1}, values2)
	assert.Equal(t, "a", meta2["a"].DisplayName)
	assert.Equal(t, 99.0, values["a"], "the earlier result is not touched either")
}

func TestCollectorConfigReload(t *testing.T) {
	f := newFixture(t, "metrics:\n  sources:\n    - path: {dir}/a.prom\n")
	f.write("a.prom", "a 1\n", 0)
	f.write("b.prom", "b 2\n", 0)
	values, _ := f.collect(60000, 0)
	assert.Equal(t, map[string]float64{"a": 1}, values)

	f.config("metrics:\n  sources:\n    - path: {dir}/a.prom\n    - path: {dir}/b.prom\n")
	values, _ = f.collect(60000, time.Second)
	assert.Equal(t, map[string]float64{"a": 1, "b": 2}, values, "a new source applies without a restart")

	f.config("metrics:\n  sources: [\n")
	values, _ = f.collect(60000, time.Second)
	assert.Equal(t, map[string]float64{"a": 1, "b": 2}, values, "a malformed edit keeps the last good config")

	require.NoError(t, os.Remove(filepath.Join(f.dir, "config.yml")))
	values, meta := f.collect(60000, time.Second)
	assert.Nil(t, values)
	assert.Nil(t, meta)
}

func TestCollectorWarningsResetOnReload(t *testing.T) {
	f := newFixture(t, "metrics:\n  sources:\n    - path: {dir}/missing\n")
	f.collect(60000, 0)
	f.collect(60000, time.Second)
	assert.Equal(t, 1, f.warned("Custom metrics source not readable"))

	f.config("metrics:\n  sources:\n    - path: {dir}/missing\n  max_age: 5m\n")
	f.collect(60000, time.Second)
	assert.Equal(t, 2, f.warned("Custom metrics source not readable"), "a new config version shows its problems at once")
}

func TestCollectorCharts(t *testing.T) {
	f := newFixture(t, `metrics:
  sources:
    - path: {dir}/power.prom
      chart:
        title: "  Power consumption  "
        description: Board, wall and core
      display_names:
        pi_power_board_watts: Board
    - path: {dir}/plug.prom
      chart:
        title: Power consumption
    - path: {dir}/other.prom
      display_names:
        single_watts: Single series
        net_receive_bytes_total_eth0: Ethernet
`)
	f.write("power.prom", "pi_power_board_watts 1.8\npi_power_wall_estimate_watts 2.6\npi_core_volts 0.9\n", 0)
	f.write("plug.prom", "plug_power_watts 9\n", 0)
	f.write("other.prom", "single_watts 1\n"+
		"# TYPE net_receive_bytes_total counter\n"+
		"net_receive_bytes_total{device=\"eth0\"} 5\nnet_receive_bytes_total{device=\"wlan0\"} 7\n", 0)

	_, meta := f.collect(60000, 0)
	charts := make(map[string]string, len(meta))
	for key, m := range meta {
		charts[key] = m.Chart
	}
	assert.Equal(t, map[string]string{
		// one chart for the whole source, whatever the units, merged with another source of the same title
		"pi_power_board_watts":         "Power consumption",
		"pi_power_wall_estimate_watts": "Power consumption",
		"pi_core_volts":                "Power consumption",
		"plug_power_watts":             "Power consumption",
		// otherwise one chart per metric: a single series takes its display name
		"single_watts": "Single series",
		// labelled series share their metric's chart, titled from the metric name
		"net_receive_bytes_total_eth0":  "Net receive bytes",
		"net_receive_bytes_total_wlan0": "Net receive bytes",
	}, charts)
	assert.Equal(t, "Board", meta["pi_power_board_watts"].DisplayName, "display names still name the lines")
	assert.Equal(t, "Board, wall and core", meta["pi_core_volts"].ChartDescription)
	assert.Equal(t, "Board, wall and core", meta["plug_power_watts"].ChartDescription, "shared by every source with the title")
	assert.Empty(t, meta["single_watts"].ChartDescription, "derived charts have none")
	assert.Equal(t, "Ethernet", meta["net_receive_bytes_total_eth0"].DisplayName)
}

// A source's display names refer to series as its file names them, without the
// prefix, and apply only to that source's series.
func TestCollectorDisplayNamesPerSource(t *testing.T) {
	f := newFixture(t, `metrics:
  sources:
    - path: {dir}/lab.prom
      prefix: lab
      display_names:
        temp_celsius: Lab temperature
        disk_celsius_sda: Lab disk
    - path: {dir}/rack.prom
      prefix: rack
`)
	f.write("lab.prom", "temp_celsius 20\ndisk_celsius{dev=\"sda\"} 30\n", 0)
	f.write("rack.prom", "temp_celsius 25\n", 0)
	_, meta := f.collect(60000, 0)
	assert.Equal(t, "Lab temperature", meta["lab_temp_celsius"].DisplayName)
	assert.Equal(t, "Lab temperature", meta["lab_temp_celsius"].Chart, "a single series' display name titles its chart")
	assert.Equal(t, "Lab disk", meta["lab_disk_celsius_sda"].DisplayName)
	assert.Equal(t, "rack_temp_celsius", meta["rack_temp_celsius"].DisplayName, "another source's names do not apply")
}
