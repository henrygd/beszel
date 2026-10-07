//go:build testing

package custommetrics

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureWarnings records logWarn output for the rest of the test.
func captureWarnings(t *testing.T) *[]string {
	t.Helper()
	var logged []string
	original := logWarn
	logWarn = func(msg string, args ...any) {
		logged = append(logged, strings.TrimSpace(fmt.Sprintln(append([]any{msg}, args...)...)))
	}
	t.Cleanup(func() { logWarn = original })
	return &logged
}

// captureInfo records logInfo output for the rest of the test.
func captureInfo(t *testing.T) *[]string {
	t.Helper()
	var logged []string
	original := logInfo
	logInfo = func(msg string, args ...any) {
		logged = append(logged, strings.TrimSpace(fmt.Sprintln(append([]any{msg}, args...)...)))
	}
	t.Cleanup(func() { logInfo = original })
	return &logged
}

// rooted makes a slash path absolute on this OS: "/a" stays "/a" on Unix and
// becomes "C:/a" on Windows, where a path without a drive is relative and so
// resolves against the config file's directory.
func rooted(path string) string {
	if runtime.GOOS == "windows" {
		return "C:" + path
	}
	return path
}

// rootedConfig applies rooted to every source path in a config.
func rootedConfig(config string) string {
	return strings.ReplaceAll(config, "path: /", "path: "+rooted("/"))
}

func TestParseConfigDesignExample(t *testing.T) {
	cfg, warnings, err := parseConfig([]byte(rootedConfig(`
metrics:
  max_series: 64
  max_age: 2m
  counters: rate
  sources:
    - path: /run/beszel-power
      max_age: 60s
      display_names:
        pi_power_board_watts: Board power
        pi_power_wall_estimate_watts: Wall power (est)
    - path: /var/lib/myapp/metrics.d
      max_age: 10m
      include: [myapp_queue_*, myapp_jobs_*]
      chart:
        title: "  Queues  "
        description: Depth and throughput of the job queues
`)), "/etc/beszel")
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Equal(t, &Config{
		MaxSeries: 64,
		MaxAge:    2 * time.Minute,
		Counters:  CountersRate,
		Sources: []SourceConfig{
			{Path: rooted("/run/beszel-power"), MaxAge: time.Minute, Counters: CountersRate,
				DisplayNames: map[string]string{
					"pi_power_board_watts":         "Board power",
					"pi_power_wall_estimate_watts": "Wall power (est)",
				}},
			{Path: rooted("/var/lib/myapp/metrics.d"), MaxAge: 10 * time.Minute, Counters: CountersRate,
				Include: []string{"myapp_queue_*", "myapp_jobs_*"},
				Chart:   ChartConfig{Title: "Queues", Description: "Depth and throughput of the job queues"}},
		},
	}, cfg)
}

func TestParseConfigMinimal(t *testing.T) {
	cfg, warnings, err := parseConfig([]byte(rootedConfig("metrics:\n  sources:\n    - path: /var/lib/beszel/metrics.d\n")), "/x")
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Equal(t, &Config{
		MaxSeries: defaultMaxSeries,
		MaxAge:    defaultMaxAge,
		Counters:  CountersRate,
		Sources:   []SourceConfig{{Path: rooted("/var/lib/beszel/metrics.d"), MaxAge: defaultMaxAge, Counters: CountersRate}},
	}, cfg)
}

func TestParseConfigInheritance(t *testing.T) {
	t.Run("source inherits global max_age and counters", func(t *testing.T) {
		cfg, _, err := parseConfig([]byte(`
metrics:
  max_age: 5m
  counters: delta
  sources:
    - path: /a
    - path: /b
      max_age: 30s
      counters: raw
`), "/")
		require.NoError(t, err)
		assert.Equal(t, 5*time.Minute, cfg.Sources[0].MaxAge)
		assert.Equal(t, CountersDelta, cfg.Sources[0].Counters)
		assert.Equal(t, 30*time.Second, cfg.Sources[1].MaxAge)
		assert.Equal(t, CountersRaw, cfg.Sources[1].Counters)
	})
	t.Run("global absent inherits built-in defaults", func(t *testing.T) {
		cfg, _, err := parseConfig([]byte("metrics:\n  sources:\n    - path: /a\n"), "/")
		require.NoError(t, err)
		assert.Equal(t, 2*time.Minute, cfg.Sources[0].MaxAge)
		assert.Equal(t, CountersRate, cfg.Sources[0].Counters)
	})
	t.Run("per-source counters override the global", func(t *testing.T) {
		cfg, _, err := parseConfig([]byte(`
metrics:
  counters: skip
  sources:
    - path: /a
      counters: rate
    - path: /b
`), "/")
		require.NoError(t, err)
		assert.Equal(t, CountersRate, cfg.Sources[0].Counters)
		assert.Equal(t, CountersSkip, cfg.Sources[1].Counters)
	})
}

func TestParseConfigDurations(t *testing.T) {
	cfg, _, err := parseConfig([]byte("metrics:\n  max_age: 60s\n"), "/")
	require.NoError(t, err)
	assert.Equal(t, time.Minute, cfg.MaxAge)

	_, _, err = parseConfig([]byte("metrics:\n  max_age: 60\n"), "/")
	assert.ErrorContains(t, err, "cannot unmarshal !!int `60` into time.Duration")
	assert.ErrorContains(t, err, "durations need a unit, such as 60s or 2m")

	// Other type errors get no duration hint.
	_, _, err = parseConfig([]byte("metrics:\n  max_series: lots\n"), "/")
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "durations need a unit")

	// A type error is not excused by unknown keys alongside it.
	_, _, err = parseConfig([]byte("metrics:\n  max_age: 60\n  bogus: 1\n"), "/")
	assert.Error(t, err)
}

// On Windows a source path with a drive or a share is absolute and kept as
// written; a relative one resolves against the config file's directory.
func TestParseConfigWindowsPaths(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path rules")
	}
	cfg, _, err := parseConfig([]byte(`metrics:
  sources:
    - path: C:\metrics\*.prom
    - path: D:/metrics.d
    - path: \\server\share\metrics.d
    - path: metrics.d
    - path: ..\shared\power.prom
`), `C:\ProgramData\beszel-agent`)
	require.NoError(t, err)
	var paths []string
	for _, src := range cfg.Sources {
		paths = append(paths, src.Path)
	}
	assert.Equal(t, []string{
		`C:\metrics\*.prom`,
		`D:/metrics.d`,
		`\\server\share\metrics.d`,
		`C:\ProgramData\beszel-agent\metrics.d`,
		`C:\ProgramData\shared\power.prom`,
	}, paths)
}

func TestParseConfigUnknownKeys(t *testing.T) {
	cfg, warnings, err := parseConfig([]byte(rootedConfig(`metrics:
  max_ages: 5m
  max_series: 10
  sources:
    - path: /a
      url: http://localhost:9100/metrics
logging: debug
`)), "/")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"unknown key max_ages on line 2 ignored",
		"unknown key url on line 6 ignored",
		"unknown key logging on line 7 ignored",
	}, warnings)
	assert.Equal(t, 10, cfg.MaxSeries, "the rest of the config applies")
	assert.Equal(t, defaultMaxAge, cfg.MaxAge)
	assert.Equal(t, rooted("/a"), cfg.Sources[0].Path)
}

func TestParseConfigInert(t *testing.T) {
	for name, input := range map[string]string{
		"empty file":         "",
		"only a comment":     "# nothing here\n",
		"no metrics section": "other: 1\n",
		"null metrics":       "metrics:\n",
	} {
		t.Run(name, func(t *testing.T) {
			cfg, _, err := parseConfig([]byte(input), "/")
			require.NoError(t, err)
			assert.Nil(t, cfg)
		})
	}
}

func TestParseConfigMalformed(t *testing.T) {
	for _, input := range []string{"metrics: [\n", "metrics:\n  sources: 5\n", "\tmetrics:\n"} {
		_, _, err := parseConfig([]byte(input), "/")
		assert.Error(t, err, input)
	}
}

func TestParseConfigValidation(t *testing.T) {
	cfg, warnings, err := parseConfig([]byte(`
metrics:
  max_series: 300
  max_age: -1m
  counters: average
  sources:
    - path: ""
    - max_age: 1m
    - path: relative/metrics.d
      max_age: -5s
      counters: bogus
      include: ["ok_*", "bad_["]
      exclude: ["[", "skip_*"]
      prefix: "lab-1 "
      display_names:
        a: "  Padded  "
        b: "   "
        c: "`+strings.Repeat("é", 70)+`"
    - path: /all-bad-includes
      include: ["[", "a["]
`), "/etc/beszel")
	require.NoError(t, err)

	assert.Equal(t, maxMaxSeries, cfg.MaxSeries)
	assert.Equal(t, defaultMaxAge, cfg.MaxAge)
	assert.Equal(t, CountersRate, cfg.Counters)
	require.Len(t, cfg.Sources, 2, "sources without a path are skipped")

	src := cfg.Sources[0]
	assert.Equal(t, filepath.Join("/etc/beszel", "relative", "metrics.d"), src.Path, "relative to the config file")
	assert.Equal(t, defaultMaxAge, src.MaxAge)
	assert.Equal(t, CountersRate, src.Counters)
	assert.Equal(t, []string{"ok_*"}, src.Include)
	assert.Equal(t, []string{"skip_*"}, src.Exclude)
	assert.Equal(t, "lab_1", src.Prefix)
	assert.True(t, src.matches("ok_one"))
	assert.False(t, src.matches("other"))

	allBad := cfg.Sources[1]
	assert.Empty(t, allBad.Include)
	assert.False(t, allBad.matches("anything"), "a source whose include patterns were all invalid matches nothing")

	assert.Equal(t, map[string]string{"a": "Padded", "c": strings.Repeat("é", 64)}, src.DisplayNames)

	assert.Equal(t, []string{
		"max_series 300 is above 256, using 256",
		"max_age -1m0s is negative, using 2m0s",
		`counters "average" is not rate, delta, raw or skip, using rate`,
		"sources[0] has no path and is skipped",
		"sources[1] has no path and is skipped",
		"sources[2].max_age -5s is negative, using 2m0s",
		`sources[2].counters "bogus" is not rate, delta, raw or skip, using rate`,
		`sources[2].include pattern "bad_[" is invalid and dropped`,
		`sources[2].exclude pattern "[" is invalid and dropped`,
		`sources[3].include pattern "[" is invalid and dropped`,
		`sources[3].include pattern "a[" is invalid and dropped`,
		"sources[3] matches nothing until its include patterns are fixed",
	}, warnings)
}

func TestParseConfigMaxSeriesDefaults(t *testing.T) {
	for input, want := range map[string]int{"0": 64, "-3": 64, "1": 1, "256": 256, "257": 256} {
		cfg, _, err := parseConfig([]byte("metrics:\n  max_series: "+input+"\n"), "/")
		require.NoError(t, err)
		assert.Equal(t, want, cfg.MaxSeries, input)
	}
}

func TestSourceMatches(t *testing.T) {
	src := SourceConfig{Include: []string{"myapp_*"}, Exclude: []string{"myapp_debug_*"}}
	assert.True(t, src.matches("myapp_queue_depth"))
	assert.False(t, src.matches("myapp_debug_flag"), "exclude applies after include")
	assert.False(t, src.matches("other_queue_depth"))

	all := SourceConfig{Exclude: []string{"go_*"}}
	assert.True(t, all.matches("anything"))
	assert.False(t, all.matches("go_goroutines"))
}

func TestConfigPath(t *testing.T) {
	t.Run("data directory default", func(t *testing.T) {
		t.Setenv("CONFIG", "")
		t.Setenv("BESZEL_AGENT_CONFIG", "")
		path, explicit := configPath("/var/lib/beszel-agent")
		assert.Equal(t, filepath.Join("/var/lib/beszel-agent", "config.yml"), path)
		assert.False(t, explicit)
	})
	t.Run("CONFIG overrides", func(t *testing.T) {
		t.Setenv("CONFIG", "/etc/beszel/agent.yml")
		path, explicit := configPath("/var/lib/beszel-agent")
		assert.Equal(t, "/etc/beszel/agent.yml", path)
		assert.True(t, explicit)
	})
	t.Run("BESZEL_AGENT_CONFIG wins over CONFIG", func(t *testing.T) {
		t.Setenv("CONFIG", "/etc/beszel/agent.yml")
		t.Setenv("BESZEL_AGENT_CONFIG", "/opt/beszel.yml")
		path, explicit := configPath("")
		assert.Equal(t, "/opt/beszel.yml", path)
		assert.True(t, explicit)
	})
	t.Run("empty CONFIG is ignored", func(t *testing.T) {
		t.Setenv("CONFIG", "")
		path, explicit := configPath("/data")
		assert.Equal(t, filepath.Join("/data", "config.yml"), path)
		assert.False(t, explicit)
	})
	t.Run("neither", func(t *testing.T) {
		t.Setenv("CONFIG", "")
		path, _ := configPath("")
		assert.Empty(t, path)
	})
}

// writeConfig writes content and sets a distinct mtime, so a rewrite within
// the filesystem's timestamp resolution is still seen as a change.
func writeConfig(t *testing.T, path, content string, mtime time.Time) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	require.NoError(t, os.Chtimes(path, mtime, mtime))
}

func TestConfigFileLifecycle(t *testing.T) {
	warnings := captureWarnings(t)
	dir := t.TempDir()
	f := &configFile{path: filepath.Join(dir, "config.yml")}
	base := time.Now().Add(-time.Hour)

	// missing default file: inert and quiet
	assert.False(t, f.refresh())
	assert.Nil(t, f.cfg)
	assert.Empty(t, *warnings)

	// appears later
	writeConfig(t, f.path, rootedConfig("metrics:\n  sources:\n    - path: /a\n"), base)
	assert.True(t, f.refresh())
	require.NotNil(t, f.cfg)
	assert.Equal(t, rooted("/a"), f.cfg.Sources[0].Path)

	// unchanged: not re-read
	assert.False(t, f.refresh())

	// edited
	writeConfig(t, f.path, rootedConfig("metrics:\n  sources:\n    - path: /b\n"), base.Add(time.Second))
	assert.True(t, f.refresh())
	assert.Equal(t, rooted("/b"), f.cfg.Sources[0].Path)

	// malformed edit keeps the last good config, warning once
	writeConfig(t, f.path, "metrics:\n  max_age: 60\n", base.Add(2*time.Second))
	assert.False(t, f.refresh())
	assert.Equal(t, rooted("/b"), f.cfg.Sources[0].Path)
	assert.False(t, f.refresh())
	require.Len(t, *warnings, 1)
	assert.Contains(t, (*warnings)[0], "Invalid custom metrics config, keeping the previous one")

	// fixed again, with an unknown key that warns
	writeConfig(t, f.path, rootedConfig("metrics:\n  sources:\n    - path: /c\n  extra: 1\n"), base.Add(3*time.Second))
	assert.True(t, f.refresh())
	assert.Equal(t, rooted("/c"), f.cfg.Sources[0].Path)
	require.Len(t, *warnings, 2)
	assert.Contains(t, (*warnings)[1], "unknown key extra on line 4 ignored")

	// same mtime but a different size is still a change
	writeConfig(t, f.path, rootedConfig("metrics:\n  sources:\n    - path: /longer\n"), base.Add(3*time.Second))
	assert.True(t, f.refresh())
	assert.Equal(t, rooted("/longer"), f.cfg.Sources[0].Path)

	// metrics section removed: inert
	writeConfig(t, f.path, "other: 1\n", base.Add(4*time.Second))
	assert.True(t, f.refresh())
	assert.Nil(t, f.cfg)

	// deleted: inert, and quiet for the default path
	writeConfig(t, f.path, rootedConfig("metrics:\n  sources:\n    - path: /d\n"), base.Add(5*time.Second))
	assert.True(t, f.refresh())
	require.NoError(t, os.Remove(f.path))
	assert.True(t, f.refresh())
	assert.Nil(t, f.cfg)
	assert.False(t, f.refresh())
	require.Len(t, *warnings, 3, "removing the default config is quiet")
	assert.Contains(t, (*warnings)[2], "unknown key other on line 1 ignored")

	// recreated with the old mtime and size is still read
	writeConfig(t, f.path, rootedConfig("metrics:\n  sources:\n    - path: /d\n"), base.Add(5*time.Second))
	assert.True(t, f.refresh())
	assert.Equal(t, rooted("/d"), f.cfg.Sources[0].Path)
}

func TestConfigFileExplicitMissingWarnsOnce(t *testing.T) {
	warnings := captureWarnings(t)
	f := &configFile{path: filepath.Join(t.TempDir(), "agent.yml"), explicit: true}
	for range 3 {
		assert.False(t, f.refresh())
	}
	require.Len(t, *warnings, 1)
	assert.Contains(t, (*warnings)[0], "Custom metrics config not loaded")

	// found, then missing again: warns again
	writeConfig(t, f.path, rootedConfig("metrics:\n  sources:\n    - path: /a\n"), time.Now())
	assert.True(t, f.refresh())
	require.NoError(t, os.Remove(f.path))
	assert.True(t, f.refresh())
	assert.Len(t, *warnings, 2)
}

// An oversized file is treated like a malformed one: the last good config
// stays, it warns once, and it is not read again until it changes.
func TestConfigFileTooLarge(t *testing.T) {
	warnings := captureWarnings(t)
	f := &configFile{path: filepath.Join(t.TempDir(), "config.yml")}
	base := time.Now().Add(-time.Hour)
	writeConfig(t, f.path, rootedConfig("metrics:\n  sources:\n    - path: /a\n"), base)
	require.True(t, f.refresh())

	big := "metrics:\n  sources:\n    - path: /b\n#" + strings.Repeat("x", maxConfigSize) + "\n"
	writeConfig(t, f.path, big, base.Add(time.Second))
	assert.False(t, f.refresh())
	assert.Equal(t, rooted("/a"), f.cfg.Sources[0].Path, "the last good config stays")
	require.Len(t, *warnings, 1)
	assert.Contains(t, (*warnings)[0], "Custom metrics config too large, keeping the previous one")

	// Not read again: make it unreadable, which a read would report.
	require.NoError(t, os.Chmod(f.path, 0))
	t.Cleanup(func() { _ = os.Chmod(f.path, 0o644) })
	for range 3 {
		assert.False(t, f.refresh())
	}
	assert.Len(t, *warnings, 1)
	require.NoError(t, os.Chmod(f.path, 0o644))

	// Shrunk again: loads.
	writeConfig(t, f.path, rootedConfig("metrics:\n  sources:\n    - path: /c\n"), base.Add(2*time.Second))
	assert.True(t, f.refresh())
	assert.Equal(t, rooted("/c"), f.cfg.Sources[0].Path)
}

func TestConfigFileTooLargeFromStart(t *testing.T) {
	warnings := captureWarnings(t)
	f := &configFile{path: filepath.Join(t.TempDir(), "config.yml")}
	writeConfig(t, f.path, "#"+strings.Repeat("x", maxConfigSize)+"\n", time.Now())
	assert.False(t, f.refresh())
	assert.Nil(t, f.cfg, "no previous config to keep")
	require.Len(t, *warnings, 1)
	assert.Contains(t, (*warnings)[0], "Custom metrics config too large, custom metrics are off until it is fixed")
}

// At start-up there is no previous config to keep, and the warning says so.
func TestConfigFileInvalidFromStart(t *testing.T) {
	warnings := captureWarnings(t)
	f := &configFile{path: filepath.Join(t.TempDir(), "config.yml")}
	writeConfig(t, f.path, "metrics:\n  max_age: 60\n", time.Now())
	assert.False(t, f.refresh())
	assert.Nil(t, f.cfg)
	require.Len(t, *warnings, 1)
	assert.Contains(t, (*warnings)[0], "Invalid custom metrics config, custom metrics are off until it is fixed")
	assert.Contains(t, (*warnings)[0], "durations need a unit")
}

// Each config that loads is logged once; one without a metrics section is not.
func TestConfigFileLogsLoad(t *testing.T) {
	logged := captureInfo(t)
	f := &configFile{path: filepath.Join(t.TempDir(), "config.yml")}
	base := time.Now().Add(-time.Hour)
	writeConfig(t, f.path, rootedConfig("metrics:\n  sources:\n    - path: /a\n    - path: /b\n"), base)
	require.True(t, f.refresh())
	assert.False(t, f.refresh())
	require.Len(t, *logged, 1)
	assert.Contains(t, (*logged)[0], "Custom metrics config loaded")
	assert.Contains(t, (*logged)[0], "sources 2")

	writeConfig(t, f.path, rootedConfig("metrics:\n  sources:\n    - path: /c\n"), base.Add(time.Second))
	require.True(t, f.refresh())
	require.Len(t, *logged, 2)
	assert.Contains(t, (*logged)[1], "sources 1")

	writeConfig(t, f.path, "other: 1\n", base.Add(2*time.Second))
	require.True(t, f.refresh())
	assert.Len(t, *logged, 2)
}

func TestConfigFileRelativeSourcePath(t *testing.T) {
	dir := t.TempDir()
	f := &configFile{path: filepath.Join(dir, "config.yml")}
	writeConfig(t, f.path, "metrics:\n  sources:\n    - path: metrics.d\n", time.Now())
	require.True(t, f.refresh())
	assert.Equal(t, filepath.Join(dir, "metrics.d"), f.cfg.Sources[0].Path)
}

func TestParseConfigCharts(t *testing.T) {
	cfg, warnings, err := parseConfig([]byte(`
metrics:
  sources:
    - path: /pi
      chart:
        title: Power
    - path: /plug
      chart:
        title: Power
        description: Measured at the plug
    - path: /ups
      chart:
        title: Power
        description: Reported by the UPS
    - path: /orphan
      chart:
        description: No title here
    - path: /long
      chart:
        title: "`+strings.Repeat("t", 70)+`"
        description: "`+strings.Repeat("d", 210)+`"
`), "/")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"sources[3].chart.description is ignored without a title",
		`chart "Power" has different descriptions in sources[1] and sources[2], using the first`,
	}, warnings)

	described := ChartConfig{Title: "Power", Description: "Measured at the plug"}
	assert.Equal(t, described, cfg.Sources[0].Chart, "the first description set applies to every source with the title")
	assert.Equal(t, described, cfg.Sources[1].Chart)
	assert.Equal(t, described, cfg.Sources[2].Chart)
	assert.Equal(t, ChartConfig{}, cfg.Sources[3].Chart)
	assert.Equal(t, ChartConfig{Title: strings.Repeat("t", 64), Description: strings.Repeat("d", 200)}, cfg.Sources[4].Chart)
}

func TestParseConfigChartMustBeAnObject(t *testing.T) {
	cfg, warnings, err := parseConfig([]byte("metrics:\n  sources:\n    - path: /a\n      chart: Power consumption\n    - path: /b\n"), "/")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.Len(t, cfg.Sources, 1)
	assert.Equal(t, "/b", cfg.Sources[0].Path)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "sources[0]")

	_, warnings, err = parseConfig([]byte("metrics:\n  sources:\n    - path: /a\n      chart:\n        name: Power\n"), "/")
	require.NoError(t, err)
	assert.Equal(t, []string{"unknown key name on line 5 ignored"}, warnings)
}

func TestParseConfigBadSourceSkipped(t *testing.T) {
	cfg, warnings, err := parseConfig([]byte(`metrics:
  sources:
    - path: /good
    - path: /bad-age
      max_age: 60
    - path: /bad-include
      include: myapp_*
`), "/")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.Len(t, cfg.Sources, 1)
	assert.Equal(t, "/good", cfg.Sources[0].Path)
	require.Len(t, warnings, 2)
	assert.Contains(t, warnings[0], "sources[1]")
	assert.Contains(t, warnings[1], "sources[2]")
	for _, w := range warnings {
		assert.NotContains(t, w, "cannot unmarshal")
	}
	_, _, err = parseConfig([]byte("metrics:\n  max_age: 60\n  sources:\n    - path: /a\n"), "/")
	assert.Error(t, err)
}

// display_names belongs to a source; one at the top level is an unknown key.
func TestParseConfigDisplayNamesAtTopLevelWarn(t *testing.T) {
	cfg, warnings, err := parseConfig([]byte("metrics:\n  sources:\n    - path: /a\n  display_names:\n    a: A\n"), "/")
	require.NoError(t, err)
	assert.Equal(t, []string{"unknown key display_names on line 4 ignored"}, warnings)
	assert.Nil(t, cfg.Sources[0].DisplayNames)
}
