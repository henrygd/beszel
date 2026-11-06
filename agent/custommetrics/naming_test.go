//go:build testing

package custommetrics

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestSanitize(t *testing.T) {
	tests := map[string]string{
		"sda":         "sda",
		"sda_1":       "sda_1",
		"sda 1":       "sda_1",
		"a--b..c":     "a_b_c",
		"/dev/sda1":   "dev_sda1",
		"/":           "",
		"_x_":         "x",
		"192.168.1.5": "192_168_1_5",
		"":            "",
		// Letters, digits and combining marks in any script are kept, so values
		// written in other languages stay distinct and readable.
		"café":              "café",
		"Ünïcode":           "Ünïcode",
		"Кухня":             "Кухня",
		"キッチン":              "キッチン",
		"living room/Кухня": "living_room_Кухня",
		"हिन्दी":            "हिन्दी", // vowel signs are combining marks
		"٣":                 "٣",      // an Arabic-Indic digit
		"温度°C":              "温度_C",
		"🔥hot":              "hot",
		"a\xffb":            "a_b", // invalid UTF-8 never reaches a key
	}
	for in, want := range tests {
		got := sanitize(in)
		assert.Equal(t, want, got, "sanitize(%q)", in)
		assert.True(t, utf8.ValidString(got), "sanitize(%q) is valid UTF-8", in)
	}
}

func TestSeriesKey(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		sample Sample
		want   string
	}{
		{"bare name", "", Sample{Name: "pi_power_board_watts"}, "pi_power_board_watts"},
		{
			"label values sorted by label name",
			"", Sample{Name: "disk_temp_celsius", Labels: []Label{{"host", "x"}, {"device", "sda"}}},
			"disk_temp_celsius_sda_x",
		},
		{
			"values sanitised and edge underscores trimmed",
			"", Sample{Name: "node_filesystem_avail_bytes", Labels: []Label{{"mountpoint", "/"}, {"device", "/dev/sda1"}}},
			"node_filesystem_avail_bytes_dev_sda1",
		},
		{"empty values skipped", "", Sample{Name: "m", Labels: []Label{{"a", ""}, {"b", "x"}}}, "m_x"},
		{"counter keeps _total", "", Sample{Name: "http_requests_total", Labels: []Label{{"code", "200"}}}, "http_requests_total_200"},
		{"prefix joined with _", "lab", Sample{Name: "temp_celsius"}, "lab_temp_celsius"},
		{"prefix and labels", "lab", Sample{Name: "temp_celsius", Labels: []Label{{"zone", "a"}}}, "lab_temp_celsius_a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, seriesKey(tt.prefix, &tt.sample))
		})
	}
}

// The key is the series identity in stored history, so it must not depend on
// the order labels are written in or on map iteration.
func TestSeriesKeyDeterministic(t *testing.T) {
	orders := [][]Label{
		{{"a", "1"}, {"b", "2"}, {"c", "3"}},
		{{"c", "3"}, {"a", "1"}, {"b", "2"}},
		{{"b", "2"}, {"c", "3"}, {"a", "1"}},
	}
	for range 100 {
		for _, labels := range orders {
			original := append([]Label(nil), labels...)
			assert.Equal(t, "m_1_2_3", seriesKey("", &Sample{Name: "m", Labels: labels}))
			assert.Equal(t, original, labels, "labels must not be reordered in place")
		}
	}
}

func TestPrefixSpellingsGiveTheSameKey(t *testing.T) {
	for _, prefix := range []string{"lab", "lab_", "lab-", "_lab"} {
		assert.Equal(t, "lab_temp_celsius", seriesKey(sanitize(prefix), &Sample{Name: "temp_celsius"}), prefix)
	}
}

func TestSeriesUnit(t *testing.T) {
	tests := []struct {
		name     string
		family   Family
		counters CounterMode
		want     string
	}{
		{"UNIT line wins", Family{Name: "pi_power_board_watts", Unit: "W"}, CountersRate, "W"},
		{"UNIT on an unconventional name", Family{Name: "pi_power", Unit: "watts"}, CountersRate, "watts"},
		{"suffix inferred", Family{Name: "pi_power_board_watts"}, CountersRate, "watts"},
		{"no unit", Family{Name: "queue_depth"}, CountersRate, ""},
		{"suffix must follow an underscore", Family{Name: "kilowatts"}, CountersRate, ""},
		{"gauge ending _total looks past it", Family{Name: "energy_joules_total", Type: Gauge}, CountersRate, "joules"},
		{"counter rate appends /s", Family{Name: "node_network_receive_bytes_total", Type: Counter}, CountersRate, "bytes/s"},
		{"OpenMetrics counter family", Family{Name: "node_network_receive_bytes", Type: Counter}, CountersRate, "bytes/s"},
		{"unitless counter rate", Family{Name: "http_requests_total", Type: Counter}, CountersRate, "/s"},
		{"counter delta keeps the unit", Family{Name: "energy_joules_total", Type: Counter}, CountersDelta, "joules"},
		{"counter raw keeps the unit", Family{Name: "energy_joules_total", Type: Counter}, CountersRaw, "joules"},
		{"long UNIT capped", Family{Name: "m", Unit: strings.Repeat("é", maxUnitLength+50)}, CountersRaw, strings.Repeat("é", maxUnitLength)},
		{"capped UNIT still gets /s", Family{Name: "m", Type: Counter, Unit: strings.Repeat("é", maxUnitLength+50)}, CountersRate, strings.Repeat("é", maxUnitLength) + "/s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, seriesUnit(&tt.family, tt.counters))
		})
	}
	for _, unit := range unitSuffixes {
		assert.Equal(t, unit, baseUnit(&Family{Name: "x_" + unit}), unit)
		assert.Equal(t, unit, baseUnit(&Family{Name: "x_" + unit + "_total"}), unit)
	}
}

func TestSeriesDisplayName(t *testing.T) {
	get := Sample{Name: "http_requests_total", Labels: []Label{{"method", "get"}}}
	tests := []struct {
		name   string
		prefix string
		names  map[string]string
		sample Sample
		want   string
	}{
		{"without an entry, the key", "", nil, Sample{Name: "pi_power_wall_estimate_watts"}, "pi_power_wall_estimate_watts"},
		{"label values after the name", "", nil, Sample{Name: "disk_temp_celsius", Labels: []Label{{"device", "sda"}}}, "disk_temp_celsius_sda"},
		{"a counter keeps _total, so it never shares a name with a gauge", "", nil, Sample{Name: "jobs_total"}, "jobs_total"},
		{"prefix included", "lab", nil, Sample{Name: "temp_celsius"}, "lab_temp_celsius"},
		{"entry as the file names it", "lab", map[string]string{"http_requests_total_get": "GET requests"}, get, "GET requests"},
		{"entry by the key, the name shown without one", "lab", map[string]string{"lab_http_requests_total_get": "GET requests"}, get, "GET requests"},
		{
			"the file's name wins over the key",
			"lab", map[string]string{"http_requests_total_get": "By file", "lab_http_requests_total_get": "By key"}, get,
			"By file",
		},
		{"another series' entry does not apply", "lab", map[string]string{"temp_celsius": "Temperature"}, get, "lab_http_requests_total_get"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, seriesDisplayName(tt.names, seriesKey(tt.prefix, &tt.sample), &tt.sample))
		})
	}
}

// A display name is presentation only and never changes the key.
func TestDisplayNameDoesNotChangeKey(t *testing.T) {
	sample := Sample{Name: "pi_power_board_watts"}
	key := seriesKey("", &sample)
	for _, names := range []map[string]string{nil, {key: "Board power"}, {key: "Something else"}} {
		assert.Equal(t, "pi_power_board_watts", seriesKey("", &sample))
		label := seriesDisplayName(names, key, &sample)
		if names == nil {
			assert.Equal(t, "pi_power_board_watts", label)
		} else {
			assert.Equal(t, names[key], label)
		}
	}
}

func TestChartTitle(t *testing.T) {
	names := map[string]string{"pi_power_board_watts": "Board power", "disk_c_sda": "SDA"}
	tests := []struct {
		name   string
		chart  string
		family Family
		want   string
	}{
		{"source chart wins", "Power consumption", Family{Name: "pi_power_board_watts", Samples: []Sample{{Name: "pi_power_board_watts"}}}, "Power consumption"},
		{"single series display name", "", Family{Name: "pi_power_board_watts", Samples: []Sample{{Name: "pi_power_board_watts"}}}, "Board power"},
		{"single labelled series display name", "", Family{Name: "disk_c", Samples: []Sample{{Name: "disk_c", Labels: []Label{{"d", "sda"}}}}}, "SDA"},
		{"humanised name, unit kept", "", Family{Name: "pi_power_wall_estimate_watts", Samples: []Sample{{Name: "pi_power_wall_estimate_watts"}}}, "Pi power wall estimate watts"},
		{"only _total removed", "", Family{Name: "node_network_receive_bytes_total", Type: Counter}, "Node network receive bytes"},
		{
			"several series ignore display names",
			"", Family{Name: "disk_c", Samples: []Sample{
				{Name: "disk_c", Labels: []Label{{"d", "sda"}}}, {Name: "disk_c", Labels: []Label{{"d", "sdb"}}},
			}},
			"Disk c",
		},
		{"underscore runs collapse", "", Family{Name: "__odd__name_"}, "Odd name"},
		{"colons kept", "", Family{Name: "job:requests:rate5m"}, "Job:requests:rate5m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, chartTitle(tt.chart, names, "", &tt.family))
		})
	}

	// An entry written as the key, prefix included, titles the chart too.
	queue := Family{Name: "queue_depth", Samples: []Sample{{Name: "queue_depth"}}}
	assert.Equal(t, "Queue", chartTitle("", map[string]string{"lab_queue_depth": "Queue"}, "lab", &queue))
}

// Metrics that differ only by unit keep distinct line names and chart titles,
// so they neither share a name in a legend nor merge into one chart.
func TestNamesKeepTheUnit(t *testing.T) {
	volts := Family{Name: "pi_core_volts", Samples: []Sample{{Name: "pi_core_volts"}}}
	amperes := Family{Name: "pi_core_amperes", Samples: []Sample{{Name: "pi_core_amperes"}}}
	assert.Equal(t, "pi_core_volts", seriesDisplayName(nil, "pi_core_volts", &volts.Samples[0]))
	assert.Equal(t, "pi_core_amperes", seriesDisplayName(nil, "pi_core_amperes", &amperes.Samples[0]))
	assert.Equal(t, "Pi core volts", chartTitle("", nil, "", &volts))
	assert.Equal(t, "Pi core amperes", chartTitle("", nil, "", &amperes))
}
