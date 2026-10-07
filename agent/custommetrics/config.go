package custommetrics

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/henrygd/beszel/agent/utils"
	"gopkg.in/yaml.v3"
)

const (
	defaultMaxSeries     = 64
	maxMaxSeries         = 256 // bounds the systems.info payload pushed to every client
	defaultMaxAge        = 2 * time.Minute
	maxConfigSize        = 256 << 10
	maxDisplayNameLength = 64  // runes, for display names and chart titles
	maxDescriptionLength = 200 // runes, for chart descriptions and # HELP text
	maxUnitLength        = 256 // runes, for # UNIT text
)

// CounterMode sets how counters are reported. Rates and deltas are timed by
// the counter's own readings, not by collections (see collection.value).
type CounterMode string

const (
	CountersRate  CounterMode = "rate"  // per-second change between the counter's last two readings; the default
	CountersDelta CounterMode = "delta" // change between the counter's last two readings, for counts better read per write
	CountersRaw   CounterMode = "raw"   // the cumulative value as is, such as a total since boot
	CountersSkip  CounterMode = "skip"  // not reported at all: no key, no metadata, no max_series slot
)

// fileConfig is the whole config.yml. Only the metrics section is read; any
// other top-level key is reported as unknown and ignored.
type fileConfig struct {
	Metrics *Config `yaml:"metrics"`
}

// fileConfigRaw decodes config.yml with sources left as nodes, so one bad
// source can be skipped while the rest apply. Only top-level fields are
// named here; source fields are never listed.
type fileConfigRaw struct {
	Metrics *struct {
		MaxSeries int           `yaml:"max_series"`
		MaxAge    time.Duration `yaml:"max_age"`
		Counters  CounterMode   `yaml:"counters"`
		Sources   []yaml.Node   `yaml:"sources"`
	} `yaml:"metrics"`
}

// Config is the metrics: section. Unknown keys only warn, so a config written
// for a later version still loads on this one. To keep that true, a later
// version may add keys but must not change an existing key's type: an older
// agent cannot decode it, so it skips the source, or, for a top-level key,
// keeps its last good config.
type Config struct {
	MaxSeries int            `yaml:"max_series"` // default 64, clamped to 256
	MaxAge    time.Duration  `yaml:"max_age"`    // default 2m
	Counters  CounterMode    `yaml:"counters"`   // default rate
	Sources   []SourceConfig `yaml:"sources"`
}

// SourceConfig is one entry in sources. After loading, MaxAge and Counters
// hold the effective values and Path is absolute.
type SourceConfig struct {
	Path     string        `yaml:"path"`     // required: directory, file or glob
	MaxAge   time.Duration `yaml:"max_age"`  // 0 = inherit
	Counters CounterMode   `yaml:"counters"` // "" = inherit
	Include  []string      `yaml:"include"`  // path.Match globs on metric names
	Exclude  []string      `yaml:"exclude"`
	// Prefix goes in front of every key from this source, to keep apart
	// producers that write the same metric names. Changing it starts new
	// series under new keys; display_names entries, written without it, still
	// apply.
	Prefix string      `yaml:"prefix"`
	Chart  ChartConfig `yaml:"chart"` // one chart for every series; unset = one chart per metric
	// DisplayNames maps a series, named as in the file (metric name and label
	// values, without the prefix) or by its key, to its line name in the legend
	// and tooltip. A metric with a single series and a display name also takes
	// it as its chart's title (see chartTitle).
	DisplayNames map[string]string `yaml:"display_names"`

	includeNone bool // every include pattern was invalid, so nothing matches
}

// ChartConfig puts every series from a source on one chart. Sources with the
// same title share the chart.
type ChartConfig struct {
	Title       string `yaml:"title"`       // the chart's heading and identity
	Description string `yaml:"description"` // text under the title
}

// configPath returns where the config file lives, and whether CONFIG named it.
// It returns "" when neither CONFIG nor a data directory is available.
func configPath(dataDir string) (path string, explicit bool) {
	if p, _ := utils.GetEnv("CONFIG"); p != "" {
		return p, true
	}
	if dataDir == "" {
		return "", false
	}
	return filepath.Join(dataDir, "config.yml"), false
}

// configFile tracks the config file and the last good config read from it.
type configFile struct {
	path     string
	explicit bool      // named by CONFIG, so a missing file is worth a warning
	modTime  time.Time // of the last version read
	size     int64
	cfg      *Config // last good config; nil = inert
	warned   bool    // missing or unreadable file already reported
}

// refresh re-reads the config when its mtime or size changes. It reports
// whether the active config changed. A malformed or oversized edit keeps the
// last good config; a missing or unreadable file, or a path that is not a
// regular file, makes the feature inert.
func (f *configFile) refresh() (changed bool) {
	info, err := os.Stat(f.path)
	if err == nil && info.ModTime().Equal(f.modTime) && info.Size() == f.size {
		return false
	}
	var data []byte
	if err == nil {
		data, _, err = readRegular(f.path, maxConfigSize)
	}
	if errors.Is(err, errTooLarge) {
		// Like a malformed edit: remember this version so it is not read again.
		f.warned = false
		f.modTime, f.size = info.ModTime(), info.Size()
		logWarn("Custom metrics config too large, "+f.fallback(), "path", f.path, "max_kib", maxConfigSize>>10)
		return false
	}
	if err != nil {
		// Forget the version so the file is retried once it is fixed.
		f.modTime, f.size = time.Time{}, 0
		if !f.warned && (f.explicit || !errors.Is(err, fs.ErrNotExist)) {
			logWarn("Custom metrics config not loaded", "path", f.path, "err", err)
			f.warned = true
		}
		changed = f.cfg != nil
		f.cfg = nil
		return changed
	}
	f.warned = false
	f.modTime, f.size = info.ModTime(), info.Size()

	cfg, warnings, err := parseConfig(data, filepath.Dir(f.path))
	if err != nil {
		logWarn("Invalid custom metrics config, "+f.fallback(), "path", f.path, "err", err)
		return false
	}
	for _, w := range warnings {
		logWarn("Custom metrics config", "path", f.path, "warning", w)
	}
	f.cfg = cfg
	if cfg != nil {
		logInfo("Custom metrics config loaded", "path", f.path, "sources", len(cfg.Sources))
	}
	return true
}

// fallback says what applies while the file cannot be used: the last good
// config, or, when none has loaded since the agent started, nothing.
func (f *configFile) fallback() string {
	if f.cfg == nil {
		return "custom metrics are off until it is fixed"
	}
	return "keeping the previous one"
}

var unknownFieldPattern = regexp.MustCompile(`^line (\d+): field (\S+) not found in type \S+$`)

// parseConfig decodes config.yml and applies defaults. Relative source paths
// resolve against dir. It returns a nil config when there is no metrics
// section, and warnings for unknown keys and invalid values, which are
// ignored or replaced by defaults. Any other problem is an error.
func parseConfig(data []byte, dir string) (*Config, []string, error) {
	var file fileConfig
	var warnings []string
	if err := decodeYAML(data, &file, true); err != nil {
		var typeErr *yaml.TypeError
		if !errors.As(err, &typeErr) {
			return nil, nil, err
		}
		// Unknown keys warn; real type errors quarantine one bad source
		// below instead of rejecting the file.
		hasReal := false
		for _, msg := range typeErr.Errors {
			m := unknownFieldPattern.FindStringSubmatch(msg)
			if m == nil {
				hasReal = true
				continue
			}
			warnings = append(warnings, fmt.Sprintf("unknown key %s on line %s ignored", m[2], m[1]))
		}
		if hasReal {
			// Top-level shape errors still reject the file; only bad
			// source blocks are skipped, warning sources[i] and path.
			var raw fileConfigRaw
			if err := decodeYAML(data, &raw, false); err != nil {
				if strings.Contains(err.Error(), "into time.Duration") {
					err = fmt.Errorf("%w; durations need a unit, such as 60s or 2m", err)
				}
				return nil, nil, err
			}
			if raw.Metrics == nil {
				return nil, warnings, nil
			}
			cfg := &Config{MaxSeries: raw.Metrics.MaxSeries, MaxAge: raw.Metrics.MaxAge, Counters: raw.Metrics.Counters}
			for i := range raw.Metrics.Sources {
				var src SourceConfig
				if err := raw.Metrics.Sources[i].Decode(&src); err != nil {
					// Name the file block and path; the message stays
					// free of Go type names.
					var pp struct {
						Path string `yaml:"path"`
					}
					_ = raw.Metrics.Sources[i].Decode(&pp)
					if pp.Path != "" {
						warnings = append(warnings, fmt.Sprintf("sources[%d] (path %q) skipped: invalid value", i, pp.Path))
					} else {
						warnings = append(warnings, fmt.Sprintf("sources[%d] skipped: invalid value", i))
					}
					continue
				}
				cfg.Sources = append(cfg.Sources, src)
			}
			warnings = append(warnings, cfg.normalize(dir)...)
			return cfg, warnings, nil
		}
		file = fileConfig{}
		if err := decodeYAML(data, &file, false); err != nil {
			return nil, nil, err
		}
	}
	if file.Metrics == nil {
		return nil, warnings, nil
	}
	warnings = append(warnings, file.Metrics.normalize(dir)...)
	return file.Metrics, warnings, nil
}

func decodeYAML(data []byte, v any, knownFields bool) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(knownFields)
	if err := dec.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// normalize applies defaults and validation, returning a warning for each
// value it had to replace or drop.
func (c *Config) normalize(dir string) (warnings []string) {
	warnf := func(format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf(format, args...))
	}

	switch {
	case c.MaxSeries < 0:
		warnf("max_series %d is negative, using %d", c.MaxSeries, defaultMaxSeries)
		c.MaxSeries = defaultMaxSeries
	case c.MaxSeries == 0:
		c.MaxSeries = defaultMaxSeries
	case c.MaxSeries > maxMaxSeries:
		warnf("max_series %d is above %d, using %d", c.MaxSeries, maxMaxSeries, maxMaxSeries)
		c.MaxSeries = maxMaxSeries
	}
	if c.MaxAge < 0 {
		warnf("max_age %s is negative, using %s", c.MaxAge, defaultMaxAge)
		c.MaxAge = 0
	}
	if c.MaxAge == 0 {
		c.MaxAge = defaultMaxAge
	}
	c.Counters = checkCounters(c.Counters, CountersRate, "counters", warnf)

	sources := make([]SourceConfig, 0, len(c.Sources))
	for i, s := range c.Sources {
		field := fmt.Sprintf("sources[%d]", i)
		if strings.TrimSpace(s.Path) == "" {
			warnf("%s has no path and is skipped", field)
			continue
		}
		if !filepath.IsAbs(s.Path) {
			s.Path = filepath.Join(dir, s.Path)
		}
		if s.MaxAge < 0 {
			warnf("%s.max_age %s is negative, using %s", field, s.MaxAge, c.MaxAge)
			s.MaxAge = 0
		}
		if s.MaxAge == 0 {
			s.MaxAge = c.MaxAge
		}
		s.Counters = checkCounters(s.Counters, c.Counters, field+".counters", warnf)
		hadInclude := len(s.Include) > 0
		s.Include = checkPatterns(s.Include, field+".include", warnf)
		s.includeNone = hadInclude && len(s.Include) == 0
		if s.includeNone {
			warnf("%s matches nothing until its include patterns are fixed", field)
		}
		s.Exclude = checkPatterns(s.Exclude, field+".exclude", warnf)
		s.Prefix = sanitize(s.Prefix)
		s.DisplayNames = cleanDisplayNames(s.DisplayNames)
		s.Chart.Title = cleanName(s.Chart.Title, maxDisplayNameLength)
		s.Chart.Description = cleanName(s.Chart.Description, maxDescriptionLength)
		if s.Chart.Title == "" && s.Chart.Description != "" {
			warnf("%s.chart.description is ignored without a title", field)
			s.Chart.Description = ""
		}
		sources = append(sources, s)
	}
	c.Sources = sources
	warnings = append(warnings, resolveChartDescriptions(c.Sources)...)
	return warnings
}

// cleanDisplayNames trims and caps each name, dropping empty ones.
func cleanDisplayNames(names map[string]string) map[string]string {
	for key, name := range names {
		if name = cleanName(name, maxDisplayNameLength); name == "" {
			delete(names, key)
		} else {
			names[key] = name
		}
	}
	return names
}

// cleanName trims a display name, chart title or description and caps it at
// maxRunes.
func cleanName(name string, maxRunes int) string {
	name = strings.TrimSpace(strings.ToValidUTF8(name, "\uFFFD"))
	if utf8.RuneCountInString(name) > maxRunes {
		name = strings.TrimSpace(string([]rune(name)[:maxRunes]))
	}
	return name
}

// resolveChartDescriptions gives every source sharing a chart title the same
// description: the first one set, in source order. A different description
// set later is ignored with a warning.
func resolveChartDescriptions(sources []SourceConfig) (warnings []string) {
	type first struct {
		description string
		source      int
	}
	chosen := make(map[string]first)
	for i := range sources {
		chart := &sources[i].Chart
		if chart.Title == "" || chart.Description == "" {
			continue
		}
		if f, ok := chosen[chart.Title]; !ok {
			chosen[chart.Title] = first{chart.Description, i}
		} else if f.description != chart.Description {
			warnings = append(warnings, fmt.Sprintf("chart %q has different descriptions in sources[%d] and sources[%d], using the first",
				chart.Title, f.source, i))
		}
	}
	for i := range sources {
		chart := &sources[i].Chart
		if f, ok := chosen[chart.Title]; ok {
			chart.Description = f.description
		}
	}
	return warnings
}

func checkCounters(mode, fallback CounterMode, field string, warnf func(string, ...any)) CounterMode {
	switch mode {
	case "":
		return fallback
	case CountersRate, CountersDelta, CountersRaw, CountersSkip:
		return mode
	}
	warnf("%s %q is not rate, delta, raw or skip, using %s", field, mode, fallback)
	return fallback
}

// checkPatterns drops malformed globs.
func checkPatterns(patterns []string, field string, warnf func(string, ...any)) []string {
	valid := patterns[:0]
	for _, p := range patterns {
		if _, err := path.Match(p, ""); err != nil {
			warnf("%s pattern %q is invalid and dropped", field, p)
			continue
		}
		valid = append(valid, p)
	}
	return valid
}

// matches applies include, then exclude, to a metric name.
func (s *SourceConfig) matches(name string) bool {
	if s.includeNone {
		return false
	}
	if len(s.Include) > 0 && !matchAny(s.Include, name) {
		return false
	}
	return !matchAny(s.Exclude, name)
}

func matchAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}
