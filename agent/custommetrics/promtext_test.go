//go:build testing

package custommetrics

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   []Family
		errors []int // line numbers expected to fail
	}{
		{
			name:  "gauge without labels, no trailing newline",
			input: "pi_power_board_watts 1.84",
			want: []Family{{Name: "pi_power_board_watts", Samples: []Sample{
				{Name: "pi_power_board_watts", Value: 1.84, Line: 1},
			}}},
		},
		{
			name: "gauge with labels and trailing newline",
			input: "# TYPE disk_temp_celsius gauge\n" +
				`disk_temp_celsius{device="sda",host="x"} 41` + "\n" +
				`disk_temp_celsius{device="sdb"} 39.5` + "\n",
			want: []Family{{Name: "disk_temp_celsius", Type: Gauge, Samples: []Sample{
				{Name: "disk_temp_celsius", Labels: []Label{{"device", "sda"}, {"host", "x"}}, Value: 41, Line: 2},
				{Name: "disk_temp_celsius", Labels: []Label{{"device", "sdb"}}, Value: 39.5, Line: 3},
			}}},
		},
		{
			name: "several series with blank lines and comments",
			input: "# a comment\n\n" +
				"# HELP a_watts First.\n# TYPE a_watts gauge\na_watts 1\n" +
				"\n#another comment\n" +
				"# HELP b_volts Second.\nb_volts 2\n\n" +
				"c 3\n",
			want: []Family{
				{Name: "a_watts", Type: Gauge, Help: "First.", Samples: []Sample{{Name: "a_watts", Value: 1, Line: 5}}},
				{Name: "b_volts", Help: "Second.", Samples: []Sample{{Name: "b_volts", Value: 2, Line: 9}}},
				{Name: "c", Samples: []Sample{{Name: "c", Value: 3, Line: 11}}},
			},
		},
		{
			name: "gauge, untyped, unknown and missing TYPE",
			input: "# TYPE g gauge\ng 1\n" +
				"# TYPE u untyped\nu 2\n" +
				"# TYPE k unknown\nk 3\n" +
				"m 4\n",
			want: []Family{
				{Name: "g", Type: Gauge, Samples: []Sample{{Name: "g", Value: 1, Line: 2}}},
				{Name: "u", Type: Untyped, Samples: []Sample{{Name: "u", Value: 2, Line: 4}}},
				{Name: "k", Type: Untyped, Samples: []Sample{{Name: "k", Value: 3, Line: 6}}},
				{Name: "m", Type: Untyped, Samples: []Sample{{Name: "m", Value: 4, Line: 7}}},
			},
		},
		{
			name: "UNIT line",
			input: "# TYPE pi_power_board_watts gauge\n# UNIT pi_power_board_watts watts\n" +
				"pi_power_board_watts 1.84\n",
			want: []Family{{Name: "pi_power_board_watts", Type: Gauge, Unit: "watts", Samples: []Sample{
				{Name: "pi_power_board_watts", Value: 1.84, Line: 3},
			}}},
		},
		{
			name: "histogram and summary skipped with their children",
			input: "# HELP req_seconds Latency.\n# TYPE req_seconds histogram\n" +
				`req_seconds_bucket{le="0.1"} 3` + "\n" +
				`req_seconds_bucket{le="+Inf"} 5` + "\n" +
				"req_seconds_sum 0.4\nreq_seconds_count 5\nreq_seconds_created 1759500000.1\n" +
				"# TYPE rpc_seconds summary\n" +
				`rpc_seconds{quantile="0.5"} 0.2` + "\n" +
				"rpc_seconds_sum 1.5\nrpc_seconds_count 9\n" +
				"after 1\n",
			want: []Family{
				{Name: "req_seconds", Type: Unsupported, Help: "Latency."},
				{Name: "rpc_seconds", Type: Unsupported},
				{Name: "after", Samples: []Sample{{Name: "after", Value: 1, Line: 12}}},
			},
		},
		{
			name: "other unsupported OpenMetrics types",
			input: "# TYPE q gaugehistogram\nq_bucket{le=\"1\"} 1\nq_gcount 1\nq_gsum 1\n" +
				"# TYPE build info\nbuild_info{version=\"1\"} 1\n" +
				"# TYPE state stateset\nstate{state=\"a\"} 1\n",
			want: []Family{
				{Name: "q", Type: Unsupported},
				{Name: "build", Type: Unsupported},
				{Name: "state", Type: Unsupported},
			},
		},
		{
			name: "OpenMetrics counter with _total and _created",
			input: "# TYPE http_requests counter\n# HELP http_requests Requests served.\n" +
				`http_requests_total{code="200"} 1027` + "\n" +
				`http_requests_created{code="200"} 1759500000.5` + "\n" +
				"# EOF\n",
			want: []Family{{Name: "http_requests", Type: Counter, Help: "Requests served.", Samples: []Sample{
				{Name: "http_requests_total", Labels: []Label{{"code", "200"}}, Value: 1027, Line: 3},
			}}},
		},
		{
			name: "classic counter named with _total",
			input: "# TYPE node_network_receive_bytes_total counter\n" +
				`node_network_receive_bytes_total{device="eth0"} 12345` + "\n",
			want: []Family{{Name: "node_network_receive_bytes_total", Type: Counter, Samples: []Sample{
				{Name: "node_network_receive_bytes_total", Labels: []Label{{"device", "eth0"}}, Value: 12345, Line: 2},
			}}},
		},
		{
			name:  "parsing stops at # EOF",
			input: "a 1\n# EOF\nb 2\n",
			want:  []Family{{Name: "a", Samples: []Sample{{Name: "a", Value: 1, Line: 1}}}},
		},
		{
			name: "timestamps in milliseconds and seconds",
			input: "a 1 1759500000000\n" +
				"b 2 1759500000.25\n" +
				"c 3 1.7595e9\n" +
				"d 4\n",
			want: []Family{
				{Name: "a", Samples: []Sample{{Name: "a", Value: 1, TimestampMs: 1759500000000, Line: 1}}},
				{Name: "b", Samples: []Sample{{Name: "b", Value: 2, TimestampMs: 1759500000250, Line: 2}}},
				{Name: "c", Samples: []Sample{{Name: "c", Value: 3, TimestampMs: 1759500000000, Line: 3}}},
				{Name: "d", Samples: []Sample{{Name: "d", Value: 4, Line: 4}}},
			},
		},
		{
			name:  "exemplar after value or timestamp ignored",
			input: "# TYPE x counter\nx_total 5 # {trace_id=\"abc\"} 1\nx_total{a=\"1\"} 6 1759500000.0 # {trace_id=\"def\"} 1\n",
			want: []Family{{Name: "x", Type: Counter, Samples: []Sample{
				{Name: "x_total", Value: 5, Line: 2},
				{Name: "x_total", Labels: []Label{{"a", "1"}}, Value: 6, TimestampMs: 1759500000000, Line: 3},
			}}},
		},
		{
			name: "escapes in HELP and label values",
			input: `# HELP path_bytes Size of C:\\data` + `\n` + `second line` + "\n" +
				`path_bytes{dir="C:\\data",quote="say \"hi\"",nl="a\nb",other="\t"} 1` + "\n",
			want: []Family{{Name: "path_bytes", Help: "Size of C:\\data\nsecond line", Samples: []Sample{
				{Name: "path_bytes", Labels: []Label{
					{"dir", `C:\data`}, {"quote", `say "hi"`}, {"nl", "a\nb"}, {"other", `\t`},
				}, Value: 1, Line: 2},
			}}},
		},
		{
			name:  "trailing comma, empty braces and spaces in label sets",
			input: "a{x=\"1\",} 1\nb{} 2\nc { x = \"1\" , y=\"2\" } 3\n",
			want: []Family{
				{Name: "a", Samples: []Sample{{Name: "a", Labels: []Label{{"x", "1"}}, Value: 1, Line: 1}}},
				{Name: "b", Samples: []Sample{{Name: "b", Value: 2, Line: 2}}},
				{Name: "c", Samples: []Sample{{Name: "c", Labels: []Label{{"x", "1"}, {"y", "2"}}, Value: 3, Line: 3}}},
			},
		},
		{
			name:  "CRLF line endings",
			input: "# HELP a Text.\r\n# TYPE a gauge\r\na{x=\"1\"} 1\r\n\r\nb 2\r\n",
			want: []Family{
				{Name: "a", Type: Gauge, Help: "Text.", Samples: []Sample{{Name: "a", Labels: []Label{{"x", "1"}}, Value: 1, Line: 3}}},
				{Name: "b", Samples: []Sample{{Name: "b", Value: 2, Line: 5}}},
			},
		},
		{
			name: "unparseable lines skipped, the rest still read",
			input: "good 1\n" +
				"bad\n" +
				"bad_value abc\n" +
				"{x=\"1\"} 2\n" +
				"bad_label{x=1} 2\n" +
				"bad_label{x=\"1\" y=\"2\"} 2\n" +
				"unterminated{x=\"1} 2\n" +
				"dup{x=\"1\",x=\"2\"} 2\n" +
				"bad_ts 1 soon\n" +
				"extra 1 2 3\n" +
				"# TYPE 9bad gauge\n" +
				"# TYPE weird sometype\nweird 5\n" +
				"also_good 2\n",
			want: []Family{
				{Name: "good", Samples: []Sample{{Name: "good", Value: 1, Line: 1}}},
				{Name: "weird", Type: Unsupported},
				{Name: "also_good", Samples: []Sample{{Name: "also_good", Value: 2, Line: 14}}},
			},
			errors: []int{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
		},
		{
			name:  "same name after another family joins its family",
			input: "# TYPE a gauge\na 1\nb 2\na{x=\"1\"} 3\n",
			want: []Family{
				{Name: "a", Type: Gauge, Samples: []Sample{
					{Name: "a", Value: 1, Line: 2},
					{Name: "a", Labels: []Label{{"x", "1"}}, Value: 3, Line: 4},
				}},
				{Name: "b", Samples: []Sample{{Name: "b", Value: 2, Line: 3}}},
			},
		},
		{
			name: "metadata in a header block applies to samples further down",
			input: "# HELP pi_power_board_watts Board power.\n# TYPE pi_power_board_watts gauge\n" +
				"# HELP pi_energy_joules_total Energy used since boot.\n# TYPE pi_energy_joules_total counter\n" +
				"# TYPE http_requests counter\n# UNIT http_requests requests\n" +
				"# TYPE req_seconds histogram\n" +
				"pi_power_board_watts 1.84\n" +
				"pi_energy_joules_total 12345\n" +
				`http_requests_total{code="200"} 1027` + "\n" +
				`http_requests_created{code="200"} 1759500000.5` + "\n" +
				`req_seconds_bucket{le="1"} 3` + "\n" +
				"req_seconds_sum 2\nreq_seconds_count 3\n" +
				"untyped 7\n",
			want: []Family{
				{Name: "pi_power_board_watts", Type: Gauge, Help: "Board power.", Samples: []Sample{
					{Name: "pi_power_board_watts", Value: 1.84, Line: 8},
				}},
				{Name: "pi_energy_joules_total", Type: Counter, Help: "Energy used since boot.", Samples: []Sample{
					{Name: "pi_energy_joules_total", Value: 12345, Line: 9},
				}},
				{Name: "http_requests", Type: Counter, Unit: "requests", Samples: []Sample{
					{Name: "http_requests_total", Labels: []Label{{"code", "200"}}, Value: 1027, Line: 10},
				}},
				{Name: "req_seconds", Type: Unsupported},
				{Name: "untyped", Samples: []Sample{{Name: "untyped", Value: 7, Line: 15}}},
			},
		},
		{
			name: "a family with the sample's own name wins over one its name extends",
			input: "# TYPE x counter\n# TYPE x_total gauge\nx_total 5\n" +
				"# TYPE y gauge\ny_total 6\n",
			want: []Family{
				{Name: "x", Type: Counter},
				{Name: "x_total", Type: Gauge, Samples: []Sample{{Name: "x_total", Value: 5, Line: 3}}},
				{Name: "y", Type: Gauge},
				{Name: "y_total", Samples: []Sample{{Name: "y_total", Value: 6, Line: 5}}},
			},
		},
		{
			name: "a suffix the family's type does not claim starts a family of its own",
			input: "# TYPE c counter\nc_total 1\nc_bucket 2\n" +
				"# TYPE h histogram\nh_total 3\n",
			want: []Family{
				{Name: "c", Type: Counter, Samples: []Sample{{Name: "c_total", Value: 1, Line: 2}}},
				{Name: "c_bucket", Samples: []Sample{{Name: "c_bucket", Value: 2, Line: 3}}},
				{Name: "h", Type: Unsupported},
				{Name: "h_total", Samples: []Sample{{Name: "h_total", Value: 3, Line: 5}}},
			},
		},
		{
			name: "label syntax errors",
			input: "a{1x=\"1\"} 1\n" + // a label name cannot start with a digit
				"a{x \"1\"} 1\n" + // no =
				"a{x=\"1\\\n" + // ends in a backslash inside the value
				"ok 1\n",
			want:   []Family{{Name: "ok", Samples: []Sample{{Name: "ok", Value: 1, Line: 4}}}},
			errors: []int{1, 2, 3},
		},
		{
			name:   "malformed and out-of-range decimal timestamps",
			input:  "a 1 1.2.3\nb 2 1e300\nc 3\n",
			want:   []Family{{Name: "c", Samples: []Sample{{Name: "c", Value: 3, Line: 3}}}},
			errors: []int{1, 2},
		},
		{
			name:  "empty input",
			input: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, errs := Parse([]byte(tt.input))
			assert.Equal(t, tt.want, got)
			var lines []int
			for _, err := range errs {
				lines = append(lines, err.Line)
			}
			assert.Equal(t, tt.errors, lines, "lines with errors: %v", errs)
		})
	}
}

func TestParseNonFiniteValues(t *testing.T) {
	families, errs := Parse([]byte("a NaN\nb +Inf\nc -Inf\nd Inf\n"))
	require.Empty(t, errs)
	require.Len(t, families, 4)
	assert.True(t, math.IsNaN(families[0].Samples[0].Value))
	assert.True(t, math.IsInf(families[1].Samples[0].Value, 1))
	assert.True(t, math.IsInf(families[2].Samples[0].Value, -1))
	assert.True(t, math.IsInf(families[3].Samples[0].Value, 1))
}

// Text that reaches the wire must be valid UTF-8, or the hub rejects the
// whole CBOR response.
func TestParseInvalidUTF8(t *testing.T) {
	families, errs := Parse([]byte("# HELP a bad \xff byte\n# UNIT a wat\xfets\na 1\n"))
	require.Empty(t, errs)
	require.Len(t, families, 1)
	assert.Equal(t, "bad \uFFFD byte", families[0].Help)
	assert.Equal(t, "wat\uFFFDts", families[0].Unit)
}

func TestParseErrorMessage(t *testing.T) {
	_, errs := Parse([]byte("ok 1\nbroken\n"))
	require.Len(t, errs, 1)
	assert.Equal(t, "line 2: missing value for broken", errs[0].Error())
}

// owns decides which samples a family claims, and which of those it keeps.
func TestFamilyOwns(t *testing.T) {
	tests := []struct {
		family       Family
		name         string
		member, keep bool
	}{
		{Family{Name: "g", Type: Gauge}, "g", true, true},
		{Family{Name: "g", Type: Gauge}, "g_total", false, false},
		{Family{Name: "g", Type: Gauge}, "other", false, false},
		{Family{Name: "c", Type: Counter}, "c_total", true, true},
		{Family{Name: "c", Type: Counter}, "c_created", true, false},
		{Family{Name: "c", Type: Counter}, "c_bucket", false, false},
		{Family{Name: "h", Type: Unsupported}, "h", true, false},
		{Family{Name: "h", Type: Unsupported}, "h_bucket", true, false},
		{Family{Name: "h", Type: Unsupported}, "h_total", false, false},
	}
	for _, tt := range tests {
		member, keep := tt.family.owns(tt.name)
		assert.Equal(t, [2]bool{tt.member, tt.keep}, [2]bool{member, keep}, "%s owns %s", tt.family.Name, tt.name)
	}
}
