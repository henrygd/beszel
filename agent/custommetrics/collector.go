package custommetrics

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/henrygd/beszel/internal/entities/system"
)

const (
	maxKeyLen   = 128      // bytes; a longer key drops the series
	maxFileSize = 64 << 10 // a larger file is skipped
	// Per collection, so a broad glob or a directory of large files cannot
	// stall collection under the agent lock: files read, the bytes in them,
	// and the samples keyed, four times the 256-series ceiling.
	maxFiles   = 128
	maxBytes   = 256 << 10
	maxSamples = 1024
	// How far ahead of the agent's clock a sample's timestamp may be before it
	// is ignored for its file's mtime.
	maxClockSkew = time.Minute
)

// Collector reads custom metrics from the sources declared in the agent's
// config file. It is safe for use from one goroutine at a time; the agent lock
// provides that.
type Collector struct {
	config configFile
	// counters holds, per collection interval, each counter's reading from
	// the previous collection, if it was fresh then. The 1 s live view and the
	// 60 s stored collections keep their own, so a stored rate covers the time
	// since the previous stored collection, not only the producer's last
	// write. (agent/deltatracker does not fit: it keeps no reading times.)
	counters map[uint16]map[string]counterReading
	warn     *warnLimiter
}

// counterReading is a counter's last reading at one collection interval, and
// the rate or delta reported for it.
type counterReading struct {
	value  float64
	at     time.Time // the sample's timestamp, else its file's mtime
	result float64   // change from the reading before: a rate or a delta
	ok     bool      // result is set; false for a first reading or a reset
}

// NewCollector returns a collector for config.yml in dataDir, or the file
// named by CONFIG. It returns nil when neither is available.
func NewCollector(dataDir string) *Collector {
	path, explicit := configPath(dataDir)
	if path == "" {
		return nil
	}
	return &Collector{
		config:   configFile{path: path, explicit: explicit},
		counters: make(map[uint16]map[string]counterReading),
		warn:     newWarnLimiter(),
	}
}

// Collect reads every source and returns the current values and metadata,
// keyed by series key. Both maps are freshly allocated on every call, or nil
// when there is nothing to report. Metadata is sent for every series in a
// file that is still present, even when its value is stale, so a stopped
// producer keeps its name and unit on its history.
func (c *Collector) Collect(cacheTimeMs uint16, now time.Time) (map[string]float64, map[string]system.CustomMetricMeta) {
	if c.config.refresh() {
		c.warn.reset()
	}
	cfg := c.config.cfg
	if cfg == nil {
		clear(c.counters)
		return nil, nil
	}

	r := collection{
		warn:     c.warn,
		previous: c.counters[cacheTimeMs],
		readings: make(map[string]counterReading),
		now:      now,
		values:   make(map[string]float64),
		meta:     make(map[string]system.CustomMetricMeta),
		owners:   make(map[string]string),
	}
	files := 0
sources:
	for i := range cfg.Sources {
		src := &cfg.Sources[i]
		for _, file := range c.sourceFiles(src, now) {
			if files == maxFiles {
				c.warn.warn(now, "files", "Too many custom metrics files, skipping the rest", "max", maxFiles)
				break sources
			}
			files++
			r.readFile(src, file)
			if r.full {
				break sources
			}
		}
	}
	// Only counters read fresh this time are kept, so no rate spans a stale gap.
	c.counters[cacheTimeMs] = r.readings

	if len(r.meta) > cfg.MaxSeries {
		keys := slices.Sorted(maps.Keys(r.meta))
		for _, key := range keys[cfg.MaxSeries:] {
			delete(r.meta, key)
			delete(r.values, key)
		}
		c.warn.warn(now, "max_series", "Too many custom metric series, dropping the rest by key order",
			"max_series", cfg.MaxSeries, "series", len(keys))
	}

	var values map[string]float64
	var meta map[string]system.CustomMetricMeta
	if len(r.values) > 0 {
		values = r.values
	}
	if len(r.meta) > 0 {
		meta = r.meta
	}
	return values, meta
}

// sourceFiles resolves a source to its files, sorted: a directory's *.prom
// files, a glob's matches, or the file itself.
func (c *Collector) sourceFiles(src *SourceConfig, now time.Time) []string {
	var files []string
	if strings.ContainsAny(src.Path, "*?[") {
		matches, err := filepath.Glob(src.Path)
		if err != nil {
			c.warn.warn(now, "missing:"+src.Path, "Invalid custom metrics source glob", "path", src.Path, "err", err)
			return nil
		}
		files = matches
	} else {
		info, err := os.Stat(src.Path)
		if err != nil {
			c.warn.warn(now, "missing:"+src.Path, "Custom metrics source not readable", "path", src.Path, "err", err)
			return nil
		}
		if !info.IsDir() {
			return []string{src.Path}
		}
		entries, err := os.ReadDir(src.Path)
		if err != nil {
			c.warn.warn(now, "missing:"+src.Path, "Custom metrics source not readable", "path", src.Path, "err", err)
			return nil
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".prom") {
				files = append(files, filepath.Join(src.Path, entry.Name()))
			}
		}
	}
	slices.Sort(files)
	return files
}

// collection is the state of one Collect call.
type collection struct {
	warn     *warnLimiter
	previous map[string]counterReading // from the previous collection at this interval
	readings map[string]counterReading // from this collection
	now      time.Time
	values   map[string]float64
	meta     map[string]system.CustomMetricMeta
	owners   map[string]string // key -> file:line of the sample that claimed it
	problems fileProblems      // in the file being read
	bytes    int               // read from files so far
	samples  int               // keyed so far
	full     bool              // a budget is spent; read nothing more
}

func (r *collection) readFile(src *SourceConfig, path string) {
	// The stat keeps FIFOs and devices from being opened at all; readRegular
	// checks the open file again, in case one was swapped in since.
	info, err := os.Stat(path)
	if err == nil && !info.Mode().IsRegular() {
		err = errNotRegular
	}
	var data []byte
	if err == nil {
		data, info, err = readRegular(path, maxFileSize)
	}
	switch {
	case errors.Is(err, errNotRegular):
		return
	case errors.Is(err, errTooLarge):
		r.warn.warn(r.now, "size:"+path, "Custom metrics file too large, skipped", "file", path, "max_kib", maxFileSize>>10)
		return
	case err != nil:
		r.warn.warn(r.now, "read:"+path, "Custom metrics file not readable", "file", path, "err", err)
		return
	}
	// Parsing is the cost, so a file over the budget is read but not parsed.
	if r.bytes += len(data); r.bytes > maxBytes {
		r.warn.warn(r.now, "bytes", "Too much custom metrics data, skipping the rest", "max_kib", maxBytes>>10)
		r.full = true
		return
	}
	families, errs := Parse(data)
	if len(errs) > 0 {
		r.warn.warn(r.now, "parse:"+path, "Unparseable lines in custom metrics file skipped",
			"file", path, "lines", len(errs), "first", errs[0].Error())
	}
	r.problems = fileProblems{}
	for i := 0; i < len(families) && !r.full; i++ {
		r.family(src, path, &families[i], info.ModTime())
	}
	r.reportProblems(path)
}

// fileProblems counts what the file being read loses, by kind, so a file with
// thousands of bad lines logs one warning per kind instead of one per line.
type fileProblems struct {
	long, collisions, unsupported, future tally
}

// tally counts one kind of problem and keeps the log arguments of its first
// example.
type tally struct {
	count int
	first []any
}

func (t *tally) add(first ...any) {
	if t.count == 0 {
		t.first = first
	}
	t.count++
}

// reportProblems logs what the file just read lost, once an hour per file and
// kind, as parse errors are.
func (r *collection) reportProblems(path string) {
	p := &r.problems
	r.warnTally(p.long, "long", path, "series", "Custom metric keys too long, skipped")
	r.warnTally(p.collisions, "collision", path, "series", "Custom metric series share keys with earlier series, keeping the first")
	r.warnTally(p.unsupported, "unsupported", path, "metrics", "Custom metric types not supported, skipped")
	r.warnTally(p.future, "future", path, "series",
		"Custom metric timestamps ahead of the agent's clock, using the file's time; timestamps are in milliseconds")
}

func (r *collection) warnTally(t tally, kind, path, counted, msg string) {
	if t.count > 0 {
		r.warn.warn(r.now, kind+":"+path, msg, append([]any{"file", path, counted, t.count}, t.first...)...)
	}
}

var (
	errNotRegular = errors.New("not a regular file")
	errTooLarge   = errors.New("too large")
)

// readRegular reads a regular file of at most limit bytes. It checks the file
// it has open, not the path, so a FIFO renamed into place after a stat is
// neither waited on nor read, and the info it returns describes the bytes read.
func readRegular(path string, limit int64) ([]byte, os.FileInfo, error) {
	file, err := openNonBlocking(path)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, errNotRegular
	}
	if info.Size() > limit {
		return nil, nil, errTooLarge
	}
	// The file can grow between the stat and the read.
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err == nil && int64(len(data)) > limit {
		err = errTooLarge
	}
	return data, info, err
}

func (r *collection) family(src *SourceConfig, path string, f *Family, mtime time.Time) {
	if !src.matches(f.Name) || (f.Type == Counter && src.Counters == CountersSkip) {
		return
	}
	if f.Type == Unsupported {
		r.problems.unsupported.add("first", f.Name)
		return
	}
	unit := seriesUnit(f, src.Counters)
	help := truncateRunes(f.Help, maxDescriptionLength)
	chart := chartTitle(src.Chart.Title, src.DisplayNames, src.Prefix, f)
	for i := range f.Samples {
		// Counted before the key is built, since a sample dropped as too long
		// or as a collision costs the same work.
		if r.samples == maxSamples {
			r.warn.warn(r.now, "samples", "Too many custom metric samples, skipping the rest", "max", maxSamples)
			r.full = true
			return
		}
		r.samples++
		s := &f.Samples[i]
		key := seriesKey(src.Prefix, s)
		if len(key) > maxKeyLen {
			// Cut at a character boundary: the key may be in any script.
			r.problems.long.add("first", strings.ToValidUTF8(key[:maxKeyLen], "")+"…", "line", s.Line)
			continue
		}
		// The first sample with a key keeps it, by source, file, the order
		// metrics first appear in the file, then line, so the same one wins on
		// every collection and a chart never switches between two quantities.
		if owner, claimed := r.owners[key]; claimed {
			r.problems.collisions.add("first", key, "line", s.Line, "kept", owner)
			continue
		}
		r.owners[key] = fmt.Sprintf("%s:%d", path, s.Line)
		r.meta[key] = system.CustomMetricMeta{
			Unit:        unit,
			Help:        help,
			DisplayName: seriesDisplayName(src.DisplayNames, key, s),
			Chart:       chart,
			// empty unless the source declares the chart
			ChartDescription: src.Chart.Description,
		}
		if value, ok := r.value(src, f, key, s, mtime); ok {
			r.values[key] = value
		}
	}
}

// value returns the value to report for a sample, if any. A sample older than
// the source's max_age, or not finite, has none.
//
// A counter in rate or delta mode reports the change between its last two
// readings, timed by the readings rather than the collections, since a
// producer writes on its own schedule. (Beszel's network rates can divide by
// the time between collections because they read kernel counters at that
// moment; a file changes only when its producer writes.) Until the producer
// writes again, the same result is repeated, so a counter written every 20 s
// reads steadily in the 1 s live view instead of 0 then a spike. There is no
// result for a first reading, a reset (a decrease), a reading timed before the
// previous one, the first reading after a stale gap, or a change too large for
// a float64.
func (r *collection) value(src *SourceConfig, f *Family, key string, s *Sample, mtime time.Time) (float64, bool) {
	at := mtime
	if s.TimestampMs != 0 {
		// Trusted, a timestamp far ahead, such as one written in µs or ns,
		// would keep a stopped producer's value fresh for ever; the mtime ages.
		if ts := time.UnixMilli(s.TimestampMs); ts.Sub(r.now) <= maxClockSkew {
			at = ts
		} else {
			r.problems.future.add("first", key)
		}
	}
	if r.now.Sub(at) > src.MaxAge || math.IsNaN(s.Value) || math.IsInf(s.Value, 0) {
		return 0, false
	}
	if f.Type != Counter || src.Counters == CountersRaw {
		return s.Value, true
	}
	reading := counterReading{value: s.Value, at: at}
	if prev, ok := r.previous[key]; ok {
		switch {
		case at.Equal(prev.at):
			// Not written since the previous collection.
			reading = prev
		case at.After(prev.at) && s.Value >= prev.value:
			reading.result, reading.ok = s.Value-prev.value, true
			if src.Counters == CountersRate {
				reading.result /= at.Sub(prev.at).Seconds()
			}
			// Huge readings can overflow, and the hub cannot store ±Inf.
			reading.ok = !math.IsInf(reading.result, 0)
		}
	}
	r.readings[key] = reading
	return reading.result, reading.ok
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
